<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import QRCode from 'qrcode'
import {
  createRecharge,
  formatUsd,
  myRecharges,
  rechargeConfig,
  rechargeStatus,
  type Recharge,
  type RechargeConfig,
} from '../api/client'
import { isLoggedIn, loadMe, useSession } from '../session'

const { t } = useI18n()
const session = useSession()
const route = useRoute()

const config = ref<RechargeConfig | null>(null)
const amountYuan = ref<number>(50)
const method = ref('')
const yipaySub = ref<'alipay' | 'wxpay'>('alipay')
const busy = ref(false)

// active order being paid
const active = ref<Recharge | null>(null)
const qrDataUrl = ref('')
const lastPayUrl = ref('')
const polling = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null
let pollDeadline = 0

const history = ref<Recharge[]>([])

const presets = [10, 50, 100, 500]

const creditUsd = computed(() => {
  if (!config.value || !config.value.cny_per_usd) return ''
  return formatUsd(undefined, amountYuan.value / config.value.cny_per_usd)
})

function methodLabel(m: string): string {
  const key = `recharge.method.${m}`
  return t(key)
}

async function reload() {
  if (!isLoggedIn()) return
  try {
    config.value = await rechargeConfig()
    if (!method.value && config.value.methods.length) {
      method.value = config.value.methods[0]
    }
    history.value = await myRecharges(20)
    // deep link ?order=... (return_url after yipay redirect)
    const q = route.query.order
    if (typeof q === 'string' && q) {
      try {
        const list = await myRecharges(50)
        const found = list.find((r) => r.order_no === q)
        if (found && found.status === 'pending') {
          startPolling(found)
        } else if (found && found.status === 'paid') {
          ElMessage.success(t('recharge.paid'))
          void loadMe(true)
        }
      } catch {
        // ignore
      }
    }
  } catch {
    config.value = null
  }
}

onMounted(reload)

function renderQR(text: string) {
  void QRCode.toDataURL(text, { width: 220, margin: 1 }).then((url) => {
    qrDataUrl.value = url
  })
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  polling.value = false
}

function startPolling(order: Recharge) {
  active.value = order
  stopPolling()
  polling.value = true
  pollDeadline = Date.now() + 5 * 60 * 1000
  pollTimer = setInterval(async () => {
    if (Date.now() > pollDeadline) {
      stopPolling()
      return
    }
    try {
      const r = await rechargeStatus(order.id)
      if (r.status === 'paid') {
        stopPolling()
        active.value = null
        ElMessage.success(t('recharge.paid'))
        void loadMe(true)
        await reload()
      } else if (r.status === 'failed') {
        stopPolling()
        active.value = null
        await reload()
      }
    } catch {
      // transient network error; keep polling
    }
  }, 3000)
}

onBeforeUnmount(stopPolling)

async function submit() {
  if (!config.value || busy.value || !method.value) return
  const fen = Math.round(amountYuan.value * 100)
  if (fen < config.value.min_cny || fen > config.value.max_cny) return
  busy.value = true
  try {
    const res = await createRecharge(fen, method.value, method.value === 'yipay' ? yipaySub.value : undefined)
    lastPayUrl.value = res.payment.pay_url || ''
    if (res.payment.qr_code) {
      renderQR(res.payment.qr_code)
    }
    startPolling(res.order)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : t('recharge.failed'))
    await reload()
  } finally {
    busy.value = false
  }
}

function statusLabel(s: string): string {
  return t(`recharge.status.${s}`)
}
</script>

<template>
  <div class="page">
    <h2>{{ t('recharge.title') }}</h2>

    <div v-if="!session.user" class="card">
      <p class="muted">{{ t('recharge.loginHint') }}</p>
      <el-button type="primary" @click="$router.push('/login?redirect=/recharge')">
        {{ t('recharge.loginCta') }}
      </el-button>
    </div>

    <template v-else>
      <div v-if="!config || !config.methods.length" class="card">
        <p class="muted">{{ t('recharge.noMethods') }}</p>
      </div>

      <template v-else>
        <div class="card">
          <p class="muted">{{ t('recharge.sub', { rate: config.cny_per_usd }) }}</p>

          <div class="row">
            <span class="label">{{ t('recharge.amount') }}</span>
            <div class="amounts">
              <el-button
                v-for="p in presets"
                :key="p"
                size="small"
                :type="amountYuan === p ? 'primary' : 'default'"
                @click="amountYuan = p"
              >
                ¥{{ p }}
              </el-button>
              <el-input-number
                v-model="amountYuan"
                :min="1"
                :max="10000"
                :step="10"
                :precision="0"
                size="default"
                style="width: 140px"
              />
            </div>
            <span class="muted">{{ t('recharge.creditPreview') }}: <strong>{{ creditUsd || '—' }}</strong></span>
          </div>

          <div class="row">
            <span class="label">{{ t('recharge.methodLabel') }}</span>
            <el-select v-model="method" style="min-width: 240px">
              <el-option v-for="m in config.methods" :key="m" :label="methodLabel(m)" :value="m" />
            </el-select>
            <el-radio-group v-if="method === 'yipay'" v-model="yipaySub" size="small">
              <el-radio value="alipay">{{ t('recharge.yipaySub.alipay') }}</el-radio>
              <el-radio value="wxpay">{{ t('recharge.yipaySub.wxpay') }}</el-radio>
            </el-radio-group>
          </div>

          <p class="muted small">{{ t('recharge.minHint') }}</p>

          <el-button type="primary" :loading="busy" @click="submit">
            {{ busy ? t('recharge.creating') : t('recharge.confirm') }}
          </el-button>
        </div>

        <div v-if="active" class="card pay-panel">
          <div class="section-head">
            <h3>{{ t('recharge.qr', { method: methodLabel(active.method) }) }}</h3>
            <span class="muted">{{ active.order_no }}</span>
          </div>
          <div class="pay-body">
            <img v-if="qrDataUrl" :src="qrDataUrl" alt="QR" class="qr" />
            <div class="pay-meta">
              <p>
                {{ t('recharge.amount') }}: ¥{{ active.amount_yuan.toFixed(2) }}
              </p>
              <p>
                {{ t('recharge.creditPreview') }}: {{ formatUsd(active.credit_micro, active.credit_usd) }}
              </p>
              <a v-if="lastPayUrl" :href="lastPayUrl" target="_blank" rel="noopener">{{ t('recharge.payBtn') }}</a>
            </div>
          </div>
          <p v-if="polling" class="muted">{{ t('recharge.wait') }}</p>
        </div>

        <div class="card">
          <h3>{{ t('recharge.history') }}</h3>
          <el-table v-if="history.length" :data="history">
            <el-table-column prop="order_no" :label="t('recharge.orderNo')" min-width="190" />
            <el-table-column :label="t('recharge.methodLabel')" width="130">
              <template #default="{ row }">{{ methodLabel(row.method) }}</template>
            </el-table-column>
            <el-table-column :label="t('recharge.amount')" width="110">
              <template #default="{ row }">¥{{ row.amount_yuan.toFixed(2) }}</template>
            </el-table-column>
            <el-table-column :label="t('recharge.credit')" width="130">
              <template #default="{ row }">{{ formatUsd(row.credit_micro, row.credit_usd) }}</template>
            </el-table-column>
            <el-table-column :label="t('console.time')" width="180">
              <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
            </el-table-column>
            <el-table-column :label="t('recharge.status')" width="100">
              <template #default="{ row }">{{ statusLabel(row.status) }}</template>
            </el-table-column>
          </el-table>
          <p v-else class="muted">{{ t('recharge.historyEmpty') }}</p>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
h3 {
  margin: 0 0 8px;
}
.row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin: 12px 0;
}
.label {
  min-width: 120px;
  color: var(--text-dim);
}
.amounts {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.small {
  font-size: 12px;
  margin-top: 4px;
}
.card {
  margin-bottom: 16px;
}
.section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}
.pay-body {
  display: flex;
  gap: 24px;
  align-items: flex-start;
  flex-wrap: wrap;
  margin-top: 12px;
}
.qr {
  width: 220px;
  height: 220px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #fff;
  padding: 8px;
}
.pay-meta p {
  margin: 6px 0;
}
</style>
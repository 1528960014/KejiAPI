<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createMyKey,
  createSubkey,
  deleteMyKey,
  deleteSubkey,
  formatUsd,
  listSubkeys,
  myKeys,
  myLedger,
  setApiKey,
  type LedgerEntry,
  type MyKey,
  type Subkey,
} from '../api/client'
import { isLoggedIn, useSession } from '../session'

const { t } = useI18n()
const session = useSession()

const keys = ref<MyKey[]>([])
const ledger = ref<LedgerEntry[]>([])
const newName = ref('')
const busy = ref(false)
const createdKey = ref('')

// P2-3: reseller subkeys (only for agent accounts)
const isAgent = computed(() => session.user?.agent_rate != null)
const subkeys = ref<Subkey[]>([])
const subName = ref('')
const subMarkup = ref(1)
const subQuota = ref<number | undefined>(undefined)
const subBusy = ref(false)
const createdSubkey = ref('')

const example = `curl https://<your-host>/v1/chat/completions \\
  -H "Authorization: Bearer sk-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "hi"}],
    "stream": true
  }'`

async function reload() {
  if (!isLoggedIn()) return
  try {
    const [k, l] = await Promise.all([myKeys(), myLedger(50)])
    keys.value = k
    ledger.value = l
    if (isAgent.value) {
      subkeys.value = await listSubkeys()
    }
  } catch {
    // session expired or network error; App.vue reloads /me on navigation
  }
}

onMounted(reload)

async function createKey() {
  const name = newName.value.trim()
  if (!name || busy.value) return
  busy.value = true
  try {
    const res = await createMyKey(name)
    createdKey.value = res.key
    newName.value = ''
    await reload()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    busy.value = false
  }
}

async function copyKey() {
  if (!createdKey.value) return
  try {
    await navigator.clipboard.writeText(createdKey.value)
    ElMessage.success(t('console.copied'))
  } catch {
    ElMessage.warning(t('console.keyCreatedHint'))
  }
}

function useKeyInChat() {
  if (!createdKey.value) return
  setApiKey(createdKey.value)
  ElMessage.success(t('console.keyUseInChat'))
}

async function removeKey(key: MyKey) {
  try {
    await ElMessageBox.confirm(t('console.keyDeleteConfirm'), t('console.keyDelete'), {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await deleteMyKey(key.id)
    await reload()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  }
}

async function createSub() {
  const name = subName.value.trim()
  if (!name || subBusy.value) return
  subBusy.value = true
  try {
    const res = await createSubkey({
      name,
      markup: subMarkup.value,
      quota_usd: subQuota.value,
    })
    createdSubkey.value = res.key
    subName.value = ''
    await reload()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    subBusy.value = false
  }
}

async function copySubkey() {
  if (!createdSubkey.value) return
  try {
    await navigator.clipboard.writeText(createdSubkey.value)
    ElMessage.success(t('console.copied'))
  } catch {
    ElMessage.warning(t('console.keyCreatedHint'))
  }
}

async function removeSubkey(sub: Subkey) {
  try {
    await ElMessageBox.confirm(t('console.keyDeleteConfirm'), t('console.keyDelete'), {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await deleteSubkey(sub.id)
    await reload()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  }
}

function kindLabel(kind: string): string {
  const key = `console.ledgerKind.${kind}`
  return t(key)
}
</script>

<template>
  <div class="page">
    <h2>{{ t('console.title') }}</h2>

    <div class="card">
      <h3>{{ t('console.quickstart') }}</h3>
      <p class="muted">{{ t('console.docs') }}</p>
      <pre class="code">{{ example }}</pre>
    </div>

    <template v-if="session.user">
      <div class="card">
        <div class="section-head">
          <h3>{{ t('console.keys') }}</h3>
          <span class="balance">
            {{ t('console.balance') }}: <strong>{{ formatUsd(session.user.balance_micro, session.user.balance_usd) }}</strong>
          </span>
        </div>
        <div class="create-row">
          <el-input
            v-model="newName"
            :placeholder="t('console.keyNamePlaceholder')"
            style="max-width: 320px"
            @keydown.enter.prevent="createKey"
          />
          <el-button type="primary" :loading="busy" @click="createKey">
            {{ t('console.keyCreate') }}
          </el-button>
        </div>

        <div v-if="createdKey" class="created">
          <code class="new-key">{{ createdKey }}</code>
          <el-button size="small" @click="copyKey">{{ t('console.copy') }}</el-button>
          <el-button size="small" @click="useKeyInChat">{{ t('console.keyUseInChat') }}</el-button>
          <p class="muted">{{ t('console.keyCreatedHint') }}</p>
        </div>

        <el-table v-if="keys.length" :data="keys">
          <el-table-column prop="name" :label="t('console.keyName')" />
          <el-table-column :label="t('console.keySpend')" width="140">
            <template #default="{ row }">{{ formatUsd(row.spend_micro, row.spend_usd) }}</template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('console.keyCreatedAt')" width="220" />
          <el-table-column width="100">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeKey(row)">
                {{ t('console.keyDelete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('console.keysEmpty') }}</p>
      </div>

      <div v-if="isAgent" class="card">
        <h3>{{ t('console.subkeys') }}</h3>
        <p class="muted">{{ t('console.subkeysHint', { rate: session.user?.agent_rate }) }}</p>
        <div class="create-row">
          <el-input
            v-model="subName"
            :placeholder="t('console.subkeyNamePlaceholder')"
            style="max-width: 220px"
            @keydown.enter.prevent="createSub"
          />
          <el-input-number
            v-model="subMarkup"
            :min="1"
            :max="100"
            :step="0.1"
            :precision="2"
            size="default"
            style="width: 130px"
            :title="t('console.subkeyMarkup')"
          />
          <el-input-number
            v-model="subQuota"
            :min="0"
            :step="10"
            :precision="2"
            size="default"
            style="width: 150px"
            :title="t('console.subkeyQuota')"
          />
          <el-button type="primary" :loading="subBusy" @click="createSub">
            {{ t('console.subkeyCreate') }}
          </el-button>
        </div>

        <div v-if="createdSubkey" class="created">
          <code class="new-key">{{ createdSubkey }}</code>
          <el-button size="small" @click="copySubkey">{{ t('console.copy') }}</el-button>
          <p class="muted">{{ t('console.keyCreatedHint') }}</p>
        </div>

        <el-table v-if="subkeys.length" :data="subkeys">
          <el-table-column prop="name" :label="t('console.keyName')" />
          <el-table-column prop="markup" :label="t('console.subkeyMarkup')" width="90" />
          <el-table-column :label="t('console.keySpend')" width="140">
            <template #default="{ row }">{{ formatUsd(row.spend_micro, row.spend_usd) }}</template>
          </el-table-column>
          <el-table-column :label="t('console.subkeyQuota')" width="120">
            <template #default="{ row }">
              {{ row.quota_usd != null ? formatUsd(undefined, row.quota_usd) : '—' }}
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('console.keyCreatedAt')" width="220" />
          <el-table-column width="100">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeSubkey(row)">
                {{ t('console.keyDelete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('console.subkeysEmpty') }}</p>
      </div>

      <div class="card">
        <h3>{{ t('console.ledger') }}</h3>
        <el-table v-if="ledger.length" :data="ledger">
          <el-table-column :label="t('console.amount')" width="140">
            <template #default="{ row }">{{ formatUsd(row.amount, row.amount_usd) }}</template>
          </el-table-column>
          <el-table-column width="100">
            <template #default="{ row }">{{ kindLabel(row.kind) }}</template>
          </el-table-column>
          <el-table-column prop="reason" :label="t('console.reason')" />
          <el-table-column prop="created_at" :label="t('console.time')" width="220" />
        </el-table>
        <p v-else class="muted">{{ t('console.ledgerEmpty') }}</p>
      </div>
    </template>

    <div v-else class="card">
      <p class="muted">{{ t('console.loginHint') }}</p>
      <el-button type="primary" @click="$router.push('/login?redirect=/console')">
        {{ t('console.loginCta') }}
      </el-button>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
h3 {
  margin: 0 0 8px;
}
.code {
  background: #0b0b0e;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 14px;
  overflow-x: auto;
  font-size: 13px;
  line-height: 1.6;
}
.section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}
.balance {
  font-size: 14px;
  color: var(--text-dim);
}
.create-row {
  display: flex;
  gap: 12px;
  margin: 12px 0;
  align-items: center;
}
.created {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px;
  border: 1px solid var(--accent);
  border-radius: 8px;
  margin-bottom: 12px;
}
.new-key {
  font-size: 13px;
  word-break: break-all;
}
.created p {
  width: 100%;
  margin: 0;
  font-size: 12px;
}
.card {
  margin-bottom: 16px;
}
</style>
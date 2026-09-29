<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  addOrgMember,
  createOrg,
  createOrgKey,
  deleteOrgKey,
  formatUsd,
  getOrg,
  listMyOrgs,
  listOrgKeys,
  orgLedger,
  orgUsage,
  removeOrgMember,
  setOrgMemberRole,
  type MyOrg,
  type OrgDetail,
  type OrgKey,
  type OrgLedgerEntry,
  type OrgUsageRecord,
  type OrgUsageSummary,
} from '../api/client'
import { isLoggedIn, useSession } from '../session'

const { t } = useI18n()
const session = useSession()

const orgs = ref<MyOrg[]>([])
const newOrgName = ref('')
const createBusy = ref(false)

const selectedId = ref<number | null>(null)
const detail = ref<OrgDetail | null>(null)
const detailBusy = ref(false)

const membersBusy = ref(false)
const newMemberEmail = ref('')
const newMemberRole = ref<'admin' | 'member'>('member')

const orgKeys = ref<OrgKey[]>([])
const newKeyOrgName = ref('')
const keyBusy = ref(false)
const createdOrgKey = ref('')

const usageSummary = ref<OrgUsageSummary | null>(null)
const usageRows = ref<OrgUsageRecord[]>([])
const orgLedgerRows = ref<OrgLedgerEntry[]>([])

const isAdmin = computed(
  () =>
    detail.value != null &&
    (detail.value.role === 'owner' || detail.value.role === 'admin'),
)

async function reload() {
  if (!isLoggedIn()) return
  try {
    orgs.value = await listMyOrgs()
    if (orgs.value.length === 0) {
      selectedId.value = null
      detail.value = null
      return
    }
    if (
      selectedId.value == null ||
      !orgs.value.some((o) => o.id === selectedId.value)
    ) {
      selectedId.value = orgs.value[0].id
    }
    await reloadDetail()
  } catch {
    // session expired or network error; App.vue reloads /me on navigation
  }
}

async function reloadDetail() {
  if (selectedId.value == null) return
  detailBusy.value = true
  try {
    detail.value = await getOrg(selectedId.value)
    createdOrgKey.value = ''
    if (isAdmin.value) {
      const [keys, usage, ledger] = await Promise.all([
        listOrgKeys(selectedId.value),
        orgUsage(selectedId.value, 50),
        orgLedger(selectedId.value, 50),
      ])
      orgKeys.value = keys
      usageSummary.value = usage.summary
      usageRows.value = usage.data
      orgLedgerRows.value = ledger
    } else {
      orgKeys.value = await listOrgKeys(selectedId.value)
      usageSummary.value = null
      usageRows.value = []
      orgLedgerRows.value = []
    }
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    detailBusy.value = false
  }
}

onMounted(reload)

function selectOrg(org: MyOrg) {
  selectedId.value = org.id
  void reloadDetail()
}

async function createOrgAction() {
  const name = newOrgName.value.trim()
  if (!name || createBusy.value) return
  createBusy.value = true
  try {
    const org = await createOrg(name)
    newOrgName.value = ''
    await reload()
    selectedId.value = org.id
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    createBusy.value = false
  }
}

async function inviteMember() {
  const email = newMemberEmail.value.trim()
  if (!email || membersBusy.value || !detail.value) return
  membersBusy.value = true
  try {
    await addOrgMember(detail.value.id, email, newMemberRole.value)
    newMemberEmail.value = ''
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    membersBusy.value = false
  }
}

async function changeRole(row: { user_id: number }, role: 'admin' | 'member') {
  if (!detail.value) return
  try {
    await setOrgMemberRole(detail.value.id, row.user_id, role)
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  }
}

async function removeMember(row: { user_id: number; email: string }) {
  if (!detail.value) return
  try {
    await ElMessageBox.confirm(
      t('orgs.removeConfirm', { email: row.email }),
      t('orgs.remove'),
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await removeOrgMember(detail.value.id, row.user_id)
    await reload()
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  }
}

async function createKeyAction() {
  const name = newKeyOrgName.value.trim()
  if (!name || keyBusy.value || !detail.value) return
  keyBusy.value = true
  try {
    const res = await createOrgKey(detail.value.id, { name })
    createdOrgKey.value = res.key
    newKeyOrgName.value = ''
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  } finally {
    keyBusy.value = false
  }
}

async function copyOrgKey() {
  if (!createdOrgKey.value) return
  try {
    await navigator.clipboard.writeText(createdOrgKey.value)
    ElMessage.success(t('console.copied'))
  } catch {
    ElMessage.warning(t('console.keyCreatedHint'))
  }
}

async function deleteKey(row: OrgKey) {
  if (!detail.value) return
  try {
    await ElMessageBox.confirm(
      t('console.keyDeleteConfirm'),
      t('console.keyDelete'),
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await deleteOrgKey(detail.value.id, row.id)
    await reloadDetail()
  } catch {
    ElMessage.error(t('login.errNetwork'))
  }
}

function roleLabel(role: string): string {
  return t(`orgs.roles.${role}`)
}

function kindLabel(kind: string): string {
  return t(`orgs.ledgerKind.${kind}`)
}
</script>

<template>
  <div class="page">
    <h2>{{ t('orgs.title') }}</h2>

    <template v-if="session.user">
      <div class="card">
        <h3>{{ t('orgs.list') }}</h3>
        <div class="create-row">
          <el-input
            v-model="newOrgName"
            :placeholder="t('orgs.namePlaceholder')"
            style="max-width: 320px"
            @keydown.enter.prevent="createOrgAction"
          />
          <el-button type="primary" :loading="createBusy" @click="createOrgAction">
            {{ t('orgs.create') }}
          </el-button>
        </div>
        <el-table
          v-if="orgs.length"
          :data="orgs"
          highlight-current-row
          @current-change="(row: MyOrg | null) => row && selectOrg(row)"
        >
          <el-table-column prop="name" :label="t('orgs.name')" />
          <el-table-column :label="t('orgs.role')" width="120">
            <template #default="{ row }">{{ roleLabel(row.role) }}</template>
          </el-table-column>
          <el-table-column :label="t('orgs.balance')" width="160">
            <template #default="{ row }">
              {{ formatUsd(row.balance_micro, row.balance_usd) }}
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('orgs.createdAt')" width="220" />
        </el-table>
        <p v-else class="muted">{{ t('orgs.listEmpty') }}</p>
      </div>

      <div v-if="detail" class="card" v-loading="detailBusy">
        <div class="section-head">
          <h3>{{ detail.name }}</h3>
          <span class="balance">
            {{ t('orgs.balance') }}:
            <strong>{{ formatUsd(detail.balance_micro, detail.balance_usd) }}</strong>
          </span>
        </div>

        <h4>{{ t('orgs.members') }}</h4>
        <div v-if="isAdmin" class="create-row">
          <el-input
            v-model="newMemberEmail"
            :placeholder="t('orgs.emailPlaceholder')"
            style="max-width: 280px"
            @keydown.enter.prevent="inviteMember"
          />
          <el-select v-model="newMemberRole" style="width: 130px">
            <el-option value="member" :label="t('orgs.role.member')" />
            <el-option value="admin" :label="t('orgs.role.admin')" />
          </el-select>
          <el-button :loading="membersBusy" @click="inviteMember">
            {{ t('orgs.invite') }}
          </el-button>
        </div>
        <el-table v-if="detail.members.length" :data="detail.members">
          <el-table-column prop="email" :label="t('orgs.email')" />
          <el-table-column :label="t('orgs.role')" width="140">
            <template #default="{ row }">
              <el-select
                v-if="isAdmin && row.role !== 'owner'"
                :model-value="row.role"
                size="small"
                style="width: 110px"
                @change="(val: string) => changeRole(row, val as 'admin' | 'member')"
              >
                <el-option value="member" :label="t('orgs.role.member')" />
                <el-option value="admin" :label="t('orgs.role.admin')" />
              </el-select>
              <span v-else>{{ roleLabel(row.role) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="joined_at" :label="t('orgs.joinedAt')" width="220" />
          <el-table-column v-if="isAdmin" width="120">
            <template #default="{ row }">
              <el-button
                v-if="row.role !== 'owner'"
                size="small"
                type="danger"
                plain
                @click="removeMember(row)"
              >
                {{ t('orgs.remove') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('orgs.membersEmpty') }}</p>

        <h4>{{ t('orgs.keys') }}</h4>
        <div v-if="isAdmin" class="create-row">
          <el-input
            v-model="newKeyOrgName"
            :placeholder="t('console.keyNamePlaceholder')"
            style="max-width: 280px"
            @keydown.enter.prevent="createKeyAction"
          />
          <el-button type="primary" :loading="keyBusy" @click="createKeyAction">
            {{ t('orgs.keyCreate') }}
          </el-button>
        </div>
        <div v-if="createdOrgKey" class="created">
          <code class="new-key">{{ createdOrgKey }}</code>
          <el-button size="small" @click="copyOrgKey">{{ t('console.copy') }}</el-button>
          <p class="muted">{{ t('console.keyCreatedHint') }}</p>
        </div>
        <el-table v-if="orgKeys.length" :data="orgKeys">
          <el-table-column prop="name" :label="t('console.keyName')" />
          <el-table-column :label="t('console.keySpend')" width="140">
            <template #default="{ row }">
              {{ formatUsd(row.spend_micro, row.spend_usd) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('console.subkeyQuota')" width="120">
            <template #default="{ row }">
              {{ row.quota_usd != null ? formatUsd(undefined, row.quota_usd) : '—' }}
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('console.keyCreatedAt')" width="220" />
          <el-table-column v-if="isAdmin" width="100">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="deleteKey(row)">
                {{ t('console.keyDelete') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('orgs.keysEmpty') }}</p>

        <template v-if="isAdmin && usageSummary">
          <h4>{{ t('orgs.usage') }}</h4>
          <div class="stats">
            <div class="stat">
              <span class="stat-num">{{ usageSummary.requests }}</span>
              <span class="muted">{{ t('orgs.usage.requests') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ usageSummary.prompt_tokens }}</span>
              <span class="muted">{{ t('orgs.usage.prompt') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ usageSummary.completion_tokens }}</span>
              <span class="muted">{{ t('orgs.usage.completion') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ formatUsd(undefined, usageSummary.cost_usd) }}</span>
              <span class="muted">{{ t('orgs.usage.cost') }}</span>
            </div>
          </div>
          <el-table v-if="usageRows.length" :data="usageRows" size="small">
            <el-table-column prop="model_id" label="model" />
            <el-table-column prop="key_name" :label="t('console.keyName')" width="140" />
            <el-table-column prop="prompt_tokens" :label="t('orgs.usage.prompt')" width="110" />
            <el-table-column prop="completion_tokens" :label="t('orgs.usage.completion')" width="110" />
            <el-table-column :label="t('orgs.usage.cost')" width="110">
              <template #default="{ row }">{{ formatUsd(undefined, row.cost_usd) }}</template>
            </el-table-column>
            <el-table-column prop="created_at" :label="t('console.time')" width="200" />
          </el-table>
          <p v-else class="muted">{{ t('orgs.usageEmpty') }}</p>

          <h4>{{ t('orgs.ledger') }}</h4>
          <el-table v-if="orgLedgerRows.length" :data="orgLedgerRows" size="small">
            <el-table-column :label="t('console.amount')" width="130">
              <template #default="{ row }">{{ formatUsd(row.amount, row.amount_usd) }}</template>
            </el-table-column>
            <el-table-column width="100">
              <template #default="{ row }">{{ kindLabel(row.kind) }}</template>
            </el-table-column>
            <el-table-column prop="reason" :label="t('console.reason')" />
            <el-table-column prop="created_at" :label="t('console.time')" width="200" />
          </el-table>
          <p v-else class="muted">{{ t('orgs.ledgerEmpty') }}</p>
        </template>
      </div>
    </template>

    <div v-else class="card">
      <p class="muted">{{ t('console.loginHint') }}</p>
      <el-button type="primary" @click="$router.push('/login?redirect=/orgs')">
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
h4 {
  margin: 20px 0 8px;
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
.stats {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.stat-num {
  font-size: 18px;
  font-weight: 600;
}
</style>
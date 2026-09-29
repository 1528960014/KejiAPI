<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  adminRecharges,
  createAdminModel,
  createAgent,
  createAssistant,
  createChannel,
  createKey,
  createUser,
  creditUser,
  deleteAdminModel,
  deleteAgent,
  deleteAssistant,
  deleteChannel,
  deleteKey,
  formatUsd,
  getMasterKey,
  getPayConfig,
  listAdminModels,
  listAgents,
  listAssistants,
  listChannels,
  listKeys,
  listUsers,
  listUsage,
  putPayConfig,
  setMasterKey,
  updateAgentRate,
  usageSummary,
  PAY_CHANNEL_FIELDS,
  PAY_CHANNEL_IDS,
  type AdminAgent,
  type AdminAssistant,
  type AdminChannel,
  type AdminKey,
  type AdminModel,
  type AdminUser,
  type PayConfigView,
  type Recharge,
  type UsageRecord,
  type UsageSummary,
} from '../api/client'

const { t } = useI18n()

type TabName =
  | 'overview'
  | 'models'
  | 'channels'
  | 'users'
  | 'keys'
  | 'agents'
  | 'assistants'
  | 'pay'
  | 'usage'
  | 'recharges'

const masterKey = ref(getMasterKey())
const tab = ref<TabName>('overview')
const loaded = reactive<Record<TabName, boolean>>({
  overview: false,
  models: false,
  channels: false,
  users: false,
  keys: false,
  agents: false,
  assistants: false,
  pay: false,
  usage: false,
  recharges: false,
})

function onKeyChange() {
  setMasterKey(masterKey.value.trim())
  // master key changed: force reload of every tab
  for (const k of Object.keys(loaded) as TabName[]) loaded[k] = false
  void loadTab(tab.value)
}

function errMsg(err: unknown): string {
  const code = (
    err as { response?: { data?: { error?: { code?: string } } } }
  )?.response?.data?.error?.code
  if (code === 'unauthorized' || code === 'forbidden') return t('admin.badKey')
  const msg = (err as { response?: { data?: { error?: { message?: string } } } })
    ?.response?.data?.error?.message
  return msg || String(err)
}

async function loadTab(name: TabName) {
  if (!masterKey.value) return
  try {
    if (name === 'overview') {
      await Promise.all([loadModels(true), loadChannels(true), loadUsers(true), loadSummary()])
    } else if (name === 'models') await loadModels()
    else if (name === 'channels') await loadChannels()
    else if (name === 'users') await loadUsers()
    else if (name === 'keys') await loadKeys()
    else if (name === 'agents') await loadAgents()
    else if (name === 'assistants') await loadAssistants()
    else if (name === 'pay') await loadPay()
    else if (name === 'usage') await Promise.all([loadSummary(), loadUsage()])
    else if (name === 'recharges') await loadRecharges()
    loaded[name] = true
  } catch (err) {
    if (name === 'overview') return // partial load failure is fine for overview
    ElMessage.error(errMsg(err))
  }
}

function onTabChange(name: string | number) {
  const n = name as TabName
  if (!loaded[n]) void loadTab(n)
}

onMounted(() => {
  if (masterKey.value) void loadTab('overview')
})

// ---------- overview ----------
const summary = ref<UsageSummary | null>(null)
const modelCount = ref(0)
const channelCount = ref(0)
const userCount = ref(0)

async function loadSummary() {
  summary.value = await usageSummary()
}

// ---------- models ----------
const models = ref<AdminModel[]>([])
async function loadModels(silent = false) {
  models.value = await listAdminModels()
  modelCount.value = models.value.length
  if (!silent) loaded.models = true
}

const CAPS = ['text', 'image', 'video', 'music', 'tts']
const modelForm = reactive({
  model_id: '',
  provider: '',
  upstream_model: '',
  capabilities: [] as string[],
  input_price_per_1k: 0,
  output_price_per_1k: 0,
  price_unit: 'token',
  unit_price: 0,
  enabled: true,
})
async function addModel() {
  if (!modelForm.model_id || !modelForm.provider || !modelForm.upstream_model) {
    ElMessage.warning(t('admin.modelRequired'))
    return
  }
  try {
    await createAdminModel({
      model_id: modelForm.model_id.trim(),
      provider: modelForm.provider.trim(),
      upstream_model: modelForm.upstream_model.trim(),
      capabilities: modelForm.capabilities,
      input_price_per_1k: modelForm.input_price_per_1k,
      output_price_per_1k: modelForm.output_price_per_1k,
      price_unit: modelForm.price_unit,
      unit_price: modelForm.price_unit === 'token' ? undefined : modelForm.unit_price,
      enabled: modelForm.enabled,
    })
    ElMessage.success(t('admin.saved'))
    modelForm.model_id = ''
    modelForm.upstream_model = ''
    await loadModels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeModel(m: AdminModel) {
  try {
    await ElMessageBox.confirm(t('admin.confirmDelete', { name: m.model_id }), t('admin.delete'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteAdminModel(m.model_id)
    await loadModels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- channels ----------
const channels = ref<AdminChannel[]>([])
async function loadChannels(silent = false) {
  channels.value = await listChannels()
  channelCount.value = channels.value.length
  if (!silent) loaded.channels = true
}
const channelForm = reactive({
  name: '',
  provider: '',
  base_url: '',
  api_key: '',
  model_id: '',
  priority: 0,
})
async function addChannel() {
  if (!channelForm.name || !channelForm.provider || !channelForm.base_url || !channelForm.api_key || !channelForm.model_id) {
    ElMessage.warning(t('admin.channelRequired'))
    return
  }
  try {
    await createChannel({ ...channelForm, name: channelForm.name.trim() })
    ElMessage.success(t('admin.saved'))
    channelForm.name = ''
    channelForm.api_key = ''
    await loadChannels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeChannel(ch: AdminChannel) {
  try {
    await ElMessageBox.confirm(t('admin.confirmDelete', { name: ch.name }), t('admin.delete'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteChannel(ch.id)
    await loadChannels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- users ----------
const users = ref<AdminUser[]>([])
async function loadUsers(silent = false) {
  users.value = await listUsers()
  userCount.value = users.value.length
  if (!silent) loaded.users = true
}
const userForm = reactive({ email: '', balance: 0 })
async function addUser() {
  if (!userForm.email.trim()) {
    ElMessage.warning(t('admin.userRequired'))
    return
  }
  try {
    await createUser(userForm.email.trim(), userForm.balance)
    ElMessage.success(t('admin.saved'))
    userForm.email = ''
    userForm.balance = 0
    await loadUsers()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function credit(u: AdminUser) {
  const { value } = await ElMessageBox.prompt(t('admin.creditPrompt'), t('admin.credit'), {
    inputPattern: /^\d+(\.\d+)?$/,
    inputErrorMessage: t('admin.creditInvalid'),
  }).catch(() => ({ value: '' }))
  const amount = Number(value)
  if (!value || Number.isNaN(amount)) return
  try {
    await creditUser(u.id, amount, 'manual admin credit')
    ElMessage.success(t('admin.saved'))
    await loadUsers()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- keys ----------
const keys = ref<AdminKey[]>([])
async function loadKeys() {
  keys.value = await listKeys()
}
const keyName = ref('')
async function addKey() {
  if (!keyName.value.trim()) {
    ElMessage.warning(t('admin.keyRequired'))
    return
  }
  try {
    const res = await createKey(keyName.value.trim())
    ElMessageBox.alert(res.key, t('admin.keyCreated'), { confirmButtonText: 'OK' })
    keyName.value = ''
    await loadKeys()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeKey(k: AdminKey) {
  try {
    await ElMessageBox.confirm(t('admin.confirmDelete', { name: k.name }), t('admin.delete'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteKey(k.id)
    await loadKeys()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- agents ----------
const agents = ref<AdminAgent[]>([])
async function loadAgents() {
  agents.value = await listAgents()
}
const agentForm = reactive({ user_id: '', rate: 1 })
async function addAgent() {
  if (!agentForm.user_id || agentForm.rate < 1) {
    ElMessage.warning(t('admin.agentRequired'))
    return
  }
  try {
    await createAgent(Number(agentForm.user_id), agentForm.rate)
    ElMessage.success(t('admin.saved'))
    agentForm.user_id = ''
    agentForm.rate = 1
    await loadAgents()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function updateRate(a: AdminAgent) {
  const { value } = await ElMessageBox.prompt(t('admin.ratePrompt'), t('admin.setRate'), {
    inputPattern: /^\d+(\.\d+)?$/,
    inputErrorMessage: t('admin.creditInvalid'),
  }).catch(() => ({ value: '' }))
  const rate = Number(value)
  if (!value || Number.isNaN(rate) || rate < 1) return
  try {
    await updateAgentRate(a.id, rate)
    await loadAgents()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeAgent(a: AdminAgent) {
  try {
    await ElMessageBox.confirm(t('admin.confirmDelete', { name: a.email }), t('admin.delete'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteAgent(a.id)
    await loadAgents()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- assistants ----------
const assistants = ref<AdminAssistant[]>([])
async function loadAssistants() {
  assistants.value = await listAssistants()
}
const assistantForm = reactive({
  agent_id: '',
  name: '',
  description: '',
  system_prompt: '',
  model: '',
  tools: '',
  enabled: true,
})
async function addAssistant() {
  if (!assistantForm.agent_id || !assistantForm.name || !assistantForm.system_prompt || !assistantForm.model) {
    ElMessage.warning(t('admin.assistantRequired'))
    return
  }
  try {
    await createAssistant({
      agent_id: assistantForm.agent_id.trim(),
      name: assistantForm.name.trim(),
      description: assistantForm.description.trim(),
      system_prompt: assistantForm.system_prompt,
      model: assistantForm.model.trim(),
      tools: assistantForm.tools || undefined,
      enabled: assistantForm.enabled,
    })
    ElMessage.success(t('admin.saved'))
    assistantForm.agent_id = ''
    assistantForm.name = ''
    assistantForm.system_prompt = ''
    await loadAssistants()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeAssistant(a: AdminAssistant) {
  try {
    await ElMessageBox.confirm(t('admin.confirmDelete', { name: a.agent_id }), t('admin.delete'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteAssistant(a.id)
    await loadAssistants()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- pay ----------
const pay = ref<PayConfigView | null>(null)
const payForms = reactive<Record<string, { enabled: boolean; config: Record<string, string> }>>({})
async function loadPay() {
  const cfg = await getPayConfig()
  pay.value = cfg
  for (const id of PAY_CHANNEL_IDS) {
    const ch = cfg.channels[id]
    payForms[id] = {
      enabled: ch?.enabled ?? false,
      config: { ...(ch?.config ?? {}) },
    }
  }
}
async function savePay() {
  if (!pay.value) return
  const body: {
    cny_per_usd: number
    public_url: string
    channels: Record<string, { enabled: boolean; config: Record<string, string> }>
  } = {
    cny_per_usd: pay.value.cny_per_usd,
    public_url: pay.value.public_url,
    channels: {},
  }
  for (const id of PAY_CHANNEL_IDS) {
    const f = payForms[id]
    if (!f) continue
    const cfg: Record<string, string> = {}
    for (const field of PAY_CHANNEL_FIELDS[id]) {
      cfg[field.name] = f.config[field.name] ?? ''
    }
    body.channels[id] = { enabled: f.enabled, config: cfg }
  }
  try {
    const fresh = await putPayConfig(body)
    pay.value = fresh
    ElMessage.success(t('admin.saved'))
    // refresh the masked view
    await loadPay()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
const payStatusText = (ch: { ok: boolean; error?: string }) =>
  ch.ok ? t('admin.payOk') : ch.error || t('admin.payBroken')

// ---------- usage ----------
const usage = ref<UsageRecord[]>([])
async function loadUsage() {
  usage.value = await listUsage(undefined, 50)
}

// ---------- recharges ----------
const recharges = ref<Recharge[]>([])
async function loadRecharges() {
  recharges.value = await adminRecharges(50)
}
</script>

<template>
  <div class="page admin">
    <h2>{{ t('admin.title') }}</h2>

    <div class="card key-card">
      <label class="key-label">
        {{ t('admin.masterKey') }}
        <el-input
          v-model="masterKey"
          type="password"
          show-password
          :placeholder="t('admin.masterKeyPlaceholder')"
          @change="onKeyChange"
        />
      </label>
      <p class="muted hint">{{ t('admin.masterKeyHint') }}</p>
    </div>

    <el-tabs v-if="masterKey" v-model="tab" class="admin-tabs" @tab-change="onTabChange">
      <!-- overview -->
      <el-tab-pane :label="t('admin.tabOverview')" name="overview">
        <div class="stats">
          <div class="stat">
            <div class="stat-num">{{ userCount }}</div>
            <div class="stat-label">{{ t('admin.statUsers') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ modelCount }}</div>
            <div class="stat-label">{{ t('admin.statModels') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ channelCount }}</div>
            <div class="stat-label">{{ t('admin.statChannels') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ summary?.requests ?? '—' }}</div>
            <div class="stat-label">{{ t('admin.statRequests') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ summary ? formatUsd(summary.cost_micro, summary.cost_usd) : '—' }}</div>
            <div class="stat-label">{{ t('admin.statCost') }}</div>
          </div>
        </div>
      </el-tab-pane>

      <!-- models -->
      <el-tab-pane :label="t('admin.tabModels')" name="models">
        <div class="form-card">
          <h3>{{ t('admin.modelAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.modelId') }}
              <el-input v-model="modelForm.model_id" placeholder="grok-4.7" />
            </label>
            <label>{{ t('admin.provider') }}
              <el-input v-model="modelForm.provider" placeholder="openai" />
            </label>
            <label>{{ t('admin.upstreamModel') }}
              <el-input v-model="modelForm.upstream_model" placeholder="grok-4.7" />
            </label>
            <label>{{ t('admin.capabilities') }}
              <el-select v-model="modelForm.capabilities" multiple style="width: 100%">
                <el-option v-for="c in CAPS" :key="c" :label="t(`admin.cap.${c}`)" :value="c" />
              </el-select>
            </label>
            <label>{{ t('admin.priceUnit') }}
              <el-select v-model="modelForm.price_unit" style="width: 100%">
                <el-option label="token" value="token" />
                <el-option label="image" value="image" />
                <el-option label="video" value="video" />
                <el-option label="music" value="music" />
                <el-option label="tts" value="tts" />
              </el-select>
            </label>
            <template v-if="modelForm.priceUnit === 'token'">
              <label>{{ t('admin.inputPrice') }}
                <el-input-number v-model="modelForm.input_price_per_1k" :min="0" :precision="6" :step="0.001" style="width: 100%" />
              </label>
              <label>{{ t('admin.outputPrice') }}
                <el-input-number v-model="modelForm.output_price_per_1k" :min="0" :precision="6" :step="0.001" style="width: 100%" />
              </label>
            </template>
            <label v-else>{{ t('admin.unitPrice') }}
              <el-input-number v-model="modelForm.unit_price" :min="0" :precision="4" :step="0.01" style="width: 100%" />
            </label>
            <label class="switch-label">{{ t('admin.enabled') }}
              <el-switch v-model="modelForm.enabled" />
            </label>
          </div>
          <el-button type="primary" @click="addModel">{{ t('admin.add') }}</el-button>
        </div>

        <el-table v-if="models.length" :data="models">
          <el-table-column prop="model_id" :label="t('admin.modelId')" min-width="160" />
          <el-table-column prop="provider" :label="t('admin.provider')" width="110" />
          <el-table-column prop="upstream_model" :label="t('admin.upstreamModel')" min-width="140" />
          <el-table-column :label="t('admin.capabilities')" min-width="140">
            <template #default="{ row }">
              <el-tag v-for="c in row.capabilities" :key="c" size="small" class="cap-tag">
                {{ t(`admin.cap.${c}`) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.price')" width="190">
            <template #default="{ row }">
              <span v-if="row.price_unit && row.price_unit !== 'token'">
                ${{ row.unit_price }} / {{ row.price_unit }}
              </span>
              <span v-else>
                {{ row.input_price_per_1k }} → {{ row.output_price_per_1k }} /1k
              </span>
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.enabled')" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                {{ row.enabled ? t('admin.on') : t('admin.off') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeModel(row)">{{ t('admin.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- channels -->
      <el-tab-pane :label="t('admin.tabChannels')" name="channels">
        <div class="form-card">
          <h3>{{ t('admin.channelAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.channelName') }}
              <el-input v-model="channelForm.name" placeholder="kejike" />
            </label>
            <label>{{ t('admin.provider') }}
              <el-input v-model="channelForm.provider" placeholder="openai" />
            </label>
            <label>{{ t('admin.baseUrl') }}
              <el-input v-model="channelForm.base_url" placeholder="https://sub.kejike.top/v1" />
            </label>
            <label>{{ t('admin.apiKey') }}
              <el-input v-model="channelForm.api_key" type="password" show-password placeholder="sk-..." />
            </label>
            <label>{{ t('admin.modelId') }}
              <el-input v-model="channelForm.model_id" placeholder="grok-4.7" />
            </label>
            <label>{{ t('admin.priority') }}
              <el-input-number v-model="channelForm.priority" :min="0" style="width: 100%" />
            </label>
          </div>
          <el-button type="primary" @click="addChannel">{{ t('admin.add') }}</el-button>
        </div>

        <el-table v-if="channels.length" :data="channels">
          <el-table-column prop="name" :label="t('admin.channelName')" min-width="120" />
          <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
          <el-table-column prop="base_url" :label="t('admin.baseUrl')" min-width="180" />
          <el-table-column prop="model_id" :label="t('admin.modelId')" min-width="120" />
          <el-table-column prop="priority" :label="t('admin.priority')" width="90" />
          <el-table-column :label="t('admin.health')" width="110">
            <template #default="{ row }">
              <el-tag :type="row.health === 'ok' ? 'success' : 'warning'" size="small">
                {{ row.health === 'ok' ? t('admin.healthOk') : t('admin.healthCooldown') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeChannel(row)">{{ t('admin.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- users -->
      <el-tab-pane :label="t('admin.tabUsers')" name="users">
        <div class="form-card">
          <h3>{{ t('admin.userAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.email') }}
              <el-input v-model="userForm.email" placeholder="user@example.com" />
            </label>
            <label>{{ t('admin.initialBalance') }}
              <el-input-number v-model="userForm.balance" :min="0" :precision="2" :step="10" style="width: 100%" />
            </label>
          </div>
          <el-button type="primary" @click="addUser">{{ t('admin.add') }}</el-button>
        </div>

        <el-table v-if="users.length" :data="users">
          <el-table-column prop="id" :label="t('admin.id')" width="70" />
          <el-table-column prop="email" :label="t('admin.email')" min-width="180" />
          <el-table-column :label="t('admin.balance')" width="140">
            <template #default="{ row }">{{ formatUsd(row.balance_micro, row.balance_usd) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.agentRate')" width="110">
            <template #default="{ row }">{{ row.agent_rate != null ? `×${row.agent_rate}` : '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.enabled')" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                {{ row.enabled ? t('admin.on') : t('admin.off') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('admin.createdAt')" width="180" />
          <el-table-column width="100">
            <template #default="{ row }">
              <el-button size="small" @click="credit(row)">{{ t('admin.credit') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- keys -->
      <el-tab-pane :label="t('admin.tabKeys')" name="keys">
        <div class="form-card">
          <h3>{{ t('admin.keyAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.keyName') }}
              <el-input v-model="keyName" placeholder="demo-key" />
            </label>
          </div>
          <el-button type="primary" @click="addKey">{{ t('admin.add') }}</el-button>
        </div>

        <el-table v-if="keys.length" :data="keys">
          <el-table-column prop="id" :label="t('admin.id')" width="70" />
          <el-table-column prop="name" :label="t('admin.keyName')" min-width="140" />
          <el-table-column :label="t('admin.spent')" width="130">
            <template #default="{ row }">{{ formatUsd(row.spend_micro, row.spend_usd) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.allowedModels')" min-width="180">
            <template #default="{ row }">
              <span v-if="(row.allowed_models || []).length">{{ row.allowed_models.join(', ') }}</span>
              <span v-else class="muted">{{ t('admin.allModels') }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('admin.createdAt')" width="180" />
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeKey(row)">{{ t('admin.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- agents -->
      <el-tab-pane :label="t('admin.tabAgents')" name="agents">
        <div class="form-card">
          <h3>{{ t('admin.agentAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.agentUserId') }}
              <el-input v-model="agentForm.user_id" placeholder="user id" />
            </label>
            <label>{{ t('admin.agentRate') }}
              <el-input-number v-model="agentForm.rate" :min="1" :precision="2" :step="0.1" style="width: 100%" />
            </label>
          </div>
          <el-button type="primary" @click="addAgent">{{ t('admin.add') }}</el-button>
        </div>

        <el-table v-if="agents.length" :data="agents">
          <el-table-column prop="id" :label="t('admin.id')" width="70" />
          <el-table-column prop="email" :label="t('admin.email')" min-width="180" />
          <el-table-column prop="rate" :label="t('admin.agentRate')" width="110" />
          <el-table-column prop="created_at" :label="t('admin.createdAt')" width="180" />
          <el-table-column width="170">
            <template #default="{ row }">
              <el-button size="small" @click="updateRate(row)">{{ t('admin.setRate') }}</el-button>
              <el-button size="small" type="danger" plain @click="removeAgent(row)">{{ t('admin.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- assistants -->
      <el-tab-pane :label="t('admin.tabAssistants')" name="assistants">
        <div class="form-card">
          <h3>{{ t('admin.assistantAdd') }}</h3>
          <div class="grid">
            <label>{{ t('admin.assistantId') }}
              <el-input v-model="assistantForm.agent_id" placeholder="translator" />
            </label>
            <label>{{ t('admin.assistantName') }}
              <el-input v-model="assistantForm.name" placeholder="Translator" />
            </label>
            <label>{{ t('admin.modelId') }}
              <el-input v-model="assistantForm.model" placeholder="grok-4.7" />
            </label>
            <label>{{ t('admin.description') }}
              <el-input v-model="assistantForm.description" />
            </label>
          </div>
          <label class="full">{{ t('admin.systemPrompt') }}
            <el-input v-model="assistantForm.system_prompt" type="textarea" :rows="3" />
          </label>
          <label class="full">{{ t('admin.toolsJson') }}
            <el-input v-model="assistantForm.tools" type="textarea" :rows="2" placeholder='[{"type":"function","function":{...}}]' />
          </label>
          <div class="form-actions">
            <el-switch v-model="assistantForm.enabled" :active-text="t('admin.enabled')" />
            <el-button type="primary" @click="addAssistant">{{ t('admin.add') }}</el-button>
          </div>
        </div>

        <el-table v-if="assistants.length" :data="assistants">
          <el-table-column prop="agent_id" :label="t('admin.assistantId')" min-width="120" />
          <el-table-column prop="name" :label="t('admin.assistantName')" min-width="120" />
          <el-table-column prop="model" :label="t('admin.modelId')" min-width="120" />
          <el-table-column :label="t('admin.enabled')" width="90">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                {{ row.enabled ? t('admin.on') : t('admin.off') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column width="90">
            <template #default="{ row }">
              <el-button size="small" type="danger" plain @click="removeAssistant(row)">{{ t('admin.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- pay -->
      <el-tab-pane :label="t('admin.tabPay')" name="pay">
        <div v-if="pay" class="pay-layout">
          <div class="form-card pay-general">
            <h3>{{ t('admin.payGeneral') }}</h3>
            <div class="grid">
              <label>{{ t('admin.payRate') }}
                <el-input-number v-model="pay.cny_per_usd" :min="0" :max="100" :precision="2" :step="0.1" style="width: 100%" />
              </label>
              <label>{{ t('admin.payPublicUrl') }}
                <el-input v-model="pay.public_url" placeholder="https://modelhub.example.com" />
              </label>
            </div>
          </div>

          <div class="pay-channels">
            <div v-for="id in PAY_CHANNEL_IDS" :key="id" class="form-card pay-channel">
              <div class="channel-head">
                <h3>{{ t(`admin.payChannel.${id}`) }}</h3>
                <el-switch v-model="payForms[id].enabled" />
              </div>
              <p class="status" :class="pay.channels[id]?.status.ok ? 'ok' : 'bad'">
                {{ payStatusText(pay.channels[id]?.status ?? { ok: false, error: '' }) }}
              </p>
              <div class="grid">
                <label
                  v-for="field in PAY_CHANNEL_FIELDS[id]"
                  :key="field.name"
                  :class="{ full: field.name === 'private_key' || field.name === 'public_key' || field.name === 'platform_key' }"
                >
                  {{ t(`admin.payField.${field.name}`) }}
                  <el-input
                    v-model="payForms[id].config[field.name]"
                    :type="field.secret ? 'password' : 'text'"
                    :show-password="field.secret"
                    :placeholder="field.secret && payForms[id].config[field.name] === '********' ? t('admin.payMasked') : ''"
                    class="wide-field"
                  />
                </label>
              </div>
            </div>
          </div>

          <div class="form-actions">
            <el-button type="primary" @click="savePay">{{ t('admin.savePay') }}</el-button>
          </div>
        </div>
        <p v-else class="muted">{{ t('admin.loading') }}</p>
      </el-tab-pane>

      <!-- usage -->
      <el-tab-pane :label="t('admin.tabUsage')" name="usage">
        <div class="stats">
          <div class="stat">
            <div class="stat-num">{{ summary?.requests ?? '—' }}</div>
            <div class="stat-label">{{ t('admin.statRequests') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ summary?.prompt_tokens ?? '—' }}</div>
            <div class="stat-label">{{ t('admin.statPrompt') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ summary?.completion_tokens ?? '—' }}</div>
            <div class="stat-label">{{ t('admin.statCompletion') }}</div>
          </div>
          <div class="stat">
            <div class="stat-num">{{ summary ? formatUsd(summary.cost_micro, summary.cost_usd) : '—' }}</div>
            <div class="stat-label">{{ t('admin.statCost') }}</div>
          </div>
        </div>
        <el-table v-if="usage.length" :data="usage">
          <el-table-column prop="created_at" :label="t('admin.createdAt')" width="180" />
          <el-table-column prop="model" :label="t('admin.modelId')" min-width="140" />
          <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
          <el-table-column prop="prompt_tokens" label="in" width="90" />
          <el-table-column prop="completion_tokens" label="out" width="90" />
          <el-table-column :label="t('admin.cost')" width="120">
            <template #default="{ row }">${{ row.cost_usd.toFixed(6) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.status')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>

      <!-- recharges -->
      <el-tab-pane :label="t('admin.tabRecharges')" name="recharges">
        <el-table v-if="recharges.length" :data="recharges">
          <el-table-column prop="order_no" :label="t('admin.orderNo')" min-width="200" />
          <el-table-column prop="method" :label="t('admin.payMethod')" width="110" />
          <el-table-column :label="t('admin.amount')" width="120">
            <template #default="{ row }">¥{{ row.amount_yuan.toFixed(2) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.creditAmount')" width="130">
            <template #default="{ row }">{{ formatUsd(row.credit_micro, row.credit_usd) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.status')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'paid' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" size="small">
                {{ t(`admin.rechargeStatus.${row.status}`) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" :label="t('admin.createdAt')" width="180" />
        </el-table>
        <p v-else class="muted">{{ t('admin.empty') }}</p>
      </el-tab-pane>
    </el-tabs>

    <div v-else class="card">
      <p class="muted">{{ t('admin.needKey') }}</p>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
.key-card {
  margin-bottom: 16px;
}
.key-label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.hint {
  margin: 10px 0 0;
  font-size: 12px;
}
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.stat {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 18px;
  text-align: center;
}
.stat-num {
  font-size: 24px;
  font-weight: 700;
  color: var(--accent);
}
.stat-label {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-dim);
}
.form-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 16px;
  margin-bottom: 16px;
}
.form-card h3 {
  margin: 0 0 12px;
  font-size: 15px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}
.grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.grid label.full,
.full {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.grid label.switch-label {
  align-items: flex-start;
}
.form-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 4px;
}
.cap-tag {
  margin-right: 4px;
}
.muted {
  color: var(--text-dim);
}
.wide-field {
  width: 100%;
}
.pay-layout {
  display: flex;
  flex-direction: column;
}
.pay-general {
  max-width: 640px;
}
.pay-channels {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 14px;
  margin-bottom: 14px;
}
.pay-channel .channel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.pay-channel .channel-head h3 {
  margin: 0 0 6px;
}
.status {
  margin: 0 0 10px;
  font-size: 12px;
}
.status.ok {
  color: #34d399;
}
.status.bad {
  color: #f87171;
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as echarts from 'echarts'
import {
  adminCreditOrg,
  adminListOrgs,
  adminRecharges,
  createAdminModel,
  createAgent,
  createAssistant,
  createChannel,
  createExternalPage,
  createKey,
  createUser,
  creditUser,
  deleteAdminModel,
  deleteAgent,
  deleteAssistant,
  deleteChannel,
  deleteExternalPage,
  deleteKey,
  formatUsd,
  getMasterKey,
  getPayConfig,
  listAdminModels,
  listAgents,
  listAssistants,
  listChannels,
  listExternalPages,
  listKeys,
  listUsers,
  listUsage,
  putPayConfig,
  setMasterKey,
  testChannel,
  updateAgentRate,
  updateAssistant,
  updateChannel,
  updateExternalPage,
  updateKey,
  updateModel,
  updateUser,
  usageDaily,
  usageSummary,
  userLedger,
  PAY_CHANNEL_FIELDS,
  PAY_CHANNEL_IDS,
  type AdminAgent,
  type AdminAssistant,
  type AdminChannel,
  type AdminKey,
  type AdminModel,
  type AdminOrg,
  type AdminUser,
  type ChannelTestResult,
  type ExternalPage,
  type LedgerEntry,
  type PayConfigView,
  type Recharge,
  type UsageDailyPoint,
  type UsageRecord,
  type UsageSummary,
} from '../api/client'

const { t } = useI18n()

type TabName =
  | 'overview'
  | 'external'
  | 'models'
  | 'channels'
  | 'users'
  | 'keys'
  | 'agents'
  | 'orgs'
  | 'assistants'
  | 'pay'
  | 'usage'
  | 'recharges'

const masterKey = ref(getMasterKey())
const active = ref<TabName>('overview')
const loaded = reactive<Record<TabName, boolean>>({
  overview: false,
  external: false,
  models: false,
  channels: false,
  users: false,
  keys: false,
  agents: false,
  orgs: false,
  assistants: false,
  pay: false,
  usage: false,
  recharges: false,
})
const search = ref('')
const saving = ref(false)

const NAV: { section: string; items: { key: TabName; icon: string; label: string }[] }[] = [
  {
    section: 'admin.sideGeneral',
    items: [
      { key: 'overview', icon: '📊', label: 'admin.tabOverview' },
      { key: 'external', icon: '🔗', label: 'admin.tabExternal' },
    ],
  },
  {
    section: 'admin.sideResources',
    items: [
      { key: 'models', icon: '🧠', label: 'admin.tabModels' },
      { key: 'channels', icon: '🔌', label: 'admin.tabChannels' },
    ],
  },
  {
    section: 'admin.sideAccounts',
    items: [
      { key: 'users', icon: '👥', label: 'admin.tabUsers' },
      { key: 'keys', icon: '🔑', label: 'admin.tabKeys' },
      { key: 'agents', icon: '🏪', label: 'admin.tabAgents' },
      { key: 'orgs', icon: '🏢', label: 'admin.tabOrgs' },
    ],
  },
  { section: 'admin.sideContent', items: [{ key: 'assistants', icon: '🤖', label: 'admin.tabAssistants' }] },
  {
    section: 'admin.sideFinance',
    items: [
      { key: 'pay', icon: '💳', label: 'admin.tabPay' },
      { key: 'recharges', icon: '🧾', label: 'admin.tabRecharges' },
    ],
  },
  { section: 'admin.sideAnalytics', items: [{ key: 'usage', icon: '📈', label: 'admin.tabUsage' }] },
]

const PAGE: Record<TabName, { title: string; desc: string }> = {
  overview: { title: 'admin.pageOverview', desc: 'admin.pageOverviewDesc' },
  external: { title: 'admin.pageExternal', desc: 'admin.pageExternalDesc' },
  models: { title: 'admin.pageModels', desc: 'admin.pageModelsDesc' },
  channels: { title: 'admin.pageChannels', desc: 'admin.pageChannelsDesc' },
  users: { title: 'admin.pageUsers', desc: 'admin.pageUsersDesc' },
  keys: { title: 'admin.pageKeys', desc: 'admin.pageKeysDesc' },
  agents: { title: 'admin.pageAgents', desc: 'admin.pageAgentsDesc' },
  orgs: { title: 'admin.pageOrgs', desc: 'admin.pageOrgsDesc' },
  assistants: { title: 'admin.pageAssistants', desc: 'admin.pageAssistantsDesc' },
  pay: { title: 'admin.pagePay', desc: 'admin.pagePayDesc' },
  usage: { title: 'admin.pageUsage', desc: 'admin.pageUsageDesc' },
  recharges: { title: 'admin.pageRecharges', desc: 'admin.pageRechargesDesc' },
}

const ADD_BTN: Record<TabName, string> = {
  overview: '',
  external: 'admin.extAdd',
  models: 'admin.modelAdd',
  channels: 'admin.channelAdd',
  users: 'admin.userAdd',
  keys: 'admin.keyAdd',
  agents: 'admin.agentAdd',
  orgs: '',
  assistants: 'admin.assistantAdd',
  pay: '',
  usage: '',
  recharges: '',
}

function onKeyChange() {
  const k = masterKey.value.trim()
  setMasterKey(k)
  if (!k) {
    for (const key of Object.keys(loaded) as TabName[]) loaded[key] = false
    return
  }
  loaded.overview = false
  void loadTab('overview')
}

function clearKey() {
  masterKey.value = ''
  setMasterKey('')
  for (const key of Object.keys(loaded) as TabName[]) loaded[key] = false
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

function go(key: TabName) {
  active.value = key
  search.value = ''
  if (!loaded[key]) void loadTab(key)
}

function reload() {
  loaded[active.value] = false
  void loadTab(active.value)
}

function openAdd() {
  const map: Partial<Record<TabName, DialogKind>> = {
    external: 'ext',
    models: 'model',
    channels: 'channel',
    users: 'user',
    keys: 'key',
    agents: 'agent',
    assistants: 'assistant',
  }
  const kind = map[active.value]
  if (kind) openNew(kind)
}

async function loadTab(name: TabName) {
  if (!masterKey.value) return
  try {
    if (name === 'overview') {
      await Promise.allSettled([
        loadModels(true),
        loadChannels(true),
        loadUsers(true),
        loadKeys(),
        loadAgents(),
        loadOrgs(),
        loadSummary(),
        loadUsage(200),
        loadDaily(),
      ])
      if (active.value === 'overview') void nextTick(renderChart)
    } else if (name === 'models') await loadModels()
    else if (name === 'channels') await loadChannels()
    else if (name === 'users') await loadUsers()
    else if (name === 'keys') await loadKeys()
    else if (name === 'agents') await loadAgents()
    else if (name === 'orgs') await loadOrgs()
    else if (name === 'assistants') await loadAssistants()
    else if (name === 'external') await loadExternal()
    else if (name === 'pay') await loadPay()
    else if (name === 'usage') await Promise.allSettled([loadSummary(), loadUsage(200)])
    else if (name === 'recharges') await loadRecharges()
    loaded[name] = true
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

onMounted(() => {
  if (masterKey.value) void loadTab('overview')
  window.addEventListener('resize', onChartResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onChartResize)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})

// ---------- data ----------
const summary = ref<UsageSummary | null>(null)
const models = ref<AdminModel[]>([])
const channels = ref<AdminChannel[]>([])
const users = ref<AdminUser[]>([])
const keys = ref<AdminKey[]>([])
const agents = ref<AdminAgent[]>([])
const orgs = ref<AdminOrg[]>([])
const assistants = ref<AdminAssistant[]>([])
const exts = ref<ExternalPage[]>([])
const usage = ref<UsageRecord[]>([])
const recharges = ref<Recharge[]>([])
const pay = ref<PayConfigView | null>(null)

async function loadSummary() {
  summary.value = await usageSummary()
}
async function loadModels(_silent = false) {
  models.value = await listAdminModels()
}
async function loadChannels(_silent = false) {
  channels.value = await listChannels()
}
async function loadUsers(_silent = false) {
  users.value = await listUsers()
}
async function loadKeys() {
  keys.value = await listKeys()
}
async function loadAgents() {
  agents.value = await listAgents()
}
async function loadOrgs() {
  orgs.value = await adminListOrgs(100)
}
async function loadAssistants() {
  assistants.value = await listAssistants()
}
async function loadExternal() {
  exts.value = await listExternalPages()
}
async function loadUsage(limit = 200) {
  usage.value = await listUsage(undefined, limit)
}
async function loadRecharges() {
  recharges.value = await adminRecharges(100)
}
const daily = ref<UsageDailyPoint[]>([])
async function loadDaily(days = 14) {
  daily.value = await usageDaily(days)
  if (active.value === 'overview') void nextTick(renderChart)
}

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
    await putPayConfig(body)
    ElMessage.success(t('admin.saved'))
    await loadPay()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- dashboard ----------
const chartRef = ref<HTMLElement | null>(null)
let chartInstance: echarts.ECharts | null = null
const days = ref(14)

function onChartResize() {
  chartInstance?.resize()
}

function renderChart() {
  if (active.value !== 'overview' || !chartRef.value) return
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }
  const rows = daily.value
  chartInstance.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    legend: {
      data: [t('admin.chartRequests'), t('admin.chartCost')],
      textStyle: { color: '#9aa4b2' },
      top: 0,
    },
    grid: { left: 48, right: 58, top: 34, bottom: 26 },
    xAxis: {
      type: 'category',
      data: rows.map((d) => d.date.slice(5)),
      axisLine: { lineStyle: { color: 'rgba(128,128,128,0.35)' } },
      axisLabel: { color: '#9aa4b2' },
    },
    yAxis: [
      {
        type: 'value',
        name: t('admin.chartRequests'),
        axisLabel: { color: '#9aa4b2' },
        splitLine: { lineStyle: { color: 'rgba(128,128,128,0.12)' } },
      },
      {
        type: 'value',
        name: 'USD',
        axisLabel: { color: '#9aa4b2' },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: t('admin.chartRequests'),
        type: 'bar',
        data: rows.map((d) => d.requests),
        itemStyle: { color: 'rgba(64,158,255,0.75)', borderRadius: [4, 4, 0, 0] },
        barMaxWidth: 22,
      },
      {
        name: t('admin.chartCost'),
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        data: rows.map((d) => Number(d.cost_usd.toFixed(6))),
        itemStyle: { color: '#f5a623' },
        lineStyle: { width: 2 },
      },
    ],
  })
  chartInstance.resize()
}

function setDays(n: number) {
  days.value = n
  void loadDaily(n)
}

// ---------- client-side search ----------
function filterRows<T extends object>(rows: T[], fields: (keyof T)[]): T[] {
  const s = search.value.trim().toLowerCase()
  if (!s) return rows
  return rows.filter((r) => fields.some((f) => String(r[f] ?? '').toLowerCase().includes(s)))
}
const modelsF = computed(() => filterRows(models.value, ['model_id', 'provider', 'upstream_model']))
const channelsF = computed(() => filterRows(channels.value, ['name', 'provider', 'base_url', 'model_id']))
const usersF = computed(() => filterRows(users.value, ['email']))
const keysF = computed(() => filterRows(keys.value, ['name']))
const agentsF = computed(() => filterRows(agents.value, ['email']))
const orgsF = computed(() => filterRows(orgs.value, ['name', 'owner_email']))
const assistantsF = computed(() => filterRows(assistants.value, ['agent_id', 'name', 'model']))
const extsF = computed(() => filterRows(exts.value, ['name', 'url']))
const rechargesF = computed(() => filterRows(recharges.value, ['order_no', 'method', 'status']))
const usageStatusFilter = ref('')
const usageF = computed(() => {
  const rows = filterRows(usage.value, ['model', 'provider', 'status'])
  const s = usageStatusFilter.value
  return s ? rows.filter((r) => r.status === s) : rows
})

// ---------- dialogs ----------
type DialogKind =
  | ''
  | 'ext'
  | 'model'
  | 'modelEdit'
  | 'channel'
  | 'channelEdit'
  | 'user'
  | 'key'
  | 'keyEdit'
  | 'agent'
  | 'assistant'
  | 'ledger'
const dialog = ref<DialogKind>('')
const dialogVisible = ref(false)
const editingKey = ref<AdminKey | null>(null)
const editingAssistant = ref<AdminAssistant | null>(null)
const editingExt = ref<ExternalPage | null>(null)
const editingModel = ref<AdminModel | null>(null)
const editingChannelId = ref<number | null>(null)
const ledgerUser = ref<AdminUser | null>(null)
const ledger = ref<LedgerEntry[]>([])

const modelForm = reactive({
  model_id: '',
  provider: '',
  upstream_model: '',
  capabilities: [] as string[],
  price_unit: 'token',
  input_price_per_1k: 0,
  output_price_per_1k: 0,
  unit_price: 0,
  enabled: true,
})
const channelForm = reactive({
  name: '',
  provider: '',
  base_url: '',
  api_key: '',
  model_id: '',
  priority: 0,
  enabled: true,
})
const userForm = reactive({ email: '', balance: 0 })
const keyForm = reactive({ name: '' })
const keyEditForm = reactive({ name: '', quota: '', allowed: '', expires: '' })
const agentForm = reactive({ user_id: '', rate: 1 })
const assistantForm = reactive({ agent_id: '', name: '', description: '', model: '', system_prompt: '', tools: '', enabled: true })
const extForm = reactive({ name: '', url: '', sort: 0, enabled: true })
const previewExt = ref<ExternalPage | null>(null)
const CAPS = ['text', 'image', 'video', 'music', 'tts']

function openNew(kind: DialogKind) {
  if (kind === 'ext') {
    Object.assign(extForm, { name: '', url: '', sort: 0, enabled: true })
    editingExt.value = null
  } else if (kind === 'model') {
    Object.assign(modelForm, {
      model_id: '',
      provider: '',
      upstream_model: '',
      capabilities: [],
      price_unit: 'token',
      input_price_per_1k: 0,
      output_price_per_1k: 0,
      unit_price: 0,
      enabled: true,
    })
  } else if (kind === 'channel') {
    Object.assign(channelForm, { name: '', provider: '', base_url: '', api_key: '', model_id: '', priority: 0, enabled: true })
  } else if (kind === 'modelEdit') {
    if (!editingModel.value) return
    const m = editingModel.value
    Object.assign(modelForm, {
      model_id: m.model_id,
      provider: m.provider,
      upstream_model: m.upstream_model,
      capabilities: [...(m.capabilities || [])],
      price_unit: m.price_unit || 'token',
      input_price_per_1k: m.input_price_per_1k ?? 0,
      output_price_per_1k: m.output_price_per_1k ?? 0,
      unit_price: m.unit_price ?? 0,
      enabled: m.enabled,
    })
  } else if (kind === 'channelEdit') {
    const id = editingChannelId.value
    if (id == null) return
    const ch = channels.value.find((c) => c.id === id)
    if (!ch) return
    Object.assign(channelForm, {
      name: ch.name,
      provider: ch.provider,
      base_url: ch.base_url,
      api_key: '',
      model_id: ch.model_id,
      priority: ch.priority,
      enabled: ch.enabled,
    })
  } else if (kind === 'user') {
    Object.assign(userForm, { email: '', balance: 0 })
  } else if (kind === 'key') {
    keyForm.name = ''
  } else if (kind === 'agent') {
    Object.assign(agentForm, { user_id: '', rate: 1 })
  } else if (kind === 'assistant') {
    Object.assign(assistantForm, { agent_id: '', name: '', description: '', model: '', system_prompt: '', tools: '', enabled: true })
    editingAssistant.value = null
  }
  dialog.value = kind
  dialogVisible.value = true
}

function openKeyEdit(k: AdminKey) {
  editingKey.value = k
  keyEditForm.name = k.name
  keyEditForm.quota = k.quota_usd != null ? String(k.quota_usd) : ''
  keyEditForm.allowed = (k.allowed_models || []).join(', ')
  keyEditForm.expires = k.expires_at ?? ''
  dialog.value = 'keyEdit'
  dialogVisible.value = true
}

function openModelEdit(m: AdminModel) {
  editingModel.value = m
  openNew('modelEdit')
}

function openChannelEdit(ch: AdminChannel) {
  editingChannelId.value = ch.id
  openNew('channelEdit')
}

function openAssistantEdit(a: AdminAssistant) {
  editingAssistant.value = a
  Object.assign(assistantForm, {
    agent_id: a.agent_id,
    name: a.name,
    description: a.description,
    model: a.model,
    system_prompt: a.system_prompt,
    tools: Array.isArray(a.tools) ? JSON.stringify(a.tools) : '',
    enabled: a.enabled,
  })
  dialog.value = 'assistant'
  dialogVisible.value = true
}

function openExtEdit(p: ExternalPage) {
  editingExt.value = p
  Object.assign(extForm, { name: p.name, url: p.url, sort: p.sort_order, enabled: p.enabled })
  dialog.value = 'ext'
  dialogVisible.value = true
}

function openExtPreview(p: ExternalPage) {
  previewExt.value = p
}

function closeExtPreview() {
  previewExt.value = null
}

function openLedger(u: AdminUser) {
  ledgerUser.value = u
  ledger.value = []
  dialog.value = 'ledger'
  dialogVisible.value = true
  userLedger(u.id, 100)
    .then((r) => {
      ledger.value = r
    })
    .catch((err) => {
      ElMessage.error(errMsg(err))
    })
}

const dialogTitle = computed(() => {
  switch (dialog.value) {
    case 'ext':
      return editingExt.value ? t('admin.extEdit') : t('admin.extAdd')
    case 'model':
      return t('admin.modelAdd')
    case 'modelEdit':
      return t('admin.modelEdit')
    case 'channel':
      return t('admin.channelAdd')
    case 'channelEdit':
      return t('admin.channelEdit')
    case 'user':
      return t('admin.userAdd')
    case 'key':
      return t('admin.keyAdd')
    case 'keyEdit':
      return t('admin.keyEdit')
    case 'agent':
      return t('admin.agentAdd')
    case 'assistant':
      return editingAssistant.value ? t('admin.assistantEdit') : t('admin.assistantAdd')
    default:
      return ''
  }
})

async function saveDialog() {
  saving.value = true
  try {
    let ok = false
    if (dialog.value === 'ext') ok = await saveExt()
    else if (dialog.value === 'model') ok = await saveModel()
    else if (dialog.value === 'modelEdit') ok = await saveModelEdit()
    else if (dialog.value === 'channel') ok = await saveChannel()
    else if (dialog.value === 'channelEdit') ok = await saveChannelEdit()
    else if (dialog.value === 'user') ok = await saveUser()
    else if (dialog.value === 'key') ok = await saveKey()
    else if (dialog.value === 'keyEdit') ok = await saveKeyEdit()
    else if (dialog.value === 'agent') ok = await saveAgent()
    else if (dialog.value === 'assistant') ok = await saveAssistant()
    if (ok) dialogVisible.value = false
  } catch (err) {
    ElMessage.error(errMsg(err))
  } finally {
    saving.value = false
  }
}

async function saveModel(): Promise<boolean> {
  if (!modelForm.model_id || !modelForm.provider || !modelForm.upstream_model) {
    ElMessage.warning(t('admin.modelRequired'))
    return false
  }
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
  await loadModels()
  return true
}
async function saveChannel(): Promise<boolean> {
  if (!channelForm.name || !channelForm.provider || !channelForm.base_url || !channelForm.api_key || !channelForm.model_id) {
    ElMessage.warning(t('admin.channelRequired'))
    return false
  }
  await createChannel({
    name: channelForm.name.trim(),
    provider: channelForm.provider,
    base_url: channelForm.base_url,
    api_key: channelForm.api_key,
    model_id: channelForm.model_id,
    priority: channelForm.priority,
  })
  ElMessage.success(t('admin.saved'))
  await loadChannels()
  return true
}
async function saveModelEdit(): Promise<boolean> {
  const m = editingModel.value
  if (!m) return false
  await updateModel(m.model_id, {
    upstream_model: modelForm.upstream_model.trim(),
    capabilities: modelForm.capabilities,
    input_price_per_1k: modelForm.input_price_per_1k,
    output_price_per_1k: modelForm.output_price_per_1k,
    price_unit: modelForm.price_unit,
    unit_price: modelForm.price_unit === 'token' ? undefined : modelForm.unit_price,
    enabled: modelForm.enabled,
  })
  ElMessage.success(t('admin.saved'))
  await loadModels()
  return true
}
async function saveChannelEdit(): Promise<boolean> {
  const id = editingChannelId.value
  if (id == null) return false
  if (!channelForm.name || !channelForm.provider || !channelForm.base_url || !channelForm.model_id) {
    ElMessage.warning(t('admin.channelRequired'))
    return false
  }
  const patch: {
    name?: string
    provider?: string
    base_url?: string
    api_key?: string
    model_id?: string
    priority?: number
    enabled?: boolean
  } = {
    name: channelForm.name.trim(),
    provider: channelForm.provider,
    base_url: channelForm.base_url,
    model_id: channelForm.model_id,
    priority: channelForm.priority,
    enabled: channelForm.enabled,
  }
  if (channelForm.api_key.trim()) patch.api_key = channelForm.api_key.trim()
  await updateChannel(id, patch)
  ElMessage.success(t('admin.saved'))
  await loadChannels()
  return true
}
async function saveUser(): Promise<boolean> {
  if (!userForm.email.trim()) {
    ElMessage.warning(t('admin.userRequired'))
    return false
  }
  await createUser(userForm.email.trim(), userForm.balance)
  ElMessage.success(t('admin.saved'))
  await loadUsers()
  return true
}
async function saveKey(): Promise<boolean> {
  if (!keyForm.name.trim()) {
    ElMessage.warning(t('admin.keyRequired'))
    return false
  }
  const res = await createKey(keyForm.name.trim())
  ElMessageBox.alert(res.key, t('admin.keyCreated'), { confirmButtonText: 'OK' })
  await loadKeys()
  return true
}
async function saveKeyEdit(): Promise<boolean> {
  const k = editingKey.value
  if (!k) return false
  if (!keyEditForm.name.trim()) {
    ElMessage.warning(t('admin.keyRequired'))
    return false
  }
  const patch: { name?: string; quota_usd?: number; allowed_models?: string[]; expires_at?: string } = {
    name: keyEditForm.name.trim(),
  }
  if (keyEditForm.quota.trim()) patch.quota_usd = Number(keyEditForm.quota)
  if (keyEditForm.allowed.trim()) {
    patch.allowed_models = keyEditForm.allowed
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
  }
  if (keyEditForm.expires.trim()) patch.expires_at = keyEditForm.expires.trim()
  await updateKey(k.id, patch)
  ElMessage.success(t('admin.saved'))
  await loadKeys()
  return true
}
async function saveAgent(): Promise<boolean> {
  if (!agentForm.user_id || agentForm.rate < 1) {
    ElMessage.warning(t('admin.agentRequired'))
    return false
  }
  await createAgent(Number(agentForm.user_id), agentForm.rate)
  ElMessage.success(t('admin.saved'))
  await loadAgents()
  return true
}
async function saveAssistant(): Promise<boolean> {
  const editing = editingAssistant.value
  if (!assistantForm.name || !assistantForm.system_prompt || !assistantForm.model) {
    ElMessage.warning(t('admin.assistantRequired'))
    return false
  }
  if (editing) {
    await updateAssistant(editing.id, {
      name: assistantForm.name.trim(),
      description: assistantForm.description.trim(),
      system_prompt: assistantForm.system_prompt,
      model: assistantForm.model.trim(),
      tools: assistantForm.tools || undefined,
      enabled: assistantForm.enabled,
    })
  } else {
    if (!assistantForm.agent_id.trim()) {
      ElMessage.warning(t('admin.assistantRequired'))
      return false
    }
    await createAssistant({
      agent_id: assistantForm.agent_id.trim(),
      name: assistantForm.name.trim(),
      description: assistantForm.description.trim(),
      system_prompt: assistantForm.system_prompt,
      model: assistantForm.model.trim(),
      tools: assistantForm.tools || undefined,
      enabled: assistantForm.enabled,
    })
  }
  ElMessage.success(t('admin.saved'))
  await loadAssistants()
  return true
}

async function saveExt(): Promise<boolean> {
  const editing = editingExt.value
  if (!extForm.name.trim() || !extForm.url.trim()) {
    ElMessage.warning(t('admin.extRequired'))
    return false
  }
  const body = {
    name: extForm.name.trim(),
    url: extForm.url.trim(),
    sort_order: extForm.sort,
    enabled: extForm.enabled,
  }
  if (editing) {
    await updateExternalPage(editing.id, body)
  } else {
    await createExternalPage(body)
  }
  ElMessage.success(t('admin.saved'))
  await loadExternal()
  return true
}

// ---------- row actions ----------
async function confirmDelete(message: string): Promise<boolean> {
  try {
    await ElMessageBox.confirm(message, t('admin.delete'), { type: 'warning' })
    return true
  } catch {
    return false
  }
}
async function removeModel(m: AdminModel) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: m.model_id })))) return
  try {
    await deleteAdminModel(m.model_id)
    await loadModels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeChannel(ch: AdminChannel) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: ch.name })))) return
  try {
    await deleteChannel(ch.id)
    await loadChannels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeKey(k: AdminKey) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: k.name })))) return
  try {
    await deleteKey(k.id)
    await loadKeys()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeAgent(a: AdminAgent) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: a.email })))) return
  try {
    await deleteAgent(a.id)
    await loadAgents()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeAssistant(a: AdminAssistant) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: a.agent_id })))) return
  try {
    await deleteAssistant(a.id)
    await loadAssistants()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function removeExt(p: ExternalPage) {
  if (!(await confirmDelete(t('admin.confirmDelete', { name: p.name })))) return
  try {
    await deleteExternalPage(p.id)
    if (previewExt.value?.id === p.id) previewExt.value = null
    await loadExternal()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function toggleExt(p: ExternalPage) {
  try {
    await updateExternalPage(p.id, { enabled: !p.enabled })
    await loadExternal()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}

// ---------- channel / model / user toggles & tests ----------
const testState = reactive<Record<number, { busy: boolean; result?: ChannelTestResult }>>({})
function testOf(id: number): { busy: boolean; result?: ChannelTestResult } {
  if (!testState[id]) testState[id] = { busy: false }
  return testState[id]
}
function testResult(id: number): ChannelTestResult | undefined {
  return testState[id]?.result
}
async function runTest(ch: AdminChannel) {
  const st = testOf(ch.id)
  st.busy = true
  st.result = undefined
  try {
    st.result = await testChannel(ch.id)
    if (st.result.ok) {
      ElMessage.success(t('admin.testOk', { ms: st.result.latency_ms ?? 0, status: st.result.status ?? '' }))
    } else {
      ElMessage.error(st.result.error || t('admin.testFailed'))
    }
    await loadChannels(true)
  } catch (err) {
    st.result = { ok: false, error: errMsg(err) }
    ElMessage.error(errMsg(err))
  } finally {
    st.busy = false
  }
}
async function toggleChannel(ch: AdminChannel) {
  try {
    await updateChannel(ch.id, { enabled: !ch.enabled })
    await loadChannels(true)
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function toggleModel(m: AdminModel) {
  try {
    await updateModel(m.model_id, { enabled: !m.enabled })
    await loadModels()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function toggleUser(u: AdminUser) {
  try {
    await updateUser(u.id, !u.enabled)
    await loadUsers()
  } catch (err) {
    ElMessage.error(errMsg(err))
  }
}
async function creditUserRow(u: AdminUser) {
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
async function creditOrgRow(o: AdminOrg) {
  const { value } = await ElMessageBox.prompt(t('admin.creditPrompt'), t('admin.credit'), {
    inputPattern: /^\d+(\.\d+)?$/,
    inputErrorMessage: t('admin.creditInvalid'),
  }).catch(() => ({ value: '' }))
  const amount = Number(value)
  if (!value || Number.isNaN(amount)) return
  try {
    await adminCreditOrg(o.id, amount, 'manual admin credit')
    ElMessage.success(t('admin.saved'))
    await loadOrgs()
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
const payStatusText = (ch: { ok: boolean; error?: string }) =>
  ch.ok ? t('admin.payOk') : ch.error || t('admin.payBroken')
</script>

<template>
  <div class="page admin">
    <!-- ============ master key gate ============ -->
    <div v-if="!masterKey" class="gate">
      <div class="card gate-card">
        <img src="/logo.svg" class="brand-logo" width="48" height="48" alt="KejiAPI" />
        <h1>{{ t('admin.gateTitle') }}</h1>
        <p class="muted">{{ t('admin.gateSub') }}</p>
        <el-input
          v-model="masterKey"
          type="password"
          show-password
          size="large"
          :placeholder="t('admin.masterKeyPlaceholder')"
          @change="onKeyChange"
          @keyup.enter="onKeyChange"
        />
        <p class="muted hint">{{ t('admin.masterKeyHint') }}</p>
        <a class="gate-back" href="/chat">{{ t('admin.backToApp') }} ←</a>
      </div>
    </div>

    <!-- ============ console layout ============ -->
    <div v-else class="layout">
      <aside class="sidebar">
        <div class="side-brand">
          <img src="/logo.svg" class="brand-logo" width="38" height="38" alt="KejiAPI" />
          <div>
            <div class="brand-name">KejiAPI</div>
            <div class="brand-sub">Admin</div>
          </div>
        </div>
        <nav class="side-nav">
          <div v-for="group in NAV" :key="group.section" class="side-group">
            <div class="side-section">{{ t(group.section) }}</div>
            <button
              v-for="item in group.items"
              :key="item.key"
              type="button"
              class="side-item"
              :class="{ active: active === item.key }"
              @click="go(item.key)"
            >
              <span class="side-icon">{{ item.icon }}</span>
              <span>{{ t(item.label) }}</span>
            </button>
          </div>
        </nav>
        <div class="side-foot">
          <button type="button" class="side-clear" @click="clearKey">{{ t('admin.clearKey') }}</button>
        </div>
      </aside>

      <main class="content">
        <div class="page-head">
          <div>
            <h1>{{ t(PAGE[active].title) }}</h1>
            <p class="muted page-desc">{{ t(PAGE[active].desc) }}</p>
          </div>
          <div class="page-actions">
            <el-button v-if="ADD_BTN[active]" type="primary" @click="openAdd">
              {{ t(ADD_BTN[active]) }}
            </el-button>
            <el-input v-model="search" class="search" :placeholder="t('admin.search')" clearable />
            <el-button @click="reload">{{ t('admin.refresh') }}</el-button>
          </div>
        </div>

        <!-- ===== overview ===== -->
        <section v-show="active === 'overview'">
          <div class="stat-grid">
            <div class="stat">
              <div class="stat-num">{{ users.length }}</div>
              <div class="stat-label">👥 {{ t('admin.statUsers') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ models.length }}</div>
              <div class="stat-label">🧠 {{ t('admin.statModels') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ channels.length }}</div>
              <div class="stat-label">🔌 {{ t('admin.statChannels') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ keys.length }}</div>
              <div class="stat-label">🔑 {{ t('admin.statKeys') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ agents.length }}</div>
              <div class="stat-label">🏪 {{ t('admin.statAgents') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ orgs.length }}</div>
              <div class="stat-label">🏢 {{ t('admin.statOrgs') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ summary?.requests ?? '—' }}</div>
              <div class="stat-label">📊 {{ t('admin.statRequests') }}</div>
            </div>
            <div class="stat">
              <div class="stat-num">{{ summary ? formatUsd(summary.cost_micro, summary.cost_usd) : '—' }}</div>
              <div class="stat-label">💰 {{ t('admin.statCost') }}</div>
            </div>
          </div>

          <div class="dash-row">
            <div class="card dash-card">
              <div class="card-head">
                <h3>{{ t('admin.chartTitle') }}</h3>
                <el-select v-model="days" size="small" class="days-select" @change="setDays">
                  <el-option :value="7" :label="t('admin.days7')" />
                  <el-option :value="14" :label="t('admin.days14')" />
                  <el-option :value="30" :label="t('admin.days30')" />
                </el-select>
              </div>
              <div ref="chartRef" class="chart-box"></div>
            </div>
            <div class="card dash-card">
              <h3>{{ t('admin.channelMini') }}</h3>
              <div v-for="ch in channels.slice(0, 8)" :key="ch.id" class="health-row">
                <span class="health-name">
                  {{ ch.name }} <span class="muted">· {{ ch.model_id }}</span>
                </span>
                <el-tag :type="ch.health === 'ok' ? 'success' : 'warning'" size="small">
                  {{ ch.health === 'ok' ? t('admin.healthOk') : t('admin.healthCooldown') }}
                </el-tag>
              </div>
              <p v-if="!channels.length" class="muted">{{ t('admin.empty') }}</p>
            </div>
          </div>

          <div class="card">
            <h3>{{ t('admin.recent') }}</h3>
            <el-table v-if="usage.length" :data="usage.slice(0, 8)" size="small">
              <el-table-column prop="created_at" :label="t('admin.time')" width="170" />
              <el-table-column prop="model" :label="t('admin.modelId')" min-width="130" />
              <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
              <el-table-column :label="t('admin.cost')" width="110">
                <template #default="{ row }">${{ row.cost_usd.toFixed(6) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.status')" width="90">
                <template #default="{ row }">
                  <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== models ===== -->
        <section v-show="active === 'models'">
          <div class="card">
            <el-table v-if="modelsF.length" :data="modelsF">
              <el-table-column prop="model_id" :label="t('admin.modelId')" min-width="150" />
              <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
              <el-table-column prop="upstream_model" :label="t('admin.upstreamModel')" min-width="130" />
              <el-table-column :label="t('admin.capabilities')" min-width="140">
                <template #default="{ row }">
                  <el-tag v-for="c in row.capabilities" :key="c" size="small" class="cap-tag">
                    {{ t(`admin.cap.${c}`) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.price')" width="180">
                <template #default="{ row }">
                  <span v-if="row.price_unit && row.price_unit !== 'token'">
                    ${{ row.unit_price }} / {{ row.price_unit }}
                  </span>
                  <span v-else>{{ row.input_price_per_1k }} → {{ row.output_price_per_1k }} /1k</span>
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.enabled')" width="80">
                <template #default="{ row }">
                  <el-switch :model-value="row.enabled" @change="toggleModel(row)" />
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.actions')" width="150">
                <template #default="{ row }">
                  <el-button size="small" @click="openModelEdit(row)">{{ t('admin.edit') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeModel(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== channels ===== -->
        <section v-show="active === 'channels'">
          <div class="card">
            <el-table v-if="channelsF.length" :data="channelsF">
              <el-table-column prop="name" :label="t('admin.channelName')" min-width="120" />
              <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
              <el-table-column prop="base_url" :label="t('admin.baseUrl')" min-width="170" show-overflow-tooltip />
              <el-table-column prop="model_id" :label="t('admin.modelId')" min-width="120" />
              <el-table-column prop="priority" :label="t('admin.priority')" width="80" />
              <el-table-column :label="t('admin.enabled')" width="80">
                <template #default="{ row }">
                  <el-switch :model-value="row.enabled" @change="toggleChannel(row)" />
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.health')" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.health === 'ok' ? 'success' : 'warning'" size="small">
                    {{ row.health === 'ok' ? t('admin.healthOk') : t('admin.healthCooldown') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.lastTest')" min-width="130">
                <template #default="{ row }">
                  <span v-if="testOf(row.id).busy" class="muted">{{ t('admin.testing') }}</span>
                  <span v-else-if="testResult(row.id)?.ok" class="test-ok">
                    ✓ {{ testResult(row.id)!.status }} · {{ testResult(row.id)!.latency_ms }}ms
                  </span>
                  <el-tooltip v-else-if="testResult(row.id)" :content="testResult(row.id)!.error || ''" placement="top">
                    <span class="test-bad">✗ {{ t('admin.testFailed') }}</span>
                  </el-tooltip>
                  <span v-else class="muted">—</span>
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.actions')" width="220">
                <template #default="{ row }">
                  <el-button size="small" type="primary" plain :loading="testOf(row.id).busy" @click="runTest(row)">
                    {{ t('admin.test') }}
                  </el-button>
                  <el-button size="small" @click="openChannelEdit(row)">{{ t('admin.edit') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeChannel(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== users ===== -->
        <section v-show="active === 'users'">
          <div class="card">
            <el-table v-if="usersF.length" :data="usersF">
              <el-table-column prop="id" :label="t('admin.id')" width="60" />
              <el-table-column prop="email" :label="t('admin.email')" min-width="180" />
              <el-table-column :label="t('admin.balance')" width="120">
                <template #default="{ row }">{{ formatUsd(row.balance_micro, row.balance_usd) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.agentRate')" width="100">
                <template #default="{ row }">{{ row.agent_rate != null ? `×${row.agent_rate}` : '—' }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.enabled')" width="80">
                <template #default="{ row }">
                  <el-switch :model-value="row.enabled" @change="toggleUser(row)" />
                </template>
              </el-table-column>
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
              <el-table-column :label="t('admin.actions')" width="160">
                <template #default="{ row }">
                  <el-button size="small" @click="creditUserRow(row)">{{ t('admin.credit') }}</el-button>
                  <el-button size="small" @click="openLedger(row)">{{ t('admin.ledgerBtn') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== keys ===== -->
        <section v-show="active === 'keys'">
          <div class="card">
            <el-table v-if="keysF.length" :data="keysF">
              <el-table-column prop="id" :label="t('admin.id')" width="60" />
              <el-table-column prop="name" :label="t('admin.keyName')" min-width="140" />
              <el-table-column :label="t('admin.spent')" width="120">
                <template #default="{ row }">{{ formatUsd(row.spend_micro, row.spend_usd) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.allowedModels')" min-width="160" show-overflow-tooltip>
                <template #default="{ row }">
                  <span v-if="(row.allowed_models || []).length">{{ row.allowed_models.join(', ') }}</span>
                  <span v-else class="muted">{{ t('admin.allModels') }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
              <el-table-column :label="t('admin.actions')" width="150">
                <template #default="{ row }">
                  <el-button size="small" @click="openKeyEdit(row)">{{ t('admin.edit') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeKey(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== agents ===== -->
        <section v-show="active === 'agents'">
          <div class="card">
            <el-table v-if="agentsF.length" :data="agentsF">
              <el-table-column prop="id" :label="t('admin.id')" width="60" />
              <el-table-column prop="email" :label="t('admin.email')" min-width="180" />
              <el-table-column prop="rate" :label="t('admin.agentRate')" width="110" />
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
              <el-table-column :label="t('admin.actions')" width="170">
                <template #default="{ row }">
                  <el-button size="small" @click="updateRate(row)">{{ t('admin.setRate') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeAgent(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== orgs ===== -->
        <section v-show="active === 'orgs'">
          <div class="card">
            <el-table v-if="orgsF.length" :data="orgsF">
              <el-table-column prop="id" :label="t('admin.id')" width="60" />
              <el-table-column prop="name" :label="t('admin.orgName')" min-width="140" />
              <el-table-column prop="owner_email" :label="t('admin.orgOwner')" min-width="170" />
              <el-table-column prop="member_count" :label="t('admin.orgMembers')" width="90" />
              <el-table-column :label="t('admin.balance')" width="120">
                <template #default="{ row }">{{ formatUsd(row.balance_micro, row.balance_usd) }}</template>
              </el-table-column>
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
              <el-table-column :label="t('admin.actions')" width="90">
                <template #default="{ row }">
                  <el-button size="small" @click="creditOrgRow(row)">{{ t('admin.credit') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== assistants ===== -->
        <!-- ===== external (P7-6) ===== -->
        <section v-show="active === 'external'">
          <div class="card">
            <el-table v-if="extsF.length" :data="extsF">
              <el-table-column prop="name" :label="t('admin.extName')" min-width="130" />
              <el-table-column prop="url" :label="t('admin.extUrl')" min-width="200" show-overflow-tooltip />
              <el-table-column :label="t('admin.enabled')" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                    {{ row.enabled ? t('admin.on') : t('admin.off') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="sort_order" :label="t('admin.extSort')" width="70" />
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
              <el-table-column :label="t('admin.actions')" width="210">
                <template #default="{ row }">
                  <el-button size="small" @click="openExtEdit(row)">{{ t('admin.edit') }}</el-button>
                  <el-button size="small" @click="openExtPreview(row)">{{ t('admin.extPreview') }}</el-button>
                  <el-button size="small" @click="toggleExt(row)">{{ row.enabled ? t('admin.off') : t('admin.on') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeExt(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>

          <div v-if="previewExt" class="card ext-preview">
            <div class="ext-preview-head">
              <strong>{{ previewExt.name }}</strong>
              <el-button size="small" text @click="closeExtPreview">{{ t('admin.close') }}</el-button>
            </div>
            <iframe :src="previewExt.url" class="ext-frame" sandbox="allow-scripts allow-same-origin allow-forms allow-popups" />
          </div>
        </section>

        <section v-show="active === 'assistants'">
          <div class="card">
            <el-table v-if="assistantsF.length" :data="assistantsF">
              <el-table-column prop="agent_id" :label="t('admin.assistantId')" min-width="120" />
              <el-table-column prop="name" :label="t('admin.assistantName')" min-width="120" />
              <el-table-column prop="model" :label="t('admin.modelId')" min-width="120" />
              <el-table-column prop="description" :label="t('admin.description')" min-width="140" show-overflow-tooltip />
              <el-table-column :label="t('admin.enabled')" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
                    {{ row.enabled ? t('admin.on') : t('admin.off') }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.actions')" width="150">
                <template #default="{ row }">
                  <el-button size="small" @click="openAssistantEdit(row)">{{ t('admin.edit') }}</el-button>
                  <el-button size="small" type="danger" plain @click="removeAssistant(row)">{{ t('admin.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== pay ===== -->
        <section v-show="active === 'pay'">
          <div v-if="pay" class="pay-layout">
            <div class="card pay-general">
              <h3>{{ t('admin.payGeneral') }}</h3>
              <div class="grid">
                <label>{{ t('admin.payRate') }}
                  <el-input-number v-model="pay.cny_per_usd" :min="0" :max="100" :precision="2" :step="0.1" style="width: 100%" />
                </label>
                <label>{{ t('admin.payPublicUrl') }}
                  <el-input v-model="pay.public_url" placeholder="https://kejiapi.example.com" />
                </label>
              </div>
            </div>

            <div class="pay-channels">
              <div v-for="id in PAY_CHANNEL_IDS" :key="id" class="card pay-channel">
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

            <div class="pay-save">
              <el-button type="primary" @click="savePay">{{ t('admin.savePay') }}</el-button>
            </div>
          </div>
          <p v-else class="muted">{{ t('admin.loading') }}</p>
        </section>

        <!-- ===== usage ===== -->
        <section v-show="active === 'usage'">
          <div class="stat-grid stat-grid-sm">
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
          <div class="card">
            <div class="card-head">
              <h3>{{ t('admin.usageTable') }}</h3>
              <el-select v-model="usageStatusFilter" size="small" class="usage-filter">
                <el-option value="" :label="t('admin.filterAll')" />
                <el-option value="ok" :label="t('admin.statusOk')" />
                <el-option value="error" :label="t('admin.statusError')" />
              </el-select>
            </div>
            <el-table v-if="usageF.length" :data="usageF" size="small">
              <el-table-column prop="created_at" :label="t('admin.time')" width="170" />
              <el-table-column prop="model" :label="t('admin.modelId')" min-width="130" />
              <el-table-column prop="provider" :label="t('admin.provider')" width="100" />
              <el-table-column prop="prompt_tokens" label="in" width="80" />
              <el-table-column prop="completion_tokens" label="out" width="80" />
              <el-table-column :label="t('admin.cost')" width="110">
                <template #default="{ row }">${{ row.cost_usd.toFixed(6) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.status')" width="90">
                <template #default="{ row }">
                  <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>

        <!-- ===== recharges ===== -->
        <section v-show="active === 'recharges'">
          <div class="card">
            <el-table v-if="rechargesF.length" :data="rechargesF" size="small">
              <el-table-column prop="order_no" :label="t('admin.orderNo')" min-width="190" show-overflow-tooltip />
              <el-table-column prop="method" :label="t('admin.payMethod')" width="100" />
              <el-table-column :label="t('admin.amount')" width="110">
                <template #default="{ row }">¥{{ row.amount_yuan.toFixed(2) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.creditAmount')" width="120">
                <template #default="{ row }">{{ formatUsd(row.credit_micro, row.credit_usd) }}</template>
              </el-table-column>
              <el-table-column :label="t('admin.status')" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.status === 'paid' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" size="small">
                    {{ t(`admin.rechargeStatus.${row.status}`) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" :label="t('admin.createdAt')" width="170" />
            </el-table>
            <p v-else class="muted">{{ t('admin.empty') }}</p>
          </div>
        </section>
      </main>
    </div>

    <!-- ============ dialogs ============ -->
    <el-dialog v-if="dialog !== 'ledger'" v-model="dialogVisible" :title="dialogTitle" width="620px" destroy-on-close>
      <el-form v-if="dialog === 'ext'" label-position="top">
        <div class="dlg-grid">
          <label>{{ t('admin.extName') }}
            <el-input v-model="extForm.name" placeholder="Ticketing" />
          </label>
          <label>{{ t('admin.extSort') }}
            <el-input-number v-model="extForm.sort" :min="0" :step="1" style="width: 100%" />
          </label>
          <label class="full">{{ t('admin.extUrl') }}
            <el-input v-model="extForm.url" placeholder="https://help.example.com/admin" />
          </label>
        </div>
        <el-switch v-model="extForm.enabled" :active-text="t('admin.enabled')" />
      </el-form>

      <el-form v-else-if="dialog === 'model'" label-position="top">
        <div class="dlg-grid">
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
          <template v-if="modelForm.price_unit === 'token'">
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
      </el-form>

      <el-form v-else-if="dialog === 'modelEdit'" label-position="top">
        <div class="dlg-grid">
          <label class="full">{{ t('admin.modelId') }}
            <el-input :model-value="modelForm.model_id" disabled />
          </label>
          <label class="full">{{ t('admin.upstreamModel') }}
            <el-input v-model="modelForm.upstream_model" placeholder="grok-4.7" />
          </label>
          <label class="full">{{ t('admin.capabilities') }}
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
          <template v-if="modelForm.price_unit === 'token'">
            <label>{{ t('admin.inputPrice') }}
              <el-input-number v-model="modelForm.input_price_per_1k" :min="0" :precision="6" :step="0.001" style="width: 100%" />
            </label>
            <label>{{ t('admin.outputPrice') }}
              <el-input-number v-model="modelForm.output_price_per_1k" :min="0" :precision="6" :step="0.001" style="width: 100%" />
            </label>
          </template>
          <label v-else class="full">{{ t('admin.unitPrice') }}
            <el-input-number v-model="modelForm.unit_price" :min="0" :precision="4" :step="0.01" style="width: 100%" />
          </label>
          <label class="switch-label">{{ t('admin.enabled') }}
            <el-switch v-model="modelForm.enabled" />
          </label>
        </div>
      </el-form>

      <el-form v-else-if="dialog === 'channel'" label-position="top">
        <div class="dlg-grid">
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
      </el-form>

      <el-form v-else-if="dialog === 'channelEdit'" label-position="top">
        <div class="dlg-grid">
          <label>{{ t('admin.channelName') }}
            <el-input v-model="channelForm.name" />
          </label>
          <label>{{ t('admin.provider') }}
            <el-input v-model="channelForm.provider" />
          </label>
          <label class="full">{{ t('admin.baseUrl') }}
            <el-input v-model="channelForm.base_url" />
          </label>
          <label class="full">{{ t('admin.apiKey') }}
            <el-input
              v-model="channelForm.api_key"
              type="password"
              show-password
              :placeholder="t('admin.apiKeyKeep')"
            />
          </label>
          <label>{{ t('admin.modelId') }}
            <el-input v-model="channelForm.model_id" />
          </label>
          <label>{{ t('admin.priority') }}
            <el-input-number v-model="channelForm.priority" :min="0" style="width: 100%" />
          </label>
          <label class="switch-label">{{ t('admin.enabled') }}
            <el-switch v-model="channelForm.enabled" />
          </label>
        </div>
      </el-form>

      <el-form v-else-if="dialog === 'user'" label-position="top">
        <div class="dlg-grid">
          <label>{{ t('admin.email') }}
            <el-input v-model="userForm.email" placeholder="user@example.com" />
          </label>
          <label>{{ t('admin.initialBalance') }}
            <el-input-number v-model="userForm.balance" :min="0" :precision="2" :step="10" style="width: 100%" />
          </label>
        </div>
      </el-form>

      <el-form v-else-if="dialog === 'key'" label-position="top">
        <label>{{ t('admin.keyName') }}
          <el-input v-model="keyForm.name" placeholder="demo-key" />
        </label>
      </el-form>

      <el-form v-else-if="dialog === 'keyEdit'" label-position="top">
        <div class="dlg-grid">
          <label>{{ t('admin.keyName') }}
            <el-input v-model="keyEditForm.name" />
          </label>
          <label>{{ t('admin.quota') }}
            <el-input v-model="keyEditForm.quota" placeholder="100" />
          </label>
          <label class="full">{{ t('admin.allowedModels') }}
            <el-input v-model="keyEditForm.allowed" :placeholder="t('admin.allowedPh')" />
          </label>
          <label class="full">{{ t('admin.expires') }}
            <el-input v-model="keyEditForm.expires" placeholder="2026-12-31T00:00:00Z" />
          </label>
        </div>
      </el-form>

      <el-form v-else-if="dialog === 'agent'" label-position="top">
        <div class="dlg-grid">
          <label>{{ t('admin.agentUserId') }}
            <el-input v-model="agentForm.user_id" placeholder="user id" />
          </label>
          <label>{{ t('admin.agentRate') }}
            <el-input-number v-model="agentForm.rate" :min="1" :precision="2" :step="0.1" style="width: 100%" />
          </label>
        </div>
      </el-form>

      <el-form v-else-if="dialog === 'assistant'" label-position="top">
        <div class="dlg-grid">
          <label v-if="!editingAssistant">{{ t('admin.assistantId') }}
            <el-input v-model="assistantForm.agent_id" placeholder="translator" />
          </label>
          <label>{{ t('admin.assistantName') }}
            <el-input v-model="assistantForm.name" placeholder="Translator" />
          </label>
          <label>{{ t('admin.modelId') }}
            <el-input v-model="assistantForm.model" placeholder="grok-4.7" />
          </label>
          <label class="full">{{ t('admin.description') }}
            <el-input v-model="assistantForm.description" />
          </label>
        </div>
        <label class="dlg-full">{{ t('admin.systemPrompt') }}
          <el-input v-model="assistantForm.system_prompt" type="textarea" :rows="3" />
        </label>
        <label class="dlg-full">{{ t('admin.toolsJson') }}
          <el-input v-model="assistantForm.tools" type="textarea" :rows="2" placeholder='[{"type":"function","function":{...}}]' />
        </label>
        <el-switch v-model="assistantForm.enabled" :active-text="t('admin.enabled')" />
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('admin.close') }}</el-button>
        <el-button type="primary" :loading="saving" @click="saveDialog">{{ t('admin.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-if="dialog === 'ledger'" v-model="dialogVisible" :title="`${t('admin.ledgerBtn')} · ${ledgerUser?.email ?? ''}`" width="680px">
      <el-table v-if="ledger.length" :data="ledger" size="small">
        <el-table-column :label="t('admin.status')" width="90">
          <template #default="{ row }">
            <el-tag size="small">{{ t(`console.ledgerKind.${row.kind}`) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.amountUsd')" width="110">
          <template #default="{ row }">${{ row.amount_usd.toFixed(6) }}</template>
        </el-table-column>
        <el-table-column prop="reason" :label="t('admin.reason')" min-width="140" show-overflow-tooltip />
        <el-table-column prop="created_at" :label="t('admin.time')" width="170" />
      </el-table>
      <p v-else class="muted">{{ t('admin.empty') }}</p>
    </el-dialog>
  </div>
</template>

<style scoped>
.gate {
  max-width: 460px;
  margin: 60px auto;
}
.gate-card {
  text-align: center;
  padding: 40px 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}
.gate-card h1 {
  margin: 0;
  font-size: 22px;
}
.gate-card .muted {
  margin: 0;
  font-size: 13px;
}
.hint {
  font-size: 12px;
}
.brand-logo {
  display: block;
  border-radius: 12px;
}
.brand-name {
  font-weight: 800;
  font-size: 16px;
  background: linear-gradient(90deg, var(--text), var(--accent));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.brand-sub {
  font-size: 11px;
  color: var(--text-dim);
  letter-spacing: 1.5px;
  text-transform: uppercase;
}
.layout {
  display: flex;
  gap: 18px;
  align-items: flex-start;
}
.sidebar {
  width: 230px;
  flex-shrink: 0;
  position: sticky;
  top: 76px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - 110px);
}
.side-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 8px 14px;
}
.side-nav {
  flex: 1;
  overflow-y: auto;
}
.side-group {
  margin-bottom: 8px;
}
.side-section {
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-dim);
  padding: 8px 10px 4px;
}
.side-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  border: none;
  background: transparent;
  color: var(--text-dim);
  font-size: 13.5px;
  font-weight: 500;
  padding: 8px 10px;
  border-radius: 10px;
  cursor: pointer;
  text-align: left;
  transition: all 0.13s ease;
}
.side-item:hover {
  background: var(--bg-hover);
  color: var(--text);
}
.side-item.active {
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent);
}
.side-icon {
  font-size: 15px;
  width: 20px;
  text-align: center;
}
.side-foot {
  border-top: 1px solid var(--border);
  padding-top: 10px;
}
.side-clear {
  width: 100%;
  border: 1px solid var(--border);
  background: transparent;
  color: var(--text-dim);
  font-size: 12px;
  padding: 7px 0;
  border-radius: 10px;
  cursor: pointer;
}
.side-clear:hover {
  color: #f87171;
  border-color: #f87171;
}
.content {
  flex: 1;
  min-width: 0;
}
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.page-head h1 {
  margin: 0;
  font-size: 20px;
}
.page-desc {
  margin: 4px 0 0;
  font-size: 12.5px;
}
.page-actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
.search {
  width: 220px;
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.stat-grid-sm {
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
}
.stat {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 16px 18px;
  box-shadow: var(--shadow);
}
.stat-num {
  font-size: 22px;
  font-weight: 700;
  color: var(--accent);
}
.stat-label {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-dim);
}
.dash-row {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
  margin-bottom: 16px;
}
@media (max-width: 900px) {
  .layout {
    flex-direction: column;
  }
  .sidebar {
    position: static;
    width: 100%;
    min-height: auto;
  }
  .dash-row {
    grid-template-columns: 1fr;
  }
}
.dash-card {
  margin: 0;
}
.dash-card h3 {
  margin: 0 0 12px;
  font-size: 14px;
}
.chart-box {
  width: 100%;
  height: 280px;
  min-height: 280px;
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}
.card-head h3 {
  margin: 0;
}
.days-select {
  width: 104px;
}
.usage-filter {
  width: 130px;
}
.test-ok {
  color: #34d399;
  font-size: 12px;
  white-space: nowrap;
}
.test-bad {
  color: #f87171;
  font-size: 12px;
  cursor: help;
}
.gate-back {
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-dim);
  text-decoration: none;
}
.gate-back:hover {
  color: var(--accent);
}
.health-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 0;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}
.health-row:last-child {
  border-bottom: none;
}
.health-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card h3 {
  margin: 0 0 12px;
  font-size: 14px;
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
.dlg-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.dlg-grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.dlg-grid label.full {
  grid-column: 1 / -1;
}
.dlg-grid label.switch-label {
  align-items: flex-start;
}
.dlg-full {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
  margin-top: 12px;
}
.pay-layout {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.pay-general h3,
.pay-channel h3 {
  margin: 0 0 12px;
  font-size: 14px;
}
.pay-channels {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 14px;
}
.channel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
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
.pay-save {
  display: flex;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
}
.grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.grid label.full {
  grid-column: 1 / -1;
}
.ext-preview {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 14px;
}
.ext-preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
.ext-frame {
  width: 100%;
  height: 70vh;
  min-height: 380px;
  border: 1px solid var(--border, #2a2f3a);
  border-radius: 10px;
  background: #0b0e14;
}
</style>

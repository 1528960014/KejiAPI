import axios from 'axios'

const http = axios.create({ baseURL: '/' })

const KEY_STORAGE = 'modelhub-api-key'
const MASTER_STORAGE = 'modelhub-master-key'
const SESSION_STORAGE = 'modelhub-session'

export function getApiKey(): string {
  return localStorage.getItem(KEY_STORAGE) || ''
}

export function setApiKey(key: string): void {
  if (key) {
    localStorage.setItem(KEY_STORAGE, key)
  } else {
    localStorage.removeItem(KEY_STORAGE)
  }
}

export function getMasterKey(): string {
  return localStorage.getItem(MASTER_STORAGE) || ''
}

export function setMasterKey(key: string): void {
  if (key) {
    localStorage.setItem(MASTER_STORAGE, key)
  } else {
    localStorage.removeItem(MASTER_STORAGE)
  }
}

// --- console session (JWT) ---

export interface Session {
  access_token: string
  refresh_token: string
}

export function getSession(): Session | null {
  const raw = localStorage.getItem(SESSION_STORAGE)
  if (!raw) return null
  try {
    const s = JSON.parse(raw) as Session
    if (s.access_token && s.refresh_token) return s
  } catch {
    localStorage.removeItem(SESSION_STORAGE)
  }
  return null
}

export function setSession(session: Session | null): void {
  if (session) {
    localStorage.setItem(SESSION_STORAGE, JSON.stringify(session))
  } else {
    localStorage.removeItem(SESSION_STORAGE)
  }
}

function isConsoleApi(url: string): boolean {
  return url.startsWith('/api/') && !url.startsWith('/api/auth/')
}

// Attach the access token to console API requests.
http.interceptors.request.use((config) => {
  const url = config.url || ''
  if (isConsoleApi(url)) {
    const s = getSession()
    if (s) {
      config.headers.set('Authorization', `Bearer ${s.access_token}`)
    }
  }
  return config
})

let refreshInFlight: Promise<boolean> | null = null

function refreshInFlightOnce(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = (async () => {
      const s = getSession()
      if (!s) return false
      try {
        const { data } = await http.post<AuthTokens>('/api/auth/refresh', {
          refresh_token: s.refresh_token,
        })
        setSession({
          access_token: data.access_token,
          refresh_token: data.refresh_token,
        })
        return true
      } catch {
        setSession(null)
        return false
      }
    })().finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

// On a 401 from the console API, refresh once and retry the request.
http.interceptors.response.use(
  (response) => response,
  async (error: unknown) => {
    const axiosError = error as {
      config?: { url?: string; _retried?: boolean }
      response?: { status?: number }
    }
    const config = axiosError.config
    if (
      config &&
      axiosError.response?.status === 401 &&
      isConsoleApi(config.url || '') &&
      !config._retried
    ) {
      config._retried = true
      if (await refreshInFlightOnce()) {
        return http(config)
      }
    }
    return Promise.reject(error)
  },
)

export interface ModelInfo {
  id: string
  object: string
  created: number
  owned_by: string
  input_price_per_1k?: number
  output_price_per_1k?: number
}

export async function listModels(): Promise<ModelInfo[]> {
  const { data } = await http.get('/v1/models', {
    headers: { Authorization: `Bearer ${getApiKey()}` },
  })
  return data.data as ModelInfo[]
}

export function authHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${getApiKey()}`,
    'Content-Type': 'application/json',
  }
}

// --- P2-2: chat agents (predefined templates) ---

export interface ChatAgent {
  agent_id: string
  name: string
  description: string
  model: string
}

// Lists enabled agent templates; call chat with model = <agent_id>.
export async function listChatAgents(): Promise<ChatAgent[]> {
  const { data } = await http.get('/v1/agents', {
    headers: { Authorization: `Bearer ${getApiKey()}` },
  })
  return data.data as ChatAgent[]
}

export function adminHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${getMasterKey()}`,
    'Content-Type': 'application/json',
  }
}

// --- admin: models ---

export interface AdminModel {
  model_id: string
  provider: string
  upstream_model: string
  capabilities: string[]
  input_price_per_1k: number
  output_price_per_1k: number
  price_unit?: string
  unit_price?: number
  enabled: boolean
}

export async function listAdminModels(): Promise<AdminModel[]> {
  const { data } = await http.get('/admin/models', { headers: adminHeaders() })
  return data.data as AdminModel[]
}

export async function createAdminModel(body: {
  model_id: string
  provider: string
  upstream_model: string
  capabilities?: string[]
  input_price_per_1k: number
  output_price_per_1k: number
  price_unit?: string
  unit_price?: number
  enabled?: boolean
}): Promise<void> {
  await http.post('/admin/models', body, { headers: adminHeaders() })
}

export async function deleteAdminModel(modelId: string): Promise<void> {
  await http.delete(`/admin/models/${encodeURIComponent(modelId)}`, { headers: adminHeaders() })
}

export async function updateModel(
  modelId: string,
  patch: {
    upstream_model?: string
    capabilities?: string[]
    input_price_per_1k?: number
    output_price_per_1k?: number
    price_unit?: string
    unit_price?: number
    enabled?: boolean
  },
): Promise<void> {
  await http.patch(`/admin/models/${encodeURIComponent(modelId)}`, patch, { headers: adminHeaders() })
}

// --- admin: channels ---

export interface AdminChannel {
  id: number
  name: string
  provider: string
  base_url: string
  api_key: string
  model_id: string
  priority: number
  enabled: boolean
  health?: string
  cooldown_until?: string
}

export async function listChannels(): Promise<AdminChannel[]> {
  const { data } = await http.get('/admin/channels', { headers: adminHeaders() })
  return data.data as AdminChannel[]
}

export async function createChannel(body: {
  name: string
  provider: string
  base_url: string
  api_key: string
  model_id: string
  priority: number
}): Promise<void> {
  await http.post('/admin/channels', body, { headers: adminHeaders() })
}

export async function deleteChannel(id: number): Promise<void> {
  await http.delete(`/admin/channels/${id}`, { headers: adminHeaders() })
}

export async function updateChannel(
  id: number,
  patch: {
    name?: string
    provider?: string
    base_url?: string
    api_key?: string
    model_id?: string
    priority?: number
    enabled?: boolean
  },
): Promise<void> {
  await http.patch(`/admin/channels/${id}`, patch, { headers: adminHeaders() })
}

export interface ChannelTestResult {
  ok: boolean
  status?: number
  latency_ms?: number
  error?: string
}

export async function testChannel(id: number): Promise<ChannelTestResult> {
  const { data } = await http.post(`/admin/channels/${id}/test`, {}, { headers: adminHeaders() })
  return data as ChannelTestResult
}

// --- admin: api keys ---

export interface AdminKey {
  id: number
  name: string
  user_id?: number
  agent_id?: number
  markup?: number
  quota_usd?: number
  allowed_models: string[]
  spend_micro: number
  spend_usd: number
  expires_at?: string
  created_at: string
}

export async function listKeys(): Promise<AdminKey[]> {
  const { data } = await http.get('/admin/api-keys', { headers: adminHeaders() })
  return data.data as AdminKey[]
}

export async function createKey(name: string): Promise<{ key: string; id: number }> {
  const { data } = await http.post('/admin/api-keys', { name }, { headers: adminHeaders() })
  return { key: data.key, id: data.id }
}

export async function updateKey(
  id: number,
  patch: {
    name?: string
    user_id?: number
    quota_usd?: number
    allowed_models?: string[]
    expires_at?: string
  },
): Promise<AdminKey> {
  const { data } = await http.patch(`/admin/api-keys/${id}`, patch, { headers: adminHeaders() })
  return data as AdminKey
}

export async function deleteKey(id: number): Promise<void> {
  await http.delete(`/admin/api-keys/${id}`, { headers: adminHeaders() })
}

// --- admin: users (billing) ---

export interface AdminUser {
  id: number
  email: string
  balance_micro: number
  balance_usd: number
  enabled: boolean
  created_at: string
  agent_rate?: number
}

export async function listUsers(): Promise<AdminUser[]> {
  const { data } = await http.get('/admin/users', { headers: adminHeaders() })
  return data.data as AdminUser[]
}

export async function createUser(email: string, initialBalanceUsd: number): Promise<AdminUser> {
  const { data } = await http.post(
    '/admin/users',
    { email, initial_balance_usd: initialBalanceUsd },
    { headers: adminHeaders() },
  )
  return data as AdminUser
}

export async function creditUser(id: number, amountUsd: number, reason: string): Promise<AdminUser> {
  const { data } = await http.post(
    `/admin/users/${id}/credit`,
    { amount_usd: amountUsd, reason },
    { headers: adminHeaders() },
  )
  return data as AdminUser
}

export async function updateUser(id: number, enabled: boolean): Promise<void> {
  await http.patch(`/admin/users/${id}`, { enabled }, { headers: adminHeaders() })
}

export interface LedgerEntry {
  id: number
  kind: 'credit' | 'hold' | 'release' | 'debit'
  amount: number
  amount_usd: number
  reason: string
  request_id?: string
  created_at: string
}

export async function userLedger(userId: number, limit = 50): Promise<LedgerEntry[]> {
  const { data } = await http.get(`/admin/users/${userId}/ledger`, {
    params: { limit, offset: 0 },
    headers: adminHeaders() as Record<string, string>,
  })
  return data.data as LedgerEntry[]
}

// --- admin: usage ---

export interface UsageRecord {
  id: number
  api_key_id?: number
  model: string
  provider: string
  stream: boolean
  prompt_tokens: number
  completion_tokens: number
  cost_usd: number
  status: string
  error: string
  created_at: string
}

export interface UsageSummary {
  requests: number
  prompt_tokens: number
  completion_tokens: number
  cost_micro: number
  cost_usd: number
}

export async function listUsage(apiKeyId?: number, limit = 50): Promise<UsageRecord[]> {
  const { data } = await http.get('/admin/usage', {
    params: { api_key_id: apiKeyId, limit, offset: 0 },
    headers: adminHeaders() as Record<string, string>,
  })
  return data.data as UsageRecord[]
}

export async function usageSummary(apiKeyId?: number, since?: string): Promise<UsageSummary> {
  const { data } = await http.get('/admin/usage/summary', {
    params: { api_key_id: apiKeyId, since },
    headers: adminHeaders() as Record<string, string>,
  })
  return data as UsageSummary
}

export interface UsageDailyPoint {
  date: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  cost_micro: number
  cost_usd: number
}

export async function usageDaily(days = 14): Promise<UsageDailyPoint[]> {
  const { data } = await http.get('/admin/usage/daily', {
    params: { days },
    headers: adminHeaders() as Record<string, string>,
  })
  return data.data as UsageDailyPoint[]
}

export function formatUsd(micro?: number, usd?: number): string {
  const value = usd !== undefined ? usd : (micro ?? 0) / 1_000_000
  return `$${value.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')}`
}

// --- console auth (JWT) ---

export interface AuthTokens {
  access_token: string
  refresh_token: string
  token_type: string
  access_token_expires_in: number
}

export interface MeUser {
  id: number
  email: string
  balance_micro: number
  balance_usd: number
  enabled: boolean
  created_at: string
  agent_rate?: number
}

function storeTokens(data: AuthTokens): void {
  setSession({
    access_token: data.access_token,
    refresh_token: data.refresh_token,
  })
}

export async function login(email: string, password: string): Promise<void> {
  const { data } = await http.post<AuthTokens>('/api/auth/login', { email, password })
  storeTokens(data)
}

export async function register(email: string, password: string): Promise<void> {
  const { data } = await http.post<AuthTokens>('/api/auth/register', { email, password })
  storeTokens(data)
}

export async function logout(): Promise<void> {
  const s = getSession()
  if (s) {
    try {
      await http.post('/api/auth/logout', { refresh_token: s.refresh_token })
    } catch {
      // best effort: local session is cleared regardless
    }
  }
  setSession(null)
}

export async function fetchMe(): Promise<MeUser> {
  const { data } = await http.get<MeUser>('/api/me')
  return data
}

export async function myLedger(limit = 50): Promise<LedgerEntry[]> {
  const { data } = await http.get<{ data: LedgerEntry[] }>('/api/me/ledger', {
    params: { limit },
  })
  return data.data
}

// --- console: my keys ---

export interface MyKey {
  id: number
  name: string
  allowed_models: string[]
  spend_micro: number
  spend_usd: number
  created_at: string
}

export async function myKeys(): Promise<MyKey[]> {
  const { data } = await http.get<{ data: MyKey[] }>('/api/me/keys')
  return data.data
}

export async function createMyKey(name: string): Promise<{ key: string; id: number; name: string }> {
  const { data } = await http.post('/api/me/keys', { name })
  return data
}

export async function deleteMyKey(id: number): Promise<void> {
  await http.delete(`/api/me/keys/${id}`)
}

// --- public models (no auth required) ---

export interface PublicModel {
  id: string
  provider: string
  capabilities: string[]
  input_price_per_1k: number
  output_price_per_1k: number
  price_unit?: string
  unit_price?: number
}

export async function listPublicModels(): Promise<PublicModel[]> {
  const { data } = await http.get<{ data: PublicModel[] }>('/api/models')
  return data.data
}

// --- public rankings ---

export type RankingPeriod = 'today' | 'week' | 'month' | 'year'

export interface RankingEntry {
  model_id: string
  provider: string
  tokens: number
  requests: number
  cost_micro: number
  change_pct: number
}

export interface RankingBucket {
  bucket: string
  model_id: string
  tokens: number
}

export interface PublicRankings {
  period: RankingPeriod
  total_tokens: number
  models: RankingEntry[]
  buckets: RankingBucket[]
}

export async function publicRankings(period: RankingPeriod): Promise<PublicRankings> {
  const { data } = await http.get<{ data: PublicRankings }>('/api/rankings', {
    params: { period },
  })
  return data.data
}

// --- M4: async media tasks ---

export type MediaStatus = 'queued' | 'running' | 'succeeded' | 'failed'

export interface MediaTask {
  task_id: string
  status: MediaStatus | string
  type: string
  model: string
  result_urls: string[]
  cost_usd: number
  error: string
  created_at: string
  updated_at: string
}

export interface MediaRequest {
  model: string
  type?: string
  prompt?: string
  text?: string
  n?: number
  size?: string
  duration?: string
}

export async function submitMedia(body: MediaRequest): Promise<{ task_id: string; status: string }> {
  const { data } = await http.post('/v1/media/generate', body, { headers: authHeaders() })
  return data
}

export async function mediaStatus(taskId: string): Promise<MediaTask> {
  const { data } = await http.get(`/v1/media/status/${encodeURIComponent(taskId)}`, {
    headers: authHeaders(),
  })
  return data
}

// --- P2-1: comic drama (storyboard + asset pack) ---

export interface DramaShot {
  shot_no: number
  scene: string
  dialogue: string
  image_prompt: string
  status: string
  image_url?: string
  audio_url?: string
  error?: string
}

export interface Drama {
  drama_id: string
  status: string
  title: string
  style: string
  storyboard_model: string
  image_model: string
  tts_model?: string | null
  shots_planned: number
  shots: DramaShot[]
  cost_usd: number
  error: string
  video_url?: string
  video_error?: string
  created_at: string
  updated_at: string
}

export interface DramaRequest {
  script: string
  style?: string
  shots?: number
  storyboard_model?: string
  image_model: string
  tts_model?: string
}

export async function submitDrama(body: DramaRequest): Promise<{ drama_id: string; status: string }> {
  const { data } = await http.post('/v1/drama/generate', body, { headers: authHeaders() })
  return data
}

export async function dramaStatus(dramaId: string): Promise<Drama> {
  const { data } = await http.get(`/v1/drama/status/${encodeURIComponent(dramaId)}`, {
    headers: authHeaders(),
  })
  return data
}

// --- P2-3: reseller agents (admin) ---

export interface AdminAgent {
  id: number
  user_id: number
  email: string
  rate: number
  created_at: string
}

export async function listAgents(): Promise<AdminAgent[]> {
  const { data } = await http.get('/admin/agents', { headers: adminHeaders() })
  return data.data as AdminAgent[]
}

export async function createAgent(userId: number, rate: number): Promise<AdminAgent> {
  const { data } = await http.post('/admin/agents', { user_id: userId, rate }, { headers: adminHeaders() })
  return data as AdminAgent
}

export async function updateAgentRate(id: number, rate: number): Promise<AdminAgent> {
  const { data } = await http.put(`/admin/agents/${id}`, { rate }, { headers: adminHeaders() })
  return data as AdminAgent
}

export async function deleteAgent(id: number): Promise<void> {
  await http.delete(`/admin/agents/${id}`, { headers: adminHeaders() })
}

// --- P2-3: reseller subkeys (console, agent accounts only) ---

export interface Subkey {
  id: number
  name: string
  markup: number
  user_id?: number
  allowed_models: string[]
  spend_micro: number
  spend_usd: number
  quota_usd?: number
  expires_at?: string
  created_at: string
}

export async function listSubkeys(): Promise<Subkey[]> {
  const { data } = await http.get<{ data: Subkey[] }>('/api/me/subkeys')
  return data.data
}

export async function createSubkey(body: {
  name: string
  markup?: number
  quota_usd?: number
  allowed_models?: string[]
  expires_at?: string
}): Promise<{ key: string; id: number; name: string }> {
  const { data } = await http.post('/api/me/subkeys', body)
  return data
}

export async function deleteSubkey(id: number): Promise<void> {
  await http.delete(`/api/me/subkeys/${id}`)
}

// --- P2-4: online recharge (CNY payment -> USD balance) ---

export interface RechargeConfig {
  enabled: boolean
  cny_per_usd: number
  methods: string[]
  min_cny: number // fen
  max_cny: number // fen
}

export interface Recharge {
  id: number
  order_no: string
  method: string
  amount_cny: number // fen
  amount_yuan: number
  credit_micro: number
  credit_usd: number
  status: 'pending' | 'paid' | 'failed' | 'refunded'
  promo_code?: string
  created_at: string
  paid_at: string | null
  refund_at?: string | null
  user_id?: number
  email?: string
}

export interface RechargePayment {
  qr_code: string
  pay_url: string
}

export async function rechargeConfig(): Promise<RechargeConfig> {
  const { data } = await http.get<RechargeConfig>('/api/me/recharge/config')
  return data
}

export async function createRecharge(
  amountCnyFen: number,
  method: string,
  subType?: string,
  promoCode?: string,
): Promise<{ order: Recharge; payment: RechargePayment }> {
  const { data } = await http.post('/api/me/recharges', {
    amount_cny: amountCnyFen,
    method,
    sub_type: subType || undefined,
    promo_code: promoCode || undefined,
  })
  return data
}

export async function myRecharges(limit = 20): Promise<Recharge[]> {
  const { data } = await http.get<{ data: Recharge[] }>('/api/me/recharges', {
    params: { limit },
  })
  return data.data
}

export async function rechargeStatus(id: number): Promise<Recharge> {
  const { data } = await http.get<Recharge>(`/api/me/recharges/${id}`)
  return data
}

// --- P3-3: organizations (multi-tenancy) ---

export type OrgRole = 'owner' | 'admin' | 'member'

export interface MyOrg {
  id: number
  name: string
  owner_user_id: number
  balance_micro: number
  balance_usd: number
  created_at: string
  role: OrgRole
}

export interface OrgMember {
  user_id: number
  email: string
  role: OrgRole
  joined_at: string
}

export interface OrgDetail extends MyOrg {
  members: OrgMember[]
}

export interface OrgKey {
  id: number
  name: string
  allowed_models?: string[]
  spend_micro: number
  spend_usd: number
  created_at: string
  quota_usd?: number
  expires_at?: string
}

export interface OrgUsageSummary {
  requests: number
  prompt_tokens: number
  completion_tokens: number
  cost_usd: number
}

export interface OrgUsageRecord {
  id: number
  api_key_id?: number
  key_name: string
  model_id: string
  provider: string
  prompt_tokens: number
  completion_tokens: number
  cost_usd: number
  status: string
  created_at: string
}

export interface OrgLedgerEntry {
  id: number
  kind: string
  amount: number
  amount_usd: number
  reason: string
  created_at: string
  request_id?: string
}

export async function listMyOrgs(): Promise<MyOrg[]> {
  const { data } = await http.get<{ data: MyOrg[] }>('/api/me/orgs')
  return data.data
}

export async function createOrg(name: string): Promise<MyOrg> {
  const { data } = await http.post<MyOrg>('/api/me/orgs', { name })
  return data
}

export async function getOrg(id: number): Promise<OrgDetail> {
  const { data } = await http.get<OrgDetail>(`/api/me/orgs/${id}`)
  return data
}

export async function addOrgMember(
  orgId: number,
  email: string,
  role: OrgRole = 'member',
): Promise<OrgMember> {
  const { data } = await http.post<OrgMember>(`/api/me/orgs/${orgId}/members`, {
    email,
    role,
  })
  return data
}

export async function setOrgMemberRole(
  orgId: number,
  userId: number,
  role: 'admin' | 'member',
): Promise<void> {
  await http.put(`/api/me/orgs/${orgId}/members/${userId}`, { role })
}

export async function removeOrgMember(orgId: number, userId: number): Promise<void> {
  await http.delete(`/api/me/orgs/${orgId}/members/${userId}`)
}

export async function listOrgKeys(orgId: number): Promise<OrgKey[]> {
  const { data } = await http.get<{ data: OrgKey[] }>(`/api/me/orgs/${orgId}/keys`)
  return data.data
}

export async function createOrgKey(
  orgId: number,
  body: {
    name: string
    allowed_models?: string[]
    quota_usd?: number
    expires_at?: string
  },
): Promise<{ key: string; org: OrgKey }> {
  const { data } = await http.post<{ key: string; org: OrgKey }>(
    `/api/me/orgs/${orgId}/keys`,
    body,
  )
  return data
}

export async function deleteOrgKey(orgId: number, keyId: number): Promise<void> {
  await http.delete(`/api/me/orgs/${orgId}/keys/${keyId}`)
}

export async function orgUsage(
  orgId: number,
  limit = 50,
): Promise<{ summary: OrgUsageSummary; data: OrgUsageRecord[] }> {
  const { data } = await http.get<{
    summary: OrgUsageSummary
    data: OrgUsageRecord[]
  }>(`/api/me/orgs/${orgId}/usage`, { params: { limit } })
  return data
}

export async function orgLedger(
  orgId: number,
  limit = 50,
): Promise<OrgLedgerEntry[]> {
  const { data } = await http.get<{ data: OrgLedgerEntry[] }>(
    `/api/me/orgs/${orgId}/ledger`,
    { params: { limit } },
  )
  return data.data
}

// --- admin: chat-agent templates (assistants) ---

export interface AdminAssistant {
  id: number
  agent_id: string
  name: string
  description: string
  system_prompt: string
  model: string
  tools: unknown
  enabled: boolean
  created_at: string
}

export async function listAssistants(): Promise<AdminAssistant[]> {
  const { data } = await http.get('/admin/assistants', { headers: adminHeaders() })
  return data.data as AdminAssistant[]
}

export async function createAssistant(body: {
  agent_id: string
  name: string
  description?: string
  system_prompt: string
  model: string
  tools?: string
  enabled?: boolean
}): Promise<void> {
  const payload: Record<string, unknown> = { ...body }
  if (typeof body.tools === 'string' && body.tools.trim()) {
    try {
      payload.tools = JSON.parse(body.tools)
    } catch {
      throw new Error('tools 必须是 JSON 数组')
    }
  } else {
    delete payload.tools
  }
  await http.post('/admin/assistants', payload, { headers: adminHeaders() })
}

export async function deleteAssistant(id: number): Promise<void> {
  await http.delete(`/admin/assistants/${id}`, { headers: adminHeaders() })
}

export async function updateAssistant(id: number, patch: {
  name?: string
  description?: string
  system_prompt?: string
  model?: string
  tools?: string
  enabled?: boolean
}): Promise<void> {
  const payload: Record<string, unknown> = { ...patch }
  if (typeof patch.tools === 'string' && patch.tools.trim()) {
    try {
      payload.tools = JSON.parse(patch.tools)
    } catch {
      throw new Error('tools 必须是 JSON 数组')
    }
  } else {
    delete payload.tools
  }
  await http.patch(`/admin/assistants/${id}`, payload, { headers: adminHeaders() })
}

// --- admin: external systems (iframe-embedded pages) ---

export interface ExternalPage {
  id: number
  name: string
  url: string
  enabled: boolean
  sort_order: number
  created_at: string
}

export async function listExternalPages(): Promise<ExternalPage[]> {
  const { data } = await http.get('/admin/external-pages', { headers: adminHeaders() })
  return data.data as ExternalPage[]
}

export async function createExternalPage(body: {
  name: string
  url: string
  sort_order?: number
}): Promise<void> {
  await http.post('/admin/external-pages', body, { headers: adminHeaders() })
}

export async function updateExternalPage(id: number, patch: {
  name?: string
  url?: string
  enabled?: boolean
  sort_order?: number
}): Promise<void> {
  await http.patch(`/admin/external-pages/${id}`, patch, { headers: adminHeaders() })
}

export async function deleteExternalPage(id: number): Promise<void> {
  await http.delete(`/admin/external-pages/${id}`, { headers: adminHeaders() })
}

// --- admin: organizations (platform audit) ---

export interface AdminOrg {
  id: number
  name: string
  owner_user_id: number
  owner_email: string
  member_count: number
  balance_micro: number
  balance_usd: number
  created_at: string
}

export async function adminListOrgs(limit = 100): Promise<AdminOrg[]> {
  const { data } = await http.get<{ data: AdminOrg[] }>('/admin/organizations', {
    params: { limit, offset: 0 },
    headers: adminHeaders(),
  })
  return data.data
}

export async function adminCreditOrg(id: number, amountUsd: number, reason: string): Promise<void> {
  await http.post(
    `/admin/organizations/${id}/credit`,
    { amount_usd: amountUsd, reason },
    { headers: adminHeaders() },
  )
}

// --- admin: recharge orders ---

export async function adminRecharges(limit = 50): Promise<Recharge[]> {
  const { data } = await http.get<{ data: Recharge[] }>('/admin/recharges', {
    params: { limit, offset: 0 },
    headers: adminHeaders() as Record<string, string>,
  })
  return data.data
}

// --- admin: payment configuration (P3-2, hot-reload) ---

export interface PayChannelView {
  enabled: boolean
  config: Record<string, string>
  status: { ok: boolean; error?: string }
}

export interface PayConfigView {
  cny_per_usd: number
  public_url: string
  channels: Record<string, PayChannelView>
}

export const PAY_CHANNEL_IDS = ['yipay', 'alipay', 'wechat'] as const

export const PAY_CHANNEL_FIELDS: Record<string, { name: string; secret: boolean }[]> = {
  yipay: [
    { name: 'mapi_url', secret: false },
    { name: 'pid', secret: false },
    { name: 'key', secret: true },
  ],
  alipay: [
    { name: 'app_id', secret: false },
    { name: 'private_key', secret: true },
    { name: 'public_key', secret: false },
  ],
  wechat: [
    { name: 'mch_id', secret: false },
    { name: 'app_id', secret: false },
    { name: 'api_v3_key', secret: true },
    { name: 'merchant_serial', secret: false },
    { name: 'private_key', secret: true },
    { name: 'platform_key', secret: true },
  ],
}

export async function getPayConfig(): Promise<PayConfigView> {
  const { data } = await http.get<PayConfigView>('/admin/pay-config', { headers: adminHeaders() })
  return data
}

export async function putPayConfig(body: {
  cny_per_usd?: number
  public_url?: string
  channels?: Record<string, { enabled?: boolean; config: Record<string, string> }>
}): Promise<PayConfigView> {
  const { data } = await http.put<PayConfigView>('/admin/pay-config', body, {
    headers: adminHeaders(),
  })
  return data
}

// --- P0: terms / announcements / redeem / promo / plans / subscription ---

export interface TermsDoc {
  content: string
  updated_at: string
}

export async function getTerms(): Promise<TermsDoc> {
  const { data } = await http.get<TermsDoc>('/api/terms')
  return data
}

export async function adminGetTerms(): Promise<TermsDoc> {
  const { data } = await http.get<TermsDoc>('/admin/terms', { headers: adminHeaders() })
  return data
}

export async function adminPutTerms(content: string): Promise<TermsDoc> {
  const { data } = await http.put<TermsDoc>('/admin/terms', { content }, { headers: adminHeaders() })
  return data
}

export interface Announcement {
  id: number
  title: string
  content: string
  style: 'popup' | 'banner'
  active: boolean
  views: number
  created_at: string
}

export async function listAnnouncements(): Promise<Announcement[]> {
  const { data } = await http.get<{ data: Announcement[] }>('/api/announcements')
  return data.data
}

export async function viewAnnouncement(id: number): Promise<void> {
  await http.post(`/api/announcements/${id}/view`)
}

export async function adminListAnnouncements(): Promise<Announcement[]> {
  const { data } = await http.get<{ data: Announcement[] }>('/admin/announcements', {
    headers: adminHeaders(),
  })
  return data.data
}

export async function adminCreateAnnouncement(body: {
  title: string
  content?: string
  style?: string
}): Promise<void> {
  await http.post('/admin/announcements', body, { headers: adminHeaders() })
}

export async function adminUpdateAnnouncement(
  id: number,
  patch: { title?: string; content?: string; style?: string; active?: boolean },
): Promise<void> {
  await http.patch(`/admin/announcements/${id}`, patch, { headers: adminHeaders() })
}

export async function adminDeleteAnnouncement(id: number): Promise<void> {
  await http.delete(`/admin/announcements/${id}`, { headers: adminHeaders() })
}

export interface RedeemCode {
  id: number
  code: string
  credit_micro: number
  credit_usd: number
  used_by: number | null
  used_at: string | null
  created_at: string
}

export async function adminListRedeemCodes(): Promise<RedeemCode[]> {
  const { data } = await http.get<{ data: RedeemCode[] }>('/admin/redeem-codes', {
    headers: adminHeaders(),
  })
  return data.data
}

export async function adminCreateRedeemCodes(
  count: number,
  creditUsd: number,
): Promise<RedeemCode[]> {
  const { data } = await http.post<{ data: RedeemCode[] }>('/admin/redeem-codes', {
    count,
    credit_usd: creditUsd,
  }, { headers: adminHeaders() })
  return data.data
}

export async function adminDeleteRedeemCode(id: number): Promise<void> {
  await http.delete(`/admin/redeem-codes/${id}`, { headers: adminHeaders() })
}

export async function redeemCode(code: string): Promise<void> {
  await http.post('/api/me/redeem', { code })
}

export interface PromoCode {
  id: number
  code: string
  kind: 'percent' | 'amount_off'
  value: number
  min_cny: number // fen
  max_uses: number
  used_count: number
  enabled: boolean
  expires_at: string | null
  created_at: string
}

export async function adminListPromos(): Promise<PromoCode[]> {
  const { data } = await http.get<{ data: PromoCode[] }>('/admin/promo-codes', {
    headers: adminHeaders(),
  })
  return data.data
}

export async function adminCreatePromo(body: {
  code: string
  kind: 'percent' | 'amount_off'
  value: number
  min_cny_fen?: number
  max_uses?: number
  expires_at?: string | null
  enabled?: boolean
}): Promise<void> {
  await http.post('/admin/promo-codes', body, { headers: adminHeaders() })
}

export async function adminUpdatePromo(
  id: number,
  patch: {
    kind?: 'percent' | 'amount_off'
    value?: number
    min_cny_fen?: number
    max_uses?: number
    expires_at?: string | null
    enabled?: boolean
  },
): Promise<void> {
  await http.patch(`/admin/promo-codes/${id}`, patch, { headers: adminHeaders() })
}

export async function adminDeletePromo(id: number): Promise<void> {
  await http.delete(`/admin/promo-codes/${id}`, { headers: adminHeaders() })
}

export interface Plan {
  id: number
  name: string
  quota_micro: number
  quota_usd: number
  period_days: number
  price_cny: number // fen
  price_yuan: number
  enabled: boolean
  created_at: string
}

export async function adminListPlans(): Promise<Plan[]> {
  const { data } = await http.get<{ data: Plan[] }>('/admin/plans', { headers: adminHeaders() })
  return data.data
}

export async function adminCreatePlan(body: {
  name: string
  quota_usd: number
  period_days: number
  price_cny_fen?: number
  enabled?: boolean
}): Promise<void> {
  await http.post('/admin/plans', body, { headers: adminHeaders() })
}

export async function adminUpdatePlan(
  id: number,
  patch: {
    name?: string
    quota_usd?: number
    period_days?: number
    price_cny_fen?: number
    enabled?: boolean
  },
): Promise<void> {
  await http.patch(`/admin/plans/${id}`, patch, { headers: adminHeaders() })
}

export async function adminDeletePlan(id: number): Promise<void> {
  await http.delete(`/admin/plans/${id}`, { headers: adminHeaders() })
}

export interface Subscription {
  plan_id: number
  plan_name: string
  quota_micro: number
  quota_usd: number
  used_micro: number
  used_usd: number
  reset_at: string
  created_at: string
  user_id?: number
}

export async function mySubscription(): Promise<Subscription | null> {
  const { data } = await http.get<{ subscription: Subscription | null }>('/api/me/subscription')
  return data.subscription
}

export async function adminSetUserSubscription(userId: number, planId: number): Promise<Subscription> {
  const { data } = await http.post<Subscription>(
    `/admin/users/${userId}/subscription`,
    { plan_id: planId },
    { headers: adminHeaders() },
  )
  return data
}

export async function adminListSubscriptions(): Promise<Subscription[]> {
  const { data } = await http.get<{ data: Subscription[] }>('/admin/subscriptions', {
    headers: adminHeaders(),
  })
  return data.data
}

export interface RechargeStats {
  total_micro: number
  total_usd: number
  by_day: { date: string; micro: number; usd: number }[]
  by_method: { method: string; micro: number; usd: number; orders: number }[]
  top_users: { user_id: number; email: string; micro: number; usd: number }[]
}

export async function adminRechargeStats(days = 30): Promise<RechargeStats> {
  const { data } = await http.get<RechargeStats>('/admin/recharge-stats', {
    params: { days },
    headers: adminHeaders() as Record<string, string>,
  })
  return data
}

export async function adminRefundRecharge(id: number, reason?: string): Promise<void> {
  await http.post(`/admin/recharges/${id}/refund`, { reason: reason || '' }, {
    headers: adminHeaders(),
  })
}
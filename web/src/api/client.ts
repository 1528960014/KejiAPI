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
}): Promise<void> {
  await http.post('/admin/models', body, { headers: adminHeaders() })
}

export async function deleteAdminModel(modelId: string): Promise<void> {
  await http.delete(`/admin/models/${encodeURIComponent(modelId)}`, { headers: adminHeaders() })
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
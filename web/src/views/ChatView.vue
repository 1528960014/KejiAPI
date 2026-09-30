<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import {
  authHeaders,
  formatUsd,
  getApiKey,
  listChatAgents,
  listPublicModels,
  mediaStatus,
  setApiKey,
  submitMedia,
  type ChatAgent,
  type MediaRequest,
  type PublicModel,
} from '../api/client'
import { getTheme, toggleTheme, type Theme } from '../theme'
import { useSession } from '../session'

const { t } = useI18n()
const router = useRouter()
const session = useSession()

const TTS_MODEL_STORAGE = 'modelhub.ttsModel'
const USED_STORAGE = 'modelhub.usedModels'
const SESSIONS_STORAGE = 'modelhub.sessions'

type Cap = 'text' | 'image' | 'video' | 'music' | 'tts'
type MediaCap = Exclude<Cap, 'text'>
type Filter = 'all' | 'text' | 'image' | 'video' | 'audio' | 'mine'

interface ChatCol {
  model: string
  content: string
  done: boolean
  error: string
  promptTokens: number
  completionTokens: number
}

interface MediaItem {
  kind: MediaCap
  model: string
  status: string
  error: string
  urls: string[]
  costUsd: number
}

interface Round {
  user: string
  media: MediaItem | null
  columns: ChatCol[]
}

interface SessionItem {
  title: string
  at: number
  rounds: Round[]
}

const CAP_ORDER: Cap[] = ['text', 'image', 'video', 'music', 'tts']
const CAP_GLYPH: Record<Cap, string> = { text: '✦', image: '❖', video: '▶', music: '♪', tts: '◉' }
const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'shell.tabAll' },
  { key: 'text', label: 'shell.tabText' },
  { key: 'image', label: 'shell.tabImage' },
  { key: 'video', label: 'shell.tabVideo' },
  { key: 'audio', label: 'shell.tabAudio' },
  { key: 'mine', label: 'shell.tabMine' },
]

// ---------- state ----------
const theme = ref<Theme>(getTheme())
const apiKey = ref(getApiKey())
const publicModels = ref<PublicModel[]>([])
const agents = ref<ChatAgent[]>([])
const selectedAgent = ref('')
const selected = ref('')
const compare = ref(false)
const compareList = ref<string[]>([])
const filter = ref<Filter>('all')
const provider = ref('')
const query = ref('')
const input = ref('')
const busy = ref(false)
const error = ref('')
const rounds = ref<Round[]>([])
const usedModels = ref<string[]>(loadJson(USED_STORAGE, []))
const sessions = ref<SessionItem[]>(loadJson(SESSIONS_STORAGE, []))
const ttsModel = ref(localStorage.getItem(TTS_MODEL_STORAGE) || '')
const playingId = ref('')
const modelPop = ref(false)
const keyPop = ref(false)
const stageRef = ref<HTMLElement | null>(null)

const pollTimers = new Set<number>()

function loadJson<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : fallback
  } catch {
    return fallback
  }
}
function persistJson(key: string, value: unknown) {
  localStorage.setItem(key, JSON.stringify(value))
}

// ---------- derived ----------
function primaryCap(m: PublicModel): Cap {
  const caps = m.capabilities || []
  for (const c of CAP_ORDER) if (caps.includes(c)) return c
  return 'text'
}

const providers = computed(() =>
  [...new Set(publicModels.value.map((m) => m.provider).filter(Boolean))].sort(),
)

const filteredModels = computed(() => {
  let list = publicModels.value
  const f = filter.value
  if (f === 'mine') list = list.filter((m) => usedModels.value.includes(m.id))
  else if (f === 'text') list = list.filter((m) => (m.capabilities || []).includes('text'))
  else if (f === 'image') list = list.filter((m) => (m.capabilities || []).includes('image'))
  else if (f === 'video') list = list.filter((m) => (m.capabilities || []).includes('video'))
  else if (f === 'audio')
    list = list.filter((m) => {
      const caps = m.capabilities || []
      return caps.includes('music') || caps.includes('tts')
    })
  if (provider.value) list = list.filter((m) => m.provider === provider.value)
  const q = query.value.trim().toLowerCase()
  if (q) list = list.filter((m) => m.id.toLowerCase().includes(q))
  return list
})

const chatModels = computed(() =>
  publicModels.value.filter((m) => (m.capabilities || []).includes('text')),
)

const selectedModel = computed(
  () => publicModels.value.find((m) => m.id === selected.value) || null,
)
const agent = computed(() => agents.value.find((a) => a.agent_id === selectedAgent.value) || null)
const pillLabel = computed(() =>
  agent.value ? `${agent.value.name} · ${agent.value.model}` : selected.value,
)
const ttsModels = computed(() =>
  publicModels.value.filter((m) => (m.capabilities || []).includes('tts')).map((m) => m.id),
)
const canSend = computed(
  () =>
    !!input.value.trim() &&
    !busy.value &&
    !!apiKey.value &&
    (!!selectedAgent.value || !!selected.value),
)
const heroDesc = computed(() => {
  if (agent.value) return agent.value.description
  const m = selectedModel.value
  if (!m) return t('shell.heroHint')
  const caps = (m.capabilities || [])
    .map((c) => t(`shell.cap.${c}`))
    .filter(Boolean)
    .join(' / ')
  return `${m.provider} · ${caps} · ${priceLine(m)}`
})

function priceLine(m: PublicModel): string {
  if (m.price_unit && m.price_unit !== 'token' && m.unit_price != null && m.unit_price > 0) {
    return t('shell.priceUnit', { p: m.unit_price.toFixed(2) })
  }
  if (m.input_price_per_1k > 0 || m.output_price_per_1k > 0) {
    return t('shell.priceTok', { i: m.input_price_per_1k, o: m.output_price_per_1k })
  }
  return m.provider
}

function modelLabel(id: string): string {
  const a = agents.value.find((x) => x.agent_id === id)
  return a ? `${a.name} · ${a.model}` : id
}

// ---------- data loading ----------
async function loadModels() {
  try {
    publicModels.value = await listPublicModels()
    if (!selected.value || !publicModels.value.some((m) => m.id === selected.value)) {
      selected.value = chatModels.value[0]?.id ?? publicModels.value[0]?.id ?? ''
    }
    if (ttsModel.value && !ttsModels.value.includes(ttsModel.value)) ttsModel.value = ''
  } catch {
    publicModels.value = []
  }
  if (apiKey.value) {
    try {
      agents.value = await listChatAgents()
    } catch {
      agents.value = []
    }
  } else {
    agents.value = []
  }
}

// ---------- selection ----------
function selectModel(m: PublicModel) {
  selected.value = m.id
  if (selectedAgent.value) selectedAgent.value = ''
  if (compare.value && !compareList.value.includes(m.id)) compareList.value.push(m.id)
}

function onAgentChange(v: string) {
  selectedAgent.value = v
}

function toggleCompare() {
  compare.value = !compare.value
  if (compare.value) {
    if (!compareList.value.length && selected.value) compareList.value = [selected.value]
  } else if (selected.value) {
    compareList.value = [selected.value]
  }
}

function markUsed(id: string) {
  if (!id) return
  if (!usedModels.value.includes(id)) {
    usedModels.value = [id, ...usedModels.value].slice(0, 50)
    persistJson(USED_STORAGE, usedModels.value)
  }
}

// ---------- chat streaming ----------
function historyFor(prev: Round[], colIndex: number, msg: string): { role: string; content: string }[] {
  const out: { role: string; content: string }[] = []
  for (const r of prev) {
    out.push({ role: 'user', content: r.user })
    const col = r.columns[colIndex]
    if (col && col.done && !col.error && col.content) {
      out.push({ role: 'assistant', content: col.content })
    }
  }
  out.push({ role: 'user', content: msg })
  return out
}

async function streamToColumn(column: ChatCol, history: { role: string; content: string }[]) {
  try {
    const res = await fetch('/v1/chat/completions', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ model: column.model, messages: history, stream: true }),
    })
    if (!res.ok || !res.body) {
      const detail = await res.text().catch(() => '')
      throw new Error(`HTTP ${res.status} ${detail.slice(0, 300)}`)
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    const toolParts = new Map<number, { name: string; args: string }>()
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const payload = trimmed.slice(5).trim()
        if (payload === '[DONE]') continue
        try {
          const json = JSON.parse(payload)
          const delta: { content?: unknown; tool_calls?: unknown } = json.choices?.[0]?.delta ?? {}
          if (typeof delta.content === 'string' && delta.content) column.content += delta.content
          if (Array.isArray(delta.tool_calls)) {
            for (const part of delta.tool_calls as Array<{ index?: number; function?: { name?: string; arguments?: string } }>) {
              const slot = toolParts.get(part.index ?? 0) ?? { name: '', args: '' }
              if (part.function?.name) slot.name += part.function.name
              if (part.function?.arguments) slot.args += part.function.arguments
              toolParts.set(part.index ?? 0, slot)
            }
          }
          const usage = json.usage
          if (usage) {
            column.promptTokens = usage.prompt_tokens ?? 0
            column.completionTokens = usage.completion_tokens ?? 0
          }
        } catch {
          // ignore partial lines
        }
      }
    }
    const calls = [...toolParts.values()].filter((s) => s.name || s.args)
    if (calls.length) {
      const rendered = calls.map((s) => `${s.name || 'tool'}(${s.args})`).join(', ')
      column.content = (column.content ? column.content + '\n\n' : '') + t('chat.toolCall') + ' ' + rendered
    }
    if (!column.content) column.content = '…'
  } catch (e) {
    column.error = String(e)
  } finally {
    column.done = true
  }
}

// ---------- media generation (image/video/music/tts via chat composer) ----------
function pollMedia(round: Round, taskId: string) {
  const timer = window.setTimeout(async () => {
    pollTimers.delete(timer)
    const item = round.media
    if (!item) return
    try {
      const task = await mediaStatus(taskId)
      item.status = task.status
      item.error = task.error
      item.urls = task.result_urls
      item.costUsd = task.cost_usd
      if (task.status !== 'succeeded' && task.status !== 'failed') pollMedia(round, taskId)
    } catch {
      pollMedia(round, taskId)
    }
  }, 3000)
  pollTimers.add(timer)
}

async function runMedia(round: Round, text: string) {
  const item = round.media
  if (!item) return
  try {
    const body: MediaRequest = { model: item.model, type: item.kind }
    if (item.kind === 'tts') body.text = text
    else body.prompt = text
    const { task_id } = await submitMedia(body)
    pollMedia(round, task_id)
  } catch (e) {
    item.status = 'failed'
    item.error = String(e)
  }
}

function mediaStatusLabel(status: string): string {
  const known: Record<string, string> = {
    queued: t('workbench.status.queued'),
    running: t('workbench.status.running'),
    succeeded: t('workbench.status.succeeded'),
    failed: t('workbench.status.failed'),
  }
  return known[status] ?? t('workbench.status.unknown')
}

// ---------- send ----------
async function send() {
  const text = input.value.trim()
  if (!text || busy.value) return
  if (!apiKey.value) {
    error.value = t('shell.noKey')
    return
  }
  let chatModelsToUse: string[] = []
  let mediaKind: MediaCap | null = null
  if (selectedAgent.value) {
    chatModelsToUse = [selectedAgent.value]
  } else if (selected.value) {
    const m = publicModels.value.find((x) => x.id === selected.value)
    const caps = m?.capabilities ?? []
    const mediaCap = (['image', 'video', 'music', 'tts'] as MediaCap[]).find((c) => caps.includes(c))
    if (m && !caps.includes('text') && mediaCap) {
      mediaKind = mediaCap
    } else {
      chatModelsToUse = compare.value && compareList.value.length ? compareList.value : [selected.value]
    }
  } else {
    error.value = t('chat.noModels')
    return
  }
  error.value = ''
  markUsed(selectedAgent.value ? agent.value?.model || '' : selected.value)
  const round: Round = mediaKind
    ? {
        user: text,
        media: { kind: mediaKind, model: selected.value, status: 'queued', error: '', urls: [], costUsd: 0 },
        columns: [],
      }
    : {
        user: text,
        media: null,
        columns: chatModelsToUse.map((model) => ({
          model,
          content: '',
          done: false,
          error: '',
          promptTokens: 0,
          completionTokens: 0,
        })),
      }
  const previous = [...rounds.value]
  rounds.value.push(round)
  input.value = ''
  busy.value = true
  if (round.media) {
    await runMedia(round, text)
  } else {
    await Promise.all(round.columns.map((col, i) => streamToColumn(col, historyFor(previous, i, text))))
  }
  busy.value = false
}

function clearRounds() {
  rounds.value = []
  error.value = ''
}

// ---------- TTS read-aloud ----------
function onTtsChange(v: string) {
  ttsModel.value = v
  if (v) localStorage.setItem(TTS_MODEL_STORAGE, v)
  else localStorage.removeItem(TTS_MODEL_STORAGE)
}

async function speak(roundIndex: number, colIndex: number, content: string) {
  if (!ttsModel.value || !apiKey.value) {
    ElMessage.warning(t('shell.noKey'))
    return
  }
  const id = `${roundIndex}:${colIndex}`
  if (playingId.value) return
  playingId.value = id
  try {
    const res = await fetch('/v1/audio/speech', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ model: ttsModel.value, input: content.slice(0, 4096) }),
    })
    if (!res.ok) {
      const detail = await res.text().catch(() => '')
      throw new Error(`HTTP ${res.status} ${detail.slice(0, 200)}`)
    }
    const buf = await res.arrayBuffer()
    const url = URL.createObjectURL(new Blob([buf], { type: 'audio/mpeg' }))
    const audio = new Audio(url)
    audio.onended = () => URL.revokeObjectURL(url)
    await audio.play()
  } catch (e) {
    ElMessage.error(`${t('chat.ttsError')}: ${String(e)}`)
  } finally {
    playingId.value = ''
  }
}

// ---------- conversation sessions (local history) ----------
function saveCurrent() {
  if (!rounds.value.length) return
  const item: SessionItem = {
    title: (rounds.value[0].user || '…').slice(0, 30),
    at: Date.now(),
    rounds: rounds.value.slice(-40).map((r) => ({
      user: r.user.slice(0, 2000),
      media: r.media ? { ...r.media } : null,
      columns: r.columns.map((c) => ({ ...c, content: c.content.slice(0, 8000) })),
    })),
  }
  sessions.value = [item, ...sessions.value].slice(0, 12)
  persistJson(SESSIONS_STORAGE, sessions.value)
}

function newChat() {
  saveCurrent()
  rounds.value = []
  input.value = ''
  error.value = ''
}

function restoreSession(item: SessionItem, idx: number) {
  saveCurrent()
  rounds.value = item.rounds.map((r) => ({
    ...r,
    media: r.media ? { ...r.media } : null,
    columns: r.columns.map((c) => ({ ...c })),
  }))
  sessions.value.splice(idx, 1)
  persistJson(SESSIONS_STORAGE, sessions.value)
  modelPop.value = false
}

// ---------- misc ----------
function saveKey() {
  setApiKey(apiKey.value.trim())
  if (apiKey.value.trim()) ElMessage.success(t('shell.keySaved'))
  void loadModels()
}

function switchTheme() {
  theme.value = toggleTheme()
}

const gridStyle = (n: number) => ({ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` })

onMounted(() => {
  void loadModels()
})

onUnmounted(() => {
  for (const timer of pollTimers) window.clearTimeout(timer)
  pollTimers.clear()
})

watch(
  () => rounds.value,
  () => {
    void nextTick(() => {
      const el = stageRef.value
      if (el) el.scrollTop = el.scrollHeight
    })
  },
  { deep: true },
)
</script>

<template>
  <div class="lh">
    <!-- ================= left sidebar ================= -->
    <aside class="side">
      <div class="brand">
        <span class="brand-mark">✦</span>
        <div class="brand-txt">
          <span class="brand-name">KejiAPI</span>
          <span class="brand-sub">{{ t('shell.brandSub') }}</span>
        </div>
      </div>

      <div class="snav">
        <button type="button" class="snav-item on">
          <span class="snav-ic">✦</span><span>{{ t('shell.navModels') }}</span>
        </button>
        <button type="button" class="snav-item" @click="modelPop = true">
          <span class="snav-ic">⬢</span><span>{{ t('shell.navAgents') }}</span>
        </button>
        <button type="button" class="snav-item" @click="router.push('/workbench')">
          <span class="snav-ic">▣</span><span>{{ t('shell.navCreate') }}</span>
        </button>
        <button type="button" class="snav-item" @click="router.push('/console')">
          <span class="snav-ic">⌘</span><span>{{ t('shell.navApi') }}</span>
        </button>
      </div>

      <div class="stabs">
        <button
          v-for="f in FILTERS"
          :key="f.key"
          type="button"
          class="stab"
          :class="{ on: filter === f.key }"
          @click="filter = f.key"
        >
          {{ t(f.label) }}
        </button>
      </div>

      <div class="stools">
        <select v-model="provider" class="prov">
          <option value="">{{ t('shell.allVendors') }}</option>
          <option v-for="p in providers" :key="p" :value="p">{{ p }}</option>
        </select>
        <input v-model="query" class="search" :placeholder="t('shell.searchPh')" />
      </div>

      <div class="slist">
        <p v-if="!filteredModels.length" class="slist-empty">{{ t('shell.modelEmpty') }}</p>
        <button
          v-for="m in filteredModels"
          :key="m.id"
          type="button"
          class="mcard"
          :class="{ on: selected === m.id || (agent && agent.model === m.id) }"
          @click="selectModel(m)"
        >
          <span class="mc-ic" :class="'cap-' + primaryCap(m)">{{ CAP_GLYPH[primaryCap(m)] }}</span>
          <span class="mc-body">
            <span class="mc-top">
              <span class="mc-name">{{ m.id }}</span>
              <span class="mc-tag" :class="'tag-' + primaryCap(m)">{{ t(`shell.cap.${primaryCap(m)}`) }}</span>
            </span>
            <span class="mc-sub">{{ priceLine(m) }}</span>
          </span>
        </button>
      </div>

      <div class="sfoot">
        <div v-if="session.user" class="user-card">
          <span class="avatar">{{ (session.user.email || '?').slice(0, 1).toUpperCase() }}</span>
          <div class="u-txt">
            <span class="u-title">{{ session.user.email }}</span>
            <span class="u-sub">{{ formatUsd(session.user.balance_micro, session.user.balance_usd) }}</span>
          </div>
          <button type="button" class="u-btn" @click="router.push('/console')">{{ t('shell.openConsole') }}</button>
        </div>
        <div v-else class="user-card">
          <span class="avatar ghost">◉</span>
          <div class="u-txt">
            <span class="u-title">{{ t('shell.loginCta') }}</span>
            <span class="u-sub">{{ t('shell.skipHint') }}</span>
          </div>
          <div class="u-actions">
            <router-link to="/login" class="u-btn">{{ t('shell.login') }}</router-link>
            <el-popover v-model:visible="keyPop" :width="250" trigger="click" placement="top-end">
              <template #reference>
                <button type="button" class="u-btn plain">{{ t('shell.useKey') }}</button>
              </template>
              <p class="key-tip">{{ t('shell.keyPh') }}</p>
              <el-input v-model="apiKey" placeholder="sk-..." @change="saveKey" />
            </el-popover>
          </div>
        </div>
      </div>
    </aside>

    <!-- ================= main ================= -->
    <div class="main-col">
      <header class="top">
        <div class="top-left">
          <button type="button" class="cta" @click="newChat">＋ {{ t('shell.newChat') }}</button>
          <button type="button" class="t-icon" :title="t('theme.toggle')" @click="switchTheme">
            {{ theme === 'dark' ? '☀' : '☾' }}
          </button>
        </div>
        <div class="top-right">
          <el-popover v-model:visible="modelPop" :width="320" trigger="click" placement="bottom-end">
            <template #reference>
              <button type="button" class="pill" :disabled="!pillLabel">
                {{ pillLabel || t('shell.model') }} <span class="pill-caret">▾</span>
              </button>
            </template>
            <div class="pop">
              <p class="pop-title">{{ t('shell.model') }}</p>
              <div class="pop-list">
                <button
                  v-for="m in chatModels"
                  :key="m.id"
                  type="button"
                  class="pop-item"
                  :class="{ on: selected === m.id && !selectedAgent }"
                  @click="selectModel(m); modelPop = false"
                >
                  <span class="pop-name">{{ m.id }}</span>
                  <span class="pop-sub">{{ m.provider }}</span>
                </button>
                <p v-if="!chatModels.length" class="pop-empty">{{ t('shell.modelEmpty') }}</p>
              </div>
              <p class="pop-title">{{ t('shell.agent') }}</p>
              <el-select v-model="selectedAgent" size="small" @change="onAgentChange">
                <el-option :label="t('shell.agentNone')" value="" />
                <el-option
                  v-for="a in agents"
                  :key="a.agent_id"
                  :label="`${a.name} · ${a.model}`"
                  :value="a.agent_id"
                />
              </el-select>
            </div>
          </el-popover>
          <router-link v-if="!session.user" to="/login" class="cta small">{{ t('shell.loginCta') }}</router-link>
          <span v-else class="avatar">{{ (session.user.email || '?').slice(0, 1).toUpperCase() }}</span>
        </div>
      </header>

      <div ref="stageRef" class="stage">
        <div class="stage-inner">
          <div v-if="!rounds.length" class="hero">
            <svg class="spark" viewBox="0 0 64 64" width="72" height="72" aria-hidden="true">
              <path
                d="M32 2 L38 26 L62 32 L38 38 L32 62 L26 38 L2 32 L26 26 Z"
                fill="url(#sparkGrad)"
              />
              <defs>
                <linearGradient id="sparkGrad" x1="0" y1="0" x2="1" y2="1">
                  <stop offset="0%" stop-color="#38bdf8" />
                  <stop offset="100%" stop-color="#2563eb" />
                </linearGradient>
              </defs>
            </svg>
            <div class="hero-card">
              <p class="hero-name">{{ pillLabel || t('shell.model') }}</p>
              <p class="hero-desc">{{ heroDesc }}</p>
            </div>
            <p v-if="error" class="hero-error">{{ error }}</p>
          </div>

          <div v-for="(r, ri) in rounds" :key="ri" class="round">
            <div class="u-b">{{ r.user }}</div>
            <div v-if="r.media" class="a-card">
              <div class="col-head">
                <span class="col-model">{{ r.media.model }}</span>
                <span class="col-meta">
                  {{ mediaStatusLabel(r.media.status) }}
                  <template v-if="r.media.costUsd > 0"> · ${{ r.media.costUsd.toFixed(4) }}</template>
                </span>
              </div>
              <p v-if="r.media.error" class="col-error">{{ t('chat.error') }}: {{ r.media.error }}</p>
              <div v-else class="m-res">
                <template v-if="r.media.kind === 'image'">
                  <img v-for="u in r.media.urls" :key="u" :src="u" alt="" />
                </template>
                <template v-else-if="r.media.kind === 'video'">
                  <video v-for="u in r.media.urls" :key="u" :src="u" controls />
                </template>
                <template v-else-if="r.media.kind === 'music' || r.media.kind === 'tts'">
                  <audio v-for="u in r.media.urls" :key="u" :src="u" controls />
                </template>
                <template v-else>
                  <a v-for="u in r.media.urls" :key="u" :href="u" target="_blank" rel="noopener">{{ u }}</a>
                </template>
              </div>
            </div>
            <div v-else class="cols" :style="gridStyle(r.columns.length)">
              <div v-for="(col, ci) in r.columns" :key="ci" class="a-card">
                <div class="col-head">
                  <span class="col-model">{{ modelLabel(col.model) }}</span>
                  <span class="col-actions">
                    <button
                      v-if="ttsModels.length && col.done && !col.error && col.content"
                      type="button"
                      class="link-btn"
                      :disabled="!!playingId"
                      @click="speak(ri, ci, col.content)"
                    >
                      {{ t('chat.ttsPlay') }}
                    </button>
                    <span v-if="col.done && !col.error && col.promptTokens + col.completionTokens > 0" class="tok">
                      {{ col.promptTokens }}+{{ col.completionTokens }} {{ t('chat.tokens') }}
                    </span>
                  </span>
                </div>
                <p v-if="col.error" class="col-error">{{ t('chat.error') }}: {{ col.error }}</p>
                <p v-else class="content" :class="{ pending: !col.done }">{{ col.content }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="composer">
        <div class="c-panel">
          <span class="c-badge" :title="t('shell.billingNote')">{{ t('shell.billing') }} ⓘ</span>
          <textarea
            v-model="input"
            rows="3"
            class="c-input"
            :placeholder="t('shell.inputPh')"
            @keydown.enter.exact.prevent="send"
          ></textarea>
          <span class="c-attach" :title="t('shell.attachSoon')">
            <span class="c-attach-tile">＋</span>{{ t('shell.attach') }} 0/10
          </span>
          <div class="c-row">
            <span class="c-chips">
              <el-popover :width="260" trigger="click">
                <template #reference>
                  <button type="button" class="chip">
                    ⬢ {{ agent ? agent.name : t('shell.agentNoneShort') }}
                  </button>
                </template>
                <el-select v-model="selectedAgent" @change="onAgentChange">
                  <el-option :label="t('shell.agentNone')" value="" />
                  <el-option
                    v-for="a in agents"
                    :key="a.agent_id"
                    :label="`${a.name} · ${a.model}`"
                    :value="a.agent_id"
                  />
                </el-select>
              </el-popover>
              <button type="button" class="chip" :class="{ on: compare }" @click="toggleCompare">
                ⇄ {{ t('shell.compare') }}
              </button>
              <el-popover v-if="ttsModels.length" :width="260" trigger="click">
                <template #reference>
                  <button type="button" class="chip">
                    ◉ {{ ttsModel || t('shell.speakNone') }}
                  </button>
                </template>
                <el-select v-model="ttsModel" @change="onTtsChange">
                  <el-option :label="t('shell.speakNone')" value="" />
                  <el-option v-for="m in ttsModels" :key="m" :label="m" :value="m" />
                </el-select>
              </el-popover>
            </span>
            <span class="c-actions">
              <button
                v-if="rounds.length"
                type="button"
                class="link-btn"
                :disabled="busy"
                @click="clearRounds"
              >
                {{ t('shell.clear') }}
              </button>
              <button type="button" class="send" :disabled="!canSend" :title="t('shell.send')" @click="send">
                ↑
              </button>
            </span>
          </div>
        </div>
      </div>

      <div class="rail">
        <el-popover :width="320" trigger="click" placement="left">
          <template #reference>
            <button type="button" class="r-btn" :title="t('shell.history')">⧗</button>
          </template>
          <p v-if="!sessions.length" class="pop-empty">{{ t('shell.historyEmpty') }}</p>
          <div
            v-for="(s, i) in sessions"
            :key="i"
            class="hist-item"
            role="button"
            tabindex="0"
            @click="restoreSession(s, i)"
            @keydown.enter="restoreSession(s, i)"
          >
            <span class="hist-title">{{ s.title }}</span>
            <span class="hist-sub">{{ new Date(s.at).toLocaleString() }} · {{ s.rounds.length }}</span>
          </div>
        </el-popover>
        <button type="button" class="r-btn" :title="t('shell.openConsole')" @click="router.push('/console')">⌘</button>
        <a class="r-btn" :title="t('shell.help')" href="https://github.com/1528960014/KejiAPI" target="_blank" rel="noopener">?</a>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lh {
  display: flex;
  height: 100vh;
  background:
    radial-gradient(1100px 460px at 85% -10%, rgba(129, 140, 248, 0.10), transparent 60%),
    radial-gradient(900px 420px at 0% 0%, rgba(34, 211, 238, 0.07), transparent 55%),
    var(--bg);
  color: var(--text);
}

/* ---------- sidebar ---------- */
.side {
  width: 292px;
  flex: 0 0 292px;
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  background: color-mix(in srgb, var(--bg-panel) 55%, transparent);
  min-height: 0;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 16px 12px;
}
.brand-mark {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  color: #0b0b12;
  font-size: 17px;
  font-weight: 800;
}
.brand-txt {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}
.brand-name {
  font-size: 15px;
  font-weight: 800;
  background: linear-gradient(90deg, #22d3ee, #818cf8);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.brand-sub {
  font-size: 11px;
  color: var(--text-dim);
}
.snav {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
  padding: 4px 12px 10px;
}
.snav-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  border: none;
  background: transparent;
  color: var(--text-dim);
  font-size: 11px;
  padding: 8px 2px;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.snav-item:hover {
  background: var(--bg-hover);
  color: var(--text);
}
.snav-item.on {
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, transparent);
}
.snav-ic {
  font-size: 15px;
}
.stabs {
  display: flex;
  gap: 2px;
  padding: 0 12px 10px;
  flex-wrap: wrap;
}
.stab {
  border: none;
  background: transparent;
  color: var(--text-dim);
  font-size: 12px;
  padding: 4px 9px;
  border-radius: 999px;
  cursor: pointer;
}
.stab:hover {
  color: var(--text);
}
.stab.on {
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  font-weight: 600;
}
.stools {
  display: flex;
  gap: 6px;
  padding: 0 12px 10px;
}
.prov,
.search {
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text);
  border-radius: 9px;
  font-size: 12px;
  padding: 5px 8px;
  outline: none;
}
.prov {
  width: 86px;
  flex: 0 0 auto;
}
.search {
  flex: 1;
  min-width: 0;
}
.prov:focus,
.search:focus {
  border-color: var(--accent);
}
.slist {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 0;
}
.slist-empty {
  color: var(--text-dim);
  font-size: 12px;
  text-align: center;
  padding: 24px 0;
}
.mcard {
  display: flex;
  gap: 10px;
  align-items: center;
  text-align: left;
  border: 1px solid transparent;
  background: transparent;
  border-radius: 12px;
  padding: 9px 10px;
  cursor: pointer;
  transition: all 0.13s ease;
}
.mcard:hover {
  background: var(--bg-hover);
}
.mcard.on {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
}
.mc-ic {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 10px;
  font-size: 15px;
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text-dim);
}
.mc-ic.cap-text {
  color: #22d3ee;
  border-color: color-mix(in srgb, #22d3ee 40%, transparent);
}
.mc-ic.cap-image {
  color: #a78bfa;
  border-color: color-mix(in srgb, #a78bfa 40%, transparent);
}
.mc-ic.cap-video {
  color: #fb923c;
  border-color: color-mix(in srgb, #fb923c 40%, transparent);
}
.mc-ic.cap-music {
  color: #f472b6;
  border-color: color-mix(in srgb, #f472b6 40%, transparent);
}
.mc-ic.cap-tts {
  color: #34d399;
  border-color: color-mix(in srgb, #34d399 40%, transparent);
}
.mc-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.mc-top {
  display: flex;
  align-items: center;
  gap: 6px;
}
.mc-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mc-tag {
  flex: 0 0 auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  border: 1px solid var(--border);
  color: var(--text-dim);
}
.mc-tag.tag-text {
  color: #22d3ee;
  border-color: color-mix(in srgb, #22d3ee 40%, transparent);
}
.mc-tag.tag-image {
  color: #a78bfa;
  border-color: color-mix(in srgb, #a78bfa 40%, transparent);
}
.mc-tag.tag-video {
  color: #fb923c;
  border-color: color-mix(in srgb, #fb923c 40%, transparent);
}
.mc-tag.tag-music {
  color: #f472b6;
  border-color: color-mix(in srgb, #f472b6 40%, transparent);
}
.mc-tag.tag-tts {
  color: #34d399;
  border-color: color-mix(in srgb, #34d399 40%, transparent);
}
.mc-sub {
  font-size: 11px;
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sfoot {
  border-top: 1px solid var(--border);
  padding: 10px 12px;
}
.user-card {
  display: flex;
  align-items: center;
  gap: 9px;
}
.avatar {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  color: #0b0b12;
  font-size: 13px;
  font-weight: 700;
}
.avatar.ghost {
  background: var(--bg-hover);
  color: var(--text-dim);
  font-weight: 400;
}
.u-txt {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  line-height: 1.3;
}
.u-title {
  font-size: 12px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.u-sub {
  font-size: 11px;
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.u-actions {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.u-btn {
  border: 1px solid color-mix(in srgb, var(--accent) 45%, transparent);
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent);
  font-size: 12px;
  padding: 3px 12px;
  border-radius: 999px;
  cursor: pointer;
  text-decoration: none;
  display: inline-block;
  text-align: center;
}
.u-btn.plain {
  background: transparent;
  color: var(--text-dim);
  border-color: var(--border);
}
.key-tip {
  font-size: 12px;
  color: var(--text-dim);
  margin: 0 0 8px;
}

/* ---------- main column ---------- */
.main-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  position: relative;
  min-height: 0;
}
.top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  gap: 12px;
}
.top-left,
.top-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cta {
  border: none;
  background: linear-gradient(135deg, #fbbf24, #f59e0b);
  color: #1a1205;
  font-size: 13px;
  font-weight: 700;
  padding: 7px 16px;
  border-radius: 999px;
  cursor: pointer;
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.cta:hover {
  filter: brightness(1.06);
}
.cta.small {
  padding: 5px 14px;
  font-size: 12px;
}
.t-icon {
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text);
  width: 32px;
  height: 32px;
  border-radius: 50%;
  cursor: pointer;
  font-size: 14px;
}
.t-icon:hover {
  background: var(--bg-hover);
}
.pill {
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text);
  font-size: 13px;
  padding: 6px 14px;
  border-radius: 999px;
  cursor: pointer;
  max-width: 340px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pill:disabled {
  cursor: default;
  color: var(--text-dim);
}
.pill-caret {
  font-size: 10px;
  color: var(--text-dim);
}
.pop {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.pop-title {
  margin: 6px 0 2px;
  font-size: 12px;
  font-weight: 700;
  color: var(--text-dim);
}
.pop-title:first-child {
  margin-top: 0;
}
.pop-list {
  max-height: 260px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.pop-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text);
  text-align: left;
  padding: 6px 9px;
  border-radius: 9px;
  cursor: pointer;
  font-size: 13px;
}
.pop-item:hover {
  background: var(--bg-hover);
}
.pop-item.on {
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}
.pop-name {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.pop-sub {
  font-size: 11px;
  color: var(--text-dim);
}
.pop-empty {
  color: var(--text-dim);
  font-size: 12px;
  padding: 8px 0;
}

/* ---------- stage ---------- */
.stage {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding: 8px 20px 12px;
}
.stage-inner {
  max-width: 860px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 9vh 0 40px;
}
.spark {
  filter: drop-shadow(0 0 24px rgba(56, 189, 248, 0.45));
}
.hero-card {
  max-width: 640px;
  border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
  background: color-mix(in srgb, var(--bg-panel) 80%, transparent);
  border-radius: 14px;
  padding: 14px 20px;
  text-align: center;
  box-shadow: 0 0 40px rgba(34, 211, 238, 0.08);
}
.hero-name {
  font-size: 15px;
  font-weight: 700;
  margin: 0 0 6px;
  color: var(--accent);
}
.hero-desc {
  font-size: 13px;
  color: var(--text-dim);
  margin: 0;
}
.hero-error {
  color: #f87171;
  font-size: 13px;
}
.round {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.u-b {
  align-self: flex-end;
  max-width: 82%;
  background: color-mix(in srgb, var(--accent) 16%, var(--bg-panel));
  border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
  border-radius: 14px 14px 4px 14px;
  padding: 10px 14px;
  font-size: 14px;
  white-space: pre-wrap;
  word-break: break-word;
}
.cols {
  display: grid;
  gap: 10px;
}
.a-card {
  border: 1px solid var(--border);
  background: color-mix(in srgb, var(--bg-panel) 82%, transparent);
  border-radius: 4px 14px 14px 14px;
  padding: 12px 14px;
}
.col-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 8px;
}
.col-model {
  font-size: 12px;
  font-weight: 700;
  color: var(--accent);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.col-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}
.col-meta,
.tok {
  font-size: 11px;
  color: var(--text-dim);
}
.content {
  margin: 0;
  font-size: 14px;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
}
.content.pending {
  color: var(--text-dim);
}
.col-error {
  margin: 0;
  font-size: 13px;
  color: #f87171;
  white-space: pre-wrap;
  word-break: break-word;
}
.link-btn {
  border: none;
  background: transparent;
  color: var(--accent);
  font-size: 12px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
}
.link-btn:hover {
  background: color-mix(in srgb, var(--accent) 12%, transparent);
}
.m-res {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.m-res img,
.m-res video {
  max-width: 100%;
  max-height: 380px;
  border-radius: 10px;
}
.m-res audio {
  width: 100%;
}
.m-res a {
  color: var(--accent);
  font-size: 12px;
  word-break: break-all;
}

/* ---------- composer ---------- */
.composer {
  padding: 6px 20px 16px;
}
.c-panel {
  position: relative;
  max-width: 820px;
  margin: 0 auto;
  border: 1px solid var(--border);
  background: color-mix(in srgb, var(--bg-panel) 92%, transparent);
  border-radius: 18px;
  padding: 14px 16px 10px;
  box-shadow: var(--shadow);
}
.c-panel:focus-within {
  border-color: color-mix(in srgb, var(--accent) 55%, transparent);
}
.c-badge {
  position: absolute;
  top: -11px;
  right: 16px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 999px;
  font-size: 11px;
  color: var(--text-dim);
  padding: 2px 10px;
  cursor: help;
}
.c-input {
  width: 100%;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text);
  font-size: 14px;
  line-height: 1.6;
  font-family: inherit;
  min-height: 44px;
}
.c-input::placeholder {
  color: var(--text-dim);
}
.c-attach {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 11px;
  color: var(--text-dim);
  padding-bottom: 8px;
  cursor: help;
  user-select: none;
}
.c-attach-tile {
  display: grid;
  place-items: center;
  width: 34px;
  height: 44px;
  border: 1px dashed var(--border);
  border-radius: 9px;
  font-size: 16px;
  color: var(--text-dim);
}
.c-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.c-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.chip {
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text);
  font-size: 12px;
  padding: 5px 12px;
  border-radius: 999px;
  cursor: pointer;
  max-width: 240px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.chip:hover {
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  color: var(--accent);
}
.chip.on {
  color: var(--accent);
  border-color: color-mix(in srgb, var(--accent) 55%, transparent);
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}
.c-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
}
.send {
  width: 38px;
  height: 38px;
  border-radius: 50%;
  border: none;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  color: #0b0b12;
  font-size: 17px;
  font-weight: 800;
  cursor: pointer;
  display: grid;
  place-items: center;
}
.send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* ---------- right rail ---------- */
.rail {
  position: absolute;
  right: 14px;
  bottom: 110px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  z-index: 20;
}
.r-btn {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text-dim);
  font-size: 14px;
  cursor: pointer;
  display: grid;
  place-items: center;
  text-decoration: none;
}
.r-btn:hover {
  color: var(--accent);
  border-color: color-mix(in srgb, var(--accent) 45%, transparent);
}
.hist-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 8px 10px;
  margin-bottom: 6px;
  cursor: pointer;
}
.hist-item:hover {
  background: var(--bg-hover);
}
.hist-title {
  font-size: 13px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.hist-sub {
  font-size: 11px;
  color: var(--text-dim);
}

@media (max-width: 900px) {
  .side {
    display: none;
  }
}
</style>

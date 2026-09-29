<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  authHeaders,
  getApiKey,
  listChatAgents,
  listModels,
  listPublicModels,
  mediaStatus,
  setApiKey,
  submitMedia,
  type ChatAgent,
  type MediaRequest,
  type PublicModel,
} from '../api/client'

const { t } = useI18n()

const TTS_MODEL_STORAGE = 'modelhub.ttsModel'

type Modality = 'text' | 'image' | 'video' | 'music' | 'tts'

interface Column {
  model: string
  content: string
  done: boolean
  error: string
  promptTokens: number
  completionTokens: number
}

interface Round {
  user: string
  columns: Column[]
}

interface MediaRound {
  id: number
  kind: Exclude<Modality, 'text'>
  model: string
  prompt: string
  status: string
  error: string
  resultUrls: string[]
  costUsd: number
}

const apiKey = ref(getApiKey())
const models = ref<string[]>([])
const selectedModels = ref<string[]>([])
const agents = ref<ChatAgent[]>([])
const selectedAgent = ref('')
const input = ref('')
const busy = ref(false)
const error = ref('')
const rounds = ref<Round[]>([])

const modality = ref<Modality>('text')

// P3-4: real-time speech — optional TTS model for reading replies aloud.
const ttsModels = ref<string[]>([])
const ttsModel = ref(localStorage.getItem(TTS_MODEL_STORAGE) || '')
const playingId = ref('')

// --- multimodal generation (image / video / music / tts) ---
const publicModels = ref<PublicModel[]>([])
const mediaModel = ref('')
const mediaPrompt = ref('')
const mediaSize = ref('')
const mediaDuration = ref('')
const mediaBusy = ref(false)
const mediaRounds = ref<MediaRound[]>([])
const mediaError = ref('')
const timers = new Map<number, number>()
let nextMediaId = 1

const MODALITIES: { key: Modality; icon: string }[] = [
  { key: 'text', icon: '💬' },
  { key: 'image', icon: '🎨' },
  { key: 'video', icon: '🎬' },
  { key: 'music', icon: '🎵' },
  { key: 'tts', icon: '🎙' },
]

const mediaModels = computed<PublicModel[]>(() => {
  const kind = modality.value
  if (kind === 'text') return []
  return publicModels.value.filter((m) => (m.capabilities || []).includes(kind))
})

async function loadModels() {
  error.value = ''
  if (!apiKey.value) {
    models.value = []
    return
  }
  try {
    const list = await listModels()
    models.value = list.map((m) => m.id)
    if (selectedAgent.value) {
      // an agent owns the model choice
      selectedModels.value = [selectedAgent.value]
    } else {
      // keep previously selected models that still exist, default to first
      selectedModels.value = selectedModels.value.filter((m) => models.value.includes(m))
      if (!selectedModels.value.length && models.value.length) {
        selectedModels.value = [models.value[0]]
      }
    }
  } catch {
    models.value = []
  }
}

async function loadAgents() {
  if (!apiKey.value) {
    agents.value = []
    return
  }
  try {
    agents.value = await listChatAgents()
  } catch {
    agents.value = []
  }
}

async function loadTtsModels() {
  try {
    const list = await listPublicModels()
    publicModels.value = list
    ttsModels.value = list
      .filter((m) => (m.capabilities || []).includes('tts'))
      .map((m) => m.id)
    if (ttsModel.value && !ttsModels.value.includes(ttsModel.value)) {
      ttsModel.value = ''
    }
    // default media model per modality: first match
    if (!mediaModel.value || !mediaModels.value.some((m) => m.id === mediaModel.value)) {
      mediaModel.value = mediaModels.value[0]?.id ?? ''
    }
  } catch {
    publicModels.value = []
    ttsModels.value = []
  }
}

function onModalityChange() {
  error.value = ''
  mediaError.value = ''
  mediaModel.value = mediaModels.value[0]?.id ?? ''
}

function onTtsChange(value: string) {
  ttsModel.value = value
  if (value) {
    localStorage.setItem(TTS_MODEL_STORAGE, value)
  } else {
    localStorage.removeItem(TTS_MODEL_STORAGE)
  }
}

onMounted(() => {
  void loadModels()
  void loadAgents()
  void loadTtsModels()
})

onUnmounted(() => {
  for (const timer of timers.values()) window.clearInterval(timer)
  timers.clear()
})

function onKeyChange() {
  setApiKey(apiKey.value.trim())
  void loadModels()
  void loadAgents()
}

function onAgentChange(value: string) {
  selectedAgent.value = value
  const agent = agents.value.find((a) => a.agent_id === value)
  if (agent) {
    selectedModels.value = [agent.agent_id]
  } else if (!selectedModels.value.length && models.value.length) {
    selectedModels.value = [models.value[0]]
  }
}

function modelLabel(id: string): string {
  const agent = agents.value.find((a) => a.agent_id === id)
  return agent ? `${agent.name} · ${agent.model}` : id
}

// history for one model: all previous user turns plus that model's own
// successful assistant replies, then the new user message.
function historyFor(roundsSoFar: Round[], colIndex: number, newMessage: string): { role: string; content: string }[] {
  const out: { role: string; content: string }[] = []
  for (const round of roundsSoFar) {
    out.push({ role: 'user', content: round.user })
    const col = round.columns[colIndex]
    if (col && col.done && !col.error && col.content) {
      out.push({ role: 'assistant', content: col.content })
    }
  }
  out.push({ role: 'user', content: newMessage })
  return out
}

async function streamToColumn(column: Column, history: { role: string; content: string }[]) {
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
    // streamed tool_call fragments keyed by call index (agent templates, P3-1)
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
    // The web chat page does not execute tools; render the calls so the user
    // sees what the model asked for (SDK clients do the real execution).
    const calls = [...toolParts.values()].filter((s) => s.name || s.args)
    if (calls.length) {
      const rendered = calls.map((s) => `${s.name || 'tool'}(${s.args})`).join(', ')
      column.content = (column.content ? column.content + '\n\n' : '') + t('chat.toolCall') + ' ' + rendered
    }
    if (!column.content) {
      column.content = '…'
    }
  } catch (e) {
    column.error = String(e)
  } finally {
    column.done = true
  }
}

async function send() {
  const text = input.value.trim()
  if (!text || busy.value) return
  if (!apiKey.value) {
    error.value = t('chat.noKey')
    return
  }
  if (!selectedModels.value.length) {
    error.value = t('chat.noModels')
    return
  }
  error.value = ''
  const previous = [...rounds.value]
  const round: Round = {
    user: text,
    columns: selectedModels.value.map((m) => ({
      model: m,
      content: '',
      done: false,
      error: '',
      promptTokens: 0,
      completionTokens: 0,
    })),
  }
  rounds.value.push(round)
  input.value = ''
  busy.value = true
  await Promise.all(
    round.columns.map((col, i) => streamToColumn(col, historyFor(previous, i, text))),
  )
  busy.value = false
}

function clearRounds() {
  rounds.value = []
  error.value = ''
}

// P3-4: synthesize a reply with the selected TTS model and play it.
async function speak(roundIndex: number, colIndex: number, content: string) {
  if (!ttsModel.value || !apiKey.value) {
    ElMessage.warning(t('chat.noKey'))
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

// --- multimodal submit + polling ---

async function generateMedia() {
  const kind = modality.value
  if (kind === 'text' || mediaBusy.value) return
  if (!apiKey.value) {
    mediaError.value = t('chat.noKey')
    return
  }
  if (!mediaModel.value) {
    mediaError.value = t('chat.mediaNoModel')
    return
  }
  const prompt = mediaPrompt.value.trim()
  if (!prompt) {
    mediaError.value = t('chat.mediaNeedPrompt')
    return
  }
  mediaError.value = ''
  mediaBusy.value = true
  try {
    const body: MediaRequest = {
      model: mediaModel.value,
      type: kind,
    }
    if (kind === 'tts') {
      body.text = prompt
    } else {
      body.prompt = prompt
    }
    if (kind === 'image' && mediaSize.value.trim()) body.size = mediaSize.value.trim()
    if ((kind === 'video' || kind === 'music') && mediaDuration.value.trim()) {
      body.duration = mediaDuration.value.trim()
    }
    const { task_id } = await submitMedia(body)
    const id = nextMediaId++
    const round: MediaRound = {
      id,
      kind,
      model: mediaModel.value,
      prompt,
      status: 'queued',
      error: '',
      resultUrls: [],
      costUsd: 0,
    }
    mediaRounds.value.unshift(round)
    startPolling(id, task_id)
  } catch (e) {
    mediaError.value = String(e)
  } finally {
    mediaBusy.value = false
  }
}

function stopPolling(id: number) {
  const timer = timers.get(id)
  if (timer !== undefined) {
    window.clearInterval(timer)
    timers.delete(id)
  }
}

async function pollOnce(id: number, taskId: string) {
  const round = mediaRounds.value.find((r) => r.id === id)
  if (!round) {
    stopPolling(id)
    return
  }
  try {
    const task = await mediaStatus(taskId)
    round.status = task.status
    round.error = task.error
    round.resultUrls = task.result_urls
    round.costUsd = task.cost_usd
    if (task.status === 'succeeded' || task.status === 'failed') {
      stopPolling(id)
    }
  } catch {
    // transient network error — keep polling
  }
}

function startPolling(id: number, taskId: string) {
  void pollOnce(id, taskId)
  const timer = window.setInterval(() => void pollOnce(id, taskId), 3000)
  timers.set(id, timer)
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

const gridStyle = (n: number) => ({ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` })
</script>

<template>
  <div class="page">
    <h2>{{ t('chat.title') }}</h2>

    <div class="modality">
      <button
        v-for="m in MODALITIES"
        :key="m.key"
        type="button"
        class="modality-btn"
        :class="{ active: modality === m.key }"
        @click="modality = m.key; onModalityChange()"
      >
        <span class="mod-icon">{{ m.icon }}</span>
        {{ t(`chat.tab${m.key[0].toUpperCase()}${m.key.slice(1)}`) }}
      </button>
    </div>

    <!-- ============ text chat ============ -->
    <template v-if="modality === 'text'">
      <div class="card config">
        <div class="row">
          <label>
            {{ t('chat.apiKey') }}
            <el-input v-model="apiKey" :placeholder="t('chat.apiKeyPlaceholder')" @change="onKeyChange" />
          </label>
          <label>
            {{ t('chat.agents') }}
            <el-select
              v-model="selectedAgent"
              style="min-width: 220px"
              @change="onAgentChange"
            >
              <el-option :label="t('chat.agentNone')" value="" />
              <el-option
                v-for="a in agents"
                :key="a.agent_id"
                :label="`${a.name} · ${a.model}`"
                :value="a.agent_id"
                :title="a.description"
              />
            </el-select>
          </label>
          <label>
            {{ t('chat.models') }}
            <el-select
              v-model="selectedModels"
              multiple
              collapse-tags
              collapse-tags-tooltip
              :disabled="!models.length || !!selectedAgent"
              style="min-width: 320px"
            >
              <el-option
                v-if="!models.length"
                :label="apiKey ? t('chat.loadingModels') : t('chat.noModels')"
                value=""
                disabled
              />
              <el-option v-for="m in models" :key="m" :label="m" :value="m" />
            </el-select>
          </label>
          <label v-if="ttsModels.length">
            {{ t('chat.ttsModel') }}
            <el-select
              v-model="ttsModel"
              style="min-width: 200px"
              @change="onTtsChange"
            >
              <el-option :label="t('chat.ttsNone')" value="" />
              <el-option v-for="m in ttsModels" :key="m" :label="m" :value="m" />
            </el-select>
          </label>
          <el-button v-if="rounds.length" @click="clearRounds">{{ t('chat.clear') }}</el-button>
        </div>
        <p class="muted hint">{{ selectedAgent ? t('chat.agentHint') : t('chat.compareHint') }}</p>
      </div>

      <div class="chat">
        <p v-if="!rounds.length && !busy" class="muted empty">{{ t('chat.placeholder') }}</p>
        <div v-for="(round, ri) in rounds" :key="ri" class="round">
          <div class="msg user">
            <div class="role">{{ t('chat.you') }}</div>
            <p class="content">{{ round.user }}</p>
          </div>
          <div class="cols" :style="gridStyle(round.columns.length)">
            <div v-for="(col, ci) in round.columns" :key="col.model" class="card col">
              <div class="col-head">
                <span class="col-model">{{ modelLabel(col.model) }}</span>
                <span class="col-actions">
                  <el-button
                    v-if="ttsModel && col.done && !col.error && col.content"
                    size="small"
                    text
                    :loading="playingId === `${ri}:${ci}`"
                    :title="t('chat.ttsPlay')"
                    @click="speak(ri, ci, col.content)"
                  >
                    {{ t('chat.ttsPlay') }}
                  </el-button>
                  <span v-if="col.done && !col.error && col.promptTokens + col.completionTokens > 0" class="muted">
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

      <div class="card input-card">
        <div v-if="error" class="error">{{ error }}</div>
        <div class="input-row">
          <el-input
            v-model="input"
            type="textarea"
            :rows="2"
            :placeholder="t('chat.placeholder')"
            :disabled="busy"
            @keydown.enter.exact.prevent="send"
          />
          <el-button type="primary" :loading="busy" @click="send">
            {{ t('chat.send') }}
          </el-button>
        </div>
      </div>
    </template>

    <!-- ============ media generation ============ -->
    <template v-else>
      <div class="card config">
        <div class="row">
          <label>
            {{ t('chat.apiKey') }}
            <el-input v-model="apiKey" :placeholder="t('chat.apiKeyPlaceholder')" @change="onKeyChange" />
          </label>
          <label style="min-width: 260px">
            {{ t('chat.mediaModel') }}
            <el-select v-model="mediaModel">
              <el-option
                v-if="!mediaModels.length"
                :label="t('chat.mediaNoModel')"
                value=""
                disabled
              />
              <el-option v-for="m in mediaModels" :key="m.id" :label="m.id" :value="m.id" />
            </el-select>
          </label>
          <template v-if="modality === 'image'">
            <label>
              {{ t('chat.mediaSize') }}
              <el-input v-model="mediaSize" placeholder="1024x1024" style="width: 140px" />
            </label>
          </template>
          <template v-else-if="modality === 'video' || modality === 'music'">
            <label>
              {{ t('chat.mediaDuration') }}
              <el-input v-model="mediaDuration" placeholder="10" style="width: 110px" />
            </label>
          </template>
        </div>
        <div v-if="error || mediaError" class="error media-error">
          {{ mediaError || error }}
        </div>
      </div>

      <div class="card input-card media-input">
        <div class="input-row">
          <el-input
            v-model="mediaPrompt"
            type="textarea"
            :rows="3"
            :placeholder="modality === 'tts' ? t('workbench.textPlaceholder') : t('workbench.promptPlaceholder')"
            :disabled="mediaBusy"
            @keydown.ctrl.enter="generateMedia"
          />
          <el-button type="primary" :loading="mediaBusy" @click="generateMedia">
            {{ mediaBusy ? t('chat.mediaGenerating') : t('chat.mediaGenerate') }}
          </el-button>
        </div>
      </div>

      <div class="media-list">
        <div v-for="r in mediaRounds" :key="r.id" class="card media-card">
          <div class="media-head">
            <span class="col-model">{{ r.model }}</span>
            <el-tag :type="r.status === 'succeeded' ? 'success' : r.status === 'failed' ? 'danger' : 'info'" size="small">
              {{ mediaStatusLabel(r.status) }}
            </el-tag>
            <span v-if="r.status === 'succeeded' && r.costUsd > 0" class="muted">
              ${{ r.costUsd.toFixed(6) }}
            </span>
          </div>
          <p class="media-prompt">{{ r.prompt }}</p>
          <p v-if="r.status === 'failed'" class="col-error">{{ r.error || t('chat.error') }}</p>
          <div v-else-if="r.status === 'succeeded' && r.resultUrls.length" class="media-results" :class="`kind-${r.kind}`">
            <template v-for="(url, i) in r.resultUrls" :key="i">
              <a :href="url" target="_blank" rel="noreferrer" class="media-item" :title="url">
                <img v-if="r.kind === 'image'" :src="url" :alt="r.prompt" loading="lazy" />
                <video v-else-if="r.kind === 'video'" :src="url" controls preload="metadata"></video>
                <div v-else class="media-audio">
                  <span class="audio-icon">{{ r.kind === 'music' ? '🎵' : '🎙' }}</span>
                  <audio :src="url" controls preload="metadata"></audio>
                </div>
              </a>
            </template>
          </div>
        </div>
        <p v-if="!mediaRounds.length" class="muted empty">{{ t('chat.mediaEmpty') }}</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
.modality {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.modality-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text-dim);
  font-size: 14px;
  font-weight: 600;
  padding: 8px 16px;
  border-radius: 999px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.modality-btn:hover {
  color: var(--text);
  border-color: var(--accent);
}
.modality-btn.active {
  color: var(--accent-contrast);
  background: var(--accent);
  border-color: var(--accent);
}
.mod-icon {
  font-size: 15px;
}
.config .row {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  align-items: flex-end;
}
.config label {
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
.chat {
  margin-top: 16px;
}
.empty {
  text-align: center;
  padding: 40px 0;
}
.round {
  margin-bottom: 20px;
}
.msg .role {
  font-size: 12px;
  color: var(--text-dim);
  margin-bottom: 4px;
}
.msg .content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
}
.msg.user {
  border-left: 3px solid var(--accent);
  padding-left: 10px;
  margin-bottom: 12px;
}
.cols {
  display: grid;
  gap: 12px;
}
.col {
  padding: 12px 14px;
  min-height: 64px;
}
.col-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
}
.col-actions {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.col-model {
  font-weight: 600;
  font-size: 13px;
  color: var(--accent);
}
.col .content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  font-size: 14px;
  line-height: 1.6;
}
.col .content.pending {
  color: var(--text-dim);
}
.col-error {
  margin: 0;
  color: #f87171;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-word;
}
.error {
  color: #f87171;
  margin-bottom: 8px;
  font-size: 13px;
}
.media-error {
  margin: 10px 0 0;
}
.input-row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
.input-card {
  margin-top: 4px;
}
.media-input {
  margin-top: 16px;
}
.media-list {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.media-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.media-prompt {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-dim);
  white-space: pre-wrap;
  word-break: break-word;
}
.media-results {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}
.media-item {
  display: block;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--border);
  background: var(--code-bg);
}
.media-item img,
.media-item video {
  display: block;
  width: 100%;
  min-height: 120px;
  object-fit: cover;
}
.media-audio {
  padding: 14px;
  display: flex;
  align-items: center;
  gap: 10px;
}
.media-audio audio {
  width: 100%;
}
.audio-icon {
  font-size: 20px;
}
</style>

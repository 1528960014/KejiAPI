<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { authHeaders, getApiKey, listModels, setApiKey } from '../api/client'

const { t } = useI18n()

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

const apiKey = ref(getApiKey())
const models = ref<string[]>([])
const selectedModels = ref<string[]>([])
const input = ref('')
const busy = ref(false)
const error = ref('')
const rounds = ref<Round[]>([])

async function loadModels() {
  error.value = ''
  if (!apiKey.value) {
    models.value = []
    return
  }
  try {
    const list = await listModels()
    models.value = list.map((m) => m.id)
    // keep previously selected models that still exist, default to first
    selectedModels.value = selectedModels.value.filter((m) => models.value.includes(m))
    if (!selectedModels.value.length && models.value.length) {
      selectedModels.value = [models.value[0]]
    }
  } catch {
    models.value = []
  }
}

onMounted(loadModels)

function onKeyChange() {
  setApiKey(apiKey.value.trim())
  void loadModels()
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
          const delta: string = json.choices?.[0]?.delta?.content ?? ''
          if (delta) column.content += delta
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

const gridStyle = (n: number) => ({ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` })
</script>

<template>
  <div class="page">
    <h2>{{ t('chat.title') }}</h2>
    <div class="card config">
      <div class="row">
        <label>
          {{ t('chat.apiKey') }}
          <el-input v-model="apiKey" :placeholder="t('chat.apiKeyPlaceholder')" @change="onKeyChange" />
        </label>
        <label>
          {{ t('chat.models') }}
          <el-select
            v-model="selectedModels"
            multiple
            collapse-tags
            collapse-tags-tooltip
            :disabled="!models.length"
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
        <el-button v-if="rounds.length" @click="clearRounds">{{ t('chat.clear') }}</el-button>
      </div>
      <p class="muted hint">{{ t('chat.compareHint') }}</p>
    </div>

    <div class="chat">
      <p v-if="!rounds.length && !busy" class="muted empty">{{ t('chat.placeholder') }}</p>
      <div v-for="(round, ri) in rounds" :key="ri" class="round">
        <div class="msg user">
          <div class="role">{{ t('chat.you') }}</div>
          <p class="content">{{ round.user }}</p>
        </div>
        <div class="cols" :style="gridStyle(round.columns.length)">
          <div v-for="col in round.columns" :key="col.model" class="card col">
            <div class="col-head">
              <span class="col-model">{{ col.model }}</span>
              <span v-if="col.done && !col.error && col.promptTokens + col.completionTokens > 0" class="muted">
                {{ col.promptTokens }}+{{ col.completionTokens }} {{ t('chat.tokens') }}
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
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
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
.input-row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
}
.input-card {
  margin-top: 4px;
}
</style>
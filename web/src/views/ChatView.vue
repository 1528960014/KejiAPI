<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { authHeaders, getApiKey, listModels, setApiKey } from '../api/client'

const { t } = useI18n()

const apiKey = ref(getApiKey())
const models = ref<string[]>([])
const model = ref('')
const input = ref('')
const busy = ref(false)
const error = ref('')
const messages = reactive<{ role: string; content: string }[]>([])

async function loadModels() {
  error.value = ''
  if (!apiKey.value) {
    models.value = []
    return
  }
  try {
    const list = await listModels()
    models.value = list.map((m) => m.id)
    if (models.value.length && !models.value.includes(model.value)) {
      model.value = models.value[0]
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

async function send() {
  const text = input.value.trim()
  if (!text || busy.value) return
  if (!apiKey.value) {
    error.value = t('chat.noKey')
    return
  }
  if (!model.value) {
    error.value = t('chat.noModels')
    return
  }
  error.value = ''
  messages.push({ role: 'user', content: text })
  const reply: { role: string; content: string } = { role: 'assistant', content: '' }
  messages.push(reply)
  input.value = ''
  busy.value = true
  try {
    const res = await fetch('/v1/chat/completions', {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        model: model.value,
        messages: messages.slice(0, -1),
        stream: true,
      }),
    })
    if (!res.ok || !res.body) {
      const detail = await res.text().catch(() => '')
      throw new Error(`HTTP ${res.status} ${detail}`)
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
          if (delta) reply.content += delta
        } catch {
          // ignore partial lines
        }
      }
    }
    if (!reply.content) {
      reply.content = '(empty response)'
    }
  } catch (e) {
    error.value = `${t('chat.error')}: ${String(e)}`
  } finally {
    busy.value = false
  }
}
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
          {{ t('chat.model') }}
          <el-select v-model="model" :disabled="!models.length" style="width: 280px">
            <el-option
              v-if="!models.length"
              :label="apiKey ? t('chat.loadingModels') : t('chat.noModels')"
              value=""
              disabled
            />
            <el-option v-for="m in models" :key="m" :label="m" :value="m" />
          </el-select>
        </label>
      </div>
    </div>

    <div class="card chat">
      <div class="messages">
        <div v-if="!messages.length" class="muted empty">…</div>
        <div
          v-for="(m, i) in messages"
          :key="i"
          class="msg"
          :class="m.role"
        >
          <div class="role">{{ m.role }}</div>
          <pre class="content">{{ m.content }}</pre>
        </div>
      </div>
      <div v-if="error" class="error">{{ error }}</div>
      <div class="input-row">
        <el-input
          v-model="input"
          type="textarea"
          :rows="2"
          :placeholder="t('chat.placeholder')"
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
}
.config label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.chat {
  margin-top: 16px;
}
.messages {
  max-height: 420px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 8px 0;
}
.empty {
  text-align: center;
  padding: 40px 0;
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
}
.error {
  color: #f87171;
  margin-top: 8px;
  font-size: 13px;
}
.input-row {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  margin-top: 12px;
}
</style>
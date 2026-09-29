<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getApiKey,
  listPublicModels,
  mediaStatus,
  setApiKey,
  submitMedia,
  type MediaRequest,
  type MediaTask,
  type PublicModel,
} from '../api/client'

const { t } = useI18n()

type Tab = 'image' | 'video' | 'music' | 'tts'

const TASKS_STORAGE = 'modelhub-tasks'
const POLL_MS = 3000

const apiKey = ref(getApiKey())
const tab = ref<Tab>('image')
const models = ref<PublicModel[]>([])
const selectedModel = ref('')
const prompt = ref('')
const text = ref('')
const n = ref(1)
const size = ref('1024x1024')
const duration = ref('5')
const busy = ref(false)
const error = ref('')
const tasks = ref<MediaTask[]>([])
let pollTimer: number | null = null

const modelsForTab = computed(() =>
  models.value.filter((m) => (m.capabilities || []).includes(tab.value)),
)

const selectedModelInfo = computed(
  () => models.value.find((m) => m.id === selectedModel.value),
)

const finishedCount = computed(() => tasks.value.length - activeTasks().length)

const priceHint = computed(() => {
  const m = selectedModelInfo.value
  if (!m || !m.price_unit || m.price_unit === 'token' || m.unit_price === undefined) {
    return ''
  }
  return `${t('workbench.priceHint')}: $${m.unit_price} / ${m.price_unit}`
})

async function loadModels() {
  try {
    models.value = await listPublicModels()
    if (!modelsForTab.value.some((m) => m.id === selectedModel.value)) {
      selectedModel.value = modelsForTab.value[0]?.id ?? ''
    }
  } catch {
    models.value = []
  }
}

function onKeyChange() {
  setApiKey(apiKey.value.trim())
}

watch(tab, () => {
  if (!modelsForTab.value.some((m) => m.id === selectedModel.value)) {
    selectedModel.value = modelsForTab.value[0]?.id ?? ''
  }
})

function isActive(t: MediaTask): boolean {
  return t.status !== 'succeeded' && t.status !== 'failed'
}

function activeTasks(): MediaTask[] {
  return tasks.value.filter(isActive)
}

function saveTaskRefs() {
  localStorage.setItem(
    TASKS_STORAGE,
    JSON.stringify(tasks.value.map((t) => ({ id: t.task_id, model: t.model, type: t.type }))),
  )
}

function loadTaskRefs() {
  try {
    const raw = localStorage.getItem(TASKS_STORAGE)
    if (!raw) return
    const refs = JSON.parse(raw) as { id: string; model: string; type: string }[]
    tasks.value = refs.map((r) => ({
      task_id: r.id,
      status: 'unknown',
      type: r.type,
      model: r.model,
      result_urls: [],
      cost_usd: 0,
      error: '',
      created_at: '',
      updated_at: '',
    }))
  } catch {
    localStorage.removeItem(TASKS_STORAGE)
  }
}

async function submit() {
  error.value = ''
  if (!apiKey.value) {
    error.value = t('workbench.noKey')
    return
  }
  if (!selectedModel.value) {
    error.value = t('workbench.noModel')
    return
  }
  const isTts = tab.value === 'tts'
  const content = isTts ? text.value.trim() : prompt.value.trim()
  if (!content) {
    error.value = t('workbench.noPrompt')
    return
  }
  const body: MediaRequest = { model: selectedModel.value, type: tab.value }
  if (isTts) body.text = content
  else body.prompt = content
  if (tab.value === 'image') {
    body.n = n.value
    body.size = size.value.trim() || undefined
  }
  if (tab.value === 'video') body.duration = duration.value.trim() || undefined
  busy.value = true
  try {
    const res = await submitMedia(body)
    tasks.value.unshift({
      task_id: res.task_id,
      status: res.status,
      type: tab.value,
      model: selectedModel.value,
      result_urls: [],
      cost_usd: 0,
      error: '',
      created_at: '',
      updated_at: '',
    })
    saveTaskRefs()
    if (isTts) text.value = ''
    else prompt.value = ''
    startPolling()
  } catch (e) {
    error.value = String((e as Error)?.message ?? e)
  } finally {
    busy.value = false
  }
}

async function refresh() {
  const actives = activeTasks()
  if (!actives.length) {
    stopPolling()
    return
  }
  await Promise.all(
    actives.map(async (task) => {
      try {
        Object.assign(task, await mediaStatus(task.task_id))
      } catch {
        // keep last known state; retry on the next tick
      }
    }),
  )
  if (!activeTasks().length) stopPolling()
}

function startPolling() {
  stopPolling()
  if (activeTasks().length) {
    pollTimer = window.setInterval(() => void refresh(), POLL_MS)
  }
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function clearFinished() {
  tasks.value = activeTasks()
  saveTaskRefs()
}

function clearAll() {
  tasks.value = []
  saveTaskRefs()
  stopPolling()
}

function resultKind(task: MediaTask): 'img' | 'video' | 'audio' {
  if (task.type === 'image') return 'img'
  if (task.type === 'video') return 'video'
  return 'audio'
}

function statusLabel(task: MediaTask): string {
  const key = ['queued', 'running', 'succeeded', 'failed'].includes(task.status) ? task.status : 'unknown'
  return t(`workbench.status.${key}`)
}

function statusTagType(task: MediaTask): 'success' | 'warning' | 'danger' | 'info' {
  if (task.status === 'succeeded') return 'success'
  if (task.status === 'failed') return 'danger'
  if (task.status === 'running') return 'warning'
  return 'info'
}

onMounted(() => {
  loadTaskRefs()
  void loadModels()
  void refresh().then(startPolling)
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="page">
    <h2>{{ t('workbench.title') }}</h2>

    <div class="card config">
      <div class="row">
        <label>
          {{ t('workbench.apiKey') }}
          <el-input v-model="apiKey" :placeholder="t('workbench.apiKeyPlaceholder')" @change="onKeyChange" />
        </label>
      </div>
    </div>

    <div class="card">
      <el-tabs v-model="tab">
        <el-tab-pane v-for="k in (['image', 'video', 'music', 'tts'] as Tab[])" :key="k" :label="t(`workbench.type.${k}`)" :name="k" />
      </el-tabs>

      <div class="row">
        <label>
          {{ t('workbench.model') }}
          <el-select v-model="selectedModel" style="min-width: 280px">
            <el-option
              v-if="!modelsForTab.length"
              :label="t('workbench.noModel')"
              value=""
              disabled
            />
            <el-option
              v-for="m in modelsForTab"
              :key="m.id"
              :label="`${m.id} (${m.provider})`"
              :value="m.id"
            />
          </el-select>
        </label>

        <template v-if="tab === 'image'">
          <label class="narrow">
            {{ t('workbench.n') }}
            <el-input-number v-model="n" :min="1" :max="4" />
          </label>
          <label class="narrow">
            {{ t('workbench.size') }}
            <el-input v-model="size" placeholder="1024x1024" />
          </label>
        </template>
        <template v-else-if="tab === 'video'">
          <label class="narrow">
            {{ t('workbench.duration') }}
            <el-input v-model="duration" placeholder="5" />
          </label>
        </template>
      </div>

      <label class="field">
        {{ tab === 'tts' ? t('workbench.text') : t('workbench.prompt') }}
        <el-input
          v-if="tab !== 'tts'"
          v-model="prompt"
          type="textarea"
          :rows="3"
          :placeholder="t('workbench.promptPlaceholder')"
          :disabled="busy"
        />
        <el-input
          v-else
          v-model="text"
          type="textarea"
          :rows="3"
          :placeholder="t('workbench.textPlaceholder')"
          :disabled="busy"
        />
      </label>

      <p v-if="tab === 'music'" class="muted hint">{{ t('workbench.musicHint') }}</p>
      <p v-if="priceHint" class="muted hint">{{ priceHint }}</p>

      <div class="actions">
        <div v-if="error" class="error">{{ error }}</div>
        <el-button type="primary" :loading="busy" @click="submit">
          {{ t('workbench.submit') }}
        </el-button>
      </div>
    </div>

    <div class="card tasks">
      <div class="tasks-head">
        <h3>{{ t('workbench.tasks') }}</h3>
        <div class="task-actions">
          <el-button size="small" :disabled="finishedCount === 0" @click="clearFinished">
            {{ t('workbench.clearFinished') }}
          </el-button>
          <el-button size="small" :disabled="!tasks.length" @click="clearAll">
            {{ t('workbench.clearAll') }}
          </el-button>
        </div>
      </div>

      <p v-if="!tasks.length" class="muted empty">{{ t('workbench.tasksEmpty') }}</p>

      <div v-for="task in tasks" :key="task.task_id" class="task">
        <div class="task-head">
          <el-tag :type="statusTagType(task)" size="small">{{ statusLabel(task) }}</el-tag>
          <span class="task-model">{{ task.model }}</span>
          <span class="muted task-type">{{ t(`workbench.type.${task.type}`) }}</span>
          <span v-if="task.cost_usd > 0" class="muted">{{ t('workbench.cost') }} ${{ task.cost_usd.toFixed(4) }}</span>
        </div>
        <p v-if="task.error" class="task-error">{{ task.error }}</p>
        <div v-if="task.result_urls.length" class="results">
          <template v-if="resultKind(task) === 'img'">
            <a v-for="url in task.result_urls" :key="url" :href="url" target="_blank" rel="noreferrer">
              <img :src="url" class="thumb" :alt="task.model" />
            </a>
          </template>
          <template v-else-if="resultKind(task) === 'video'">
            <video v-for="url in task.result_urls" :key="url" :src="url" controls class="result-media" />
          </template>
          <template v-else>
            <div v-for="url in task.result_urls" :key="url" class="audio-row">
              <audio :src="url" controls />
              <a :href="url" target="_blank" rel="noreferrer">{{ t('workbench.open') }}</a>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
.card .row {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  align-items: flex-end;
}
.config label,
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.field {
  margin-top: 16px;
}
.row label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.row label.narrow {
  min-width: 140px;
}
.hint {
  margin: 10px 0 0;
  font-size: 12px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 14px;
}
.error {
  color: #f87171;
  font-size: 13px;
}
.tasks {
  margin-top: 16px;
}
.tasks-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.tasks-head h3 {
  margin: 0;
  font-size: 15px;
}
.empty {
  text-align: center;
  padding: 24px 0;
}
.task {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 12px;
  margin-top: 10px;
}
.task-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.task-model {
  font-weight: 600;
  font-size: 13px;
  color: var(--accent);
}
.task-type {
  font-size: 12px;
}
.task-error {
  margin: 8px 0 0;
  color: #f87171;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-word;
}
.results {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 10px;
}
.thumb {
  width: 160px;
  height: 160px;
  object-fit: cover;
  border-radius: 8px;
  border: 1px solid var(--border);
}
.result-media {
  max-width: 100%;
  max-height: 320px;
  border-radius: 8px;
}
.audio-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
</style>
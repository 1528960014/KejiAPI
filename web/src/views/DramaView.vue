<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  dramaStatus,
  getApiKey,
  listPublicModels,
  setApiKey,
  submitDrama,
  type Drama,
  type PublicModel,
} from '../api/client'

const { t } = useI18n()

const DRAMAS_STORAGE = 'modelhub-dramas'
const POLL_MS = 3000

const apiKey = ref(getApiKey())
const models = ref<PublicModel[]>([])
const script = ref('')
const style = ref('')
const shotCount = ref(8)
const storyboardModel = ref('')
const imageModel = ref('')
const ttsModel = ref('')
const busy = ref(false)
const error = ref('')
const dramas = ref<Drama[]>([])
let pollTimer: number | null = null

const chatModels = computed(() => models.value.filter((m) => (m.capabilities || []).includes('chat')))
const imageModels = computed(() => models.value.filter((m) => (m.capabilities || []).includes('image')))
const ttsModels = computed(() => models.value.filter((m) => (m.capabilities || []).includes('tts')))

const priceHint = computed(() => {
  const m = imageModels.value.find((x) => x.id === imageModel.value)
  if (!m || m.unit_price === undefined || m.unit_price === 0) return ''
  const perShot = m.unit_price + (ttsModel.value ? (ttsModels.value.find((x) => x.id === ttsModel.value)?.unit_price ?? 0) : 0)
  return `${t('drama.priceHint')}: ≈ $${(perShot * shotCount.value).toFixed(4)}`
})

async function loadModels() {
  try {
    models.value = await listPublicModels()
    if (!imageModels.value.some((m) => m.id === imageModel.value)) {
      imageModel.value = imageModels.value[0]?.id ?? ''
    }
    if (!chatModels.value.some((m) => m.id === storyboardModel.value)) {
      storyboardModel.value = chatModels.value[0]?.id ?? ''
    }
  } catch {
    models.value = []
  }
}

function onKeyChange() {
  setApiKey(apiKey.value.trim())
}

function isActive(d: Drama): boolean {
  return d.status !== 'succeeded' && d.status !== 'failed'
}

function saveRefs() {
  localStorage.setItem(
    DRAMAS_STORAGE,
    JSON.stringify(dramas.value.map((d) => d.drama_id)),
  )
}

function loadRefs() {
  try {
    const raw = localStorage.getItem(DRAMAS_STORAGE)
    if (!raw) return
    const ids = JSON.parse(raw) as string[]
    dramas.value = ids.map((id) => ({
      drama_id: id,
      status: 'unknown',
      title: '',
      style: '',
      storyboard_model: '',
      image_model: '',
      tts_model: null,
      shots_planned: 0,
      shots: [],
      cost_usd: 0,
      error: '',
      video_url: '',
      video_error: '',
      created_at: '',
      updated_at: '',
    }))
  } catch {
    localStorage.removeItem(DRAMAS_STORAGE)
  }
}

async function submit() {
  error.value = ''
  if (!apiKey.value) {
    error.value = t('drama.noKey')
    return
  }
  if (!script.value.trim()) {
    error.value = t('drama.noScript')
    return
  }
  if (!imageModel.value) {
    error.value = t('drama.noModel')
    return
  }
  const body: Record<string, unknown> = {
    script: script.value.trim(),
    style: style.value.trim(),
    shots: shotCount.value,
    image_model: imageModel.value,
  }
  if (storyboardModel.value) body.storyboard_model = storyboardModel.value
  if (ttsModel.value) body.tts_model = ttsModel.value
  busy.value = true
  try {
    const res = await submitDrama(body as never)
    dramas.value.unshift({
      drama_id: res.drama_id,
      status: res.status,
      title: script.value.trim().split('\n')[0].slice(0, 60),
      style: style.value.trim(),
      storyboard_model: storyboardModel.value,
      image_model: imageModel.value,
      tts_model: ttsModel.value || null,
      shots_planned: shotCount.value,
      shots: [],
      cost_usd: 0,
      error: '',
      video_url: '',
      video_error: '',
      created_at: '',
      updated_at: '',
    })
    saveRefs()
    script.value = ''
    startPolling()
  } catch (e) {
    error.value = String((e as Error)?.message ?? e)
  } finally {
    busy.value = false
  }
}

async function refresh() {
  const actives = dramas.value.filter(isActive)
  if (!actives.length) {
    stopPolling()
    return
  }
  await Promise.all(
    actives.map(async (d) => {
      try {
        Object.assign(d, await dramaStatus(d.drama_id))
      } catch {
        // keep last known state; retry on the next tick
      }
    }),
  )
  if (!dramas.value.some(isActive)) stopPolling()
}

function startPolling() {
  stopPolling()
  if (dramas.value.some(isActive)) {
    pollTimer = window.setInterval(() => void refresh(), POLL_MS)
  }
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

const finishedCount = computed(() => dramas.value.length - dramas.value.filter(isActive).length)

function clearFinished() {
  dramas.value = dramas.value.filter(isActive)
  saveRefs()
}

function clearAll() {
  dramas.value = []
  saveRefs()
  stopPolling()
}

function statusLabel(d: Drama): string {
  const key = ['queued', 'running', 'succeeded', 'failed'].includes(d.status) ? d.status : 'unknown'
  return t(`drama.status.${key}`)
}

function statusTagType(d: Drama): 'success' | 'warning' | 'danger' | 'info' {
  if (d.status === 'succeeded') return 'success'
  if (d.status === 'failed') return 'danger'
  if (d.status === 'running') return 'warning'
  return 'info'
}

function shotStatusTag(shotStatus: string): 'success' | 'warning' | 'danger' | 'info' {
  if (shotStatus === 'succeeded') return 'success'
  if (shotStatus === 'failed') return 'danger'
  if (shotStatus === 'running') return 'warning'
  return 'info'
}

function shotStatusLabel(shotStatus: string): string {
  const key = ['pending', 'running', 'succeeded', 'failed'].includes(shotStatus) ? shotStatus : 'unknown'
  return t(`drama.shotStatus.${key}`)
}

function downloadAssetPack(d: Drama) {
  const pack = {
    drama_id: d.drama_id,
    created_at: d.created_at,
    title: d.title,
    style: d.style,
    storyboard_model: d.storyboard_model,
    image_model: d.image_model,
    tts_model: d.tts_model || null,
    shots_planned: d.shots_planned,
    cost_usd: d.cost_usd,
    shots: d.shots,
  }
  const blob = new Blob([JSON.stringify(pack, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `modelhub-drama-${d.drama_id.slice(0, 8)}.json`
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(() => {
  loadRefs()
  void loadModels()
  void refresh().then(startPolling)
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="page">
    <h2>{{ t('drama.title') }}</h2>
    <p class="muted sub">{{ t('drama.sub') }}</p>

    <div class="card config">
      <div class="row">
        <label>
          {{ t('drama.apiKey') }}
          <el-input v-model="apiKey" :placeholder="t('drama.apiKeyPlaceholder')" @change="onKeyChange" />
        </label>
      </div>
    </div>

    <div class="card">
      <label class="field">
        {{ t('drama.script') }}
        <el-input v-model="script" type="textarea" :rows="6" :placeholder="t('drama.scriptPlaceholder')" :disabled="busy" />
      </label>

      <div class="row">
        <label>
          {{ t('drama.style') }}
          <el-input v-model="style" :placeholder="t('drama.stylePlaceholder')" />
        </label>
        <label class="narrow">
          {{ t('drama.shots') }}
          <el-input-number v-model="shotCount" :min="2" :max="24" />
        </label>
      </div>

      <div class="row">
        <label>
          {{ t('drama.storyboardModel') }}
          <el-select v-model="storyboardModel" style="min-width: 220px">
            <el-option :label="t('drama.naiveSplit')" value="" />
            <el-option v-for="m in chatModels" :key="m.id" :label="`${m.id} (${m.provider})`" :value="m.id" />
          </el-select>
        </label>
        <label>
          {{ t('drama.imageModel') }}
          <el-select v-model="imageModel" style="min-width: 220px">
            <el-option v-if="!imageModels.length" :label="t('drama.noModel')" value="" disabled />
            <el-option v-for="m in imageModels" :key="m.id" :label="`${m.id} (${m.provider})`" :value="m.id" />
          </el-select>
        </label>
        <label>
          {{ t('drama.ttsModel') }}
          <el-select v-model="ttsModel" style="min-width: 220px">
            <el-option :label="t('drama.noTts')" value="" />
            <el-option v-for="m in ttsModels" :key="m.id" :label="`${m.id} (${m.provider})`" :value="m.id" />
          </el-select>
        </label>
      </div>

      <p v-if="priceHint" class="muted hint">{{ priceHint }}</p>
      <p class="muted hint">{{ t('drama.billingHint') }}</p>

      <div class="actions">
        <div v-if="error" class="error">{{ error }}</div>
        <el-button type="primary" :loading="busy" @click="submit">
          {{ t('drama.submit') }}
        </el-button>
      </div>
    </div>

    <div class="card dramas">
      <div class="list-head">
        <h3>{{ t('drama.list') }}</h3>
        <div class="list-actions">
          <el-button size="small" :disabled="finishedCount === 0" @click="clearFinished">
            {{ t('drama.clearFinished') }}
          </el-button>
          <el-button size="small" :disabled="!dramas.length" @click="clearAll">
            {{ t('drama.clearAll') }}
          </el-button>
        </div>
      </div>

      <p v-if="!dramas.length" class="muted empty">{{ t('drama.empty') }}</p>

      <div v-for="d in dramas" :key="d.drama_id" class="drama">
        <div class="drama-head">
          <el-tag :type="statusTagType(d)" size="small">{{ statusLabel(d) }}</el-tag>
          <span class="drama-title">{{ d.title || t('drama.untitled') }}</span>
          <span class="muted drama-models">
            {{ d.image_model }}<template v-if="d.tts_model"> + {{ d.tts_model }}</template>
          </span>
          <span v-if="d.cost_usd > 0" class="muted">{{ t('drama.cost') }} ${{ d.cost_usd.toFixed(4) }}</span>
          <el-button
            v-if="d.shots.length"
            size="small"
            @click="downloadAssetPack(d)"
          >
            {{ t('drama.export') }}
          </el-button>
        </div>
        <p v-if="d.error" class="drama-error">{{ d.error }}</p>
        <div v-if="d.video_url" class="final-video-wrap">
          <video :src="d.video_url" controls class="final-video" />
          <a :href="d.video_url" target="_blank" rel="noreferrer" class="muted">{{ t('drama.openVideo') }}</a>
        </div>
        <p v-else-if="d.status === 'succeeded' && d.video_error" class="muted video-missing">
          {{ t('drama.videoMissing') }}: {{ d.video_error }}
        </p>
        <div v-if="d.shots.length" class="shots">
          <div v-for="shot in d.shots" :key="shot.shot_no" class="shot">
            <div class="shot-head">
              <span class="shot-no">#{{ shot.shot_no }}</span>
              <el-tag :type="shotStatusTag(shot.status)" size="small">{{ shotStatusLabel(shot.status) }}</el-tag>
            </div>
            <a v-if="shot.image_url" :href="shot.image_url" target="_blank" rel="noreferrer">
              <img :src="shot.image_url" class="shot-img" :alt="shot.scene" />
            </a>
            <p v-if="shot.dialogue" class="shot-dialogue">{{ shot.dialogue }}</p>
            <audio v-if="shot.audio_url" :src="shot.audio_url" controls class="shot-audio" />
            <p v-if="shot.error" class="shot-error">{{ shot.error }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 4px;
}
.sub {
  margin: 0 0 16px;
  font-size: 13px;
}
.card .row {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  align-items: flex-end;
}
.config label,
.field,
.row label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}
.field {
  margin-bottom: 14px;
}
.row {
  margin-bottom: 14px;
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
.dramas {
  margin-top: 16px;
}
.list-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.list-head h3 {
  margin: 0;
  font-size: 15px;
}
.empty {
  text-align: center;
  padding: 24px 0;
}
.drama {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 12px;
  margin-top: 10px;
}
.drama-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.drama-title {
  font-weight: 600;
  font-size: 13px;
}
.drama-models {
  font-size: 12px;
}
.drama-error {
  margin: 8px 0 0;
  color: #f87171;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-word;
}
.final-video-wrap {
  margin-top: 10px;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.final-video {
  max-width: 100%;
  max-height: 360px;
  border-radius: 8px;
  border: 1px solid var(--border);
}
.video-missing {
  margin: 8px 0 0;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.shots {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 10px;
  margin-top: 10px;
}
.shot {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px;
}
.shot-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.shot-no {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-dim);
}
.shot-img {
  width: 100%;
  aspect-ratio: 1;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid var(--border);
}
.shot-dialogue {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--text-dim);
  white-space: pre-wrap;
  word-break: break-word;
}
.shot-audio {
  width: 100%;
  margin-top: 6px;
  height: 32px;
}
.shot-error {
  margin: 6px 0 0;
  color: #f87171;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
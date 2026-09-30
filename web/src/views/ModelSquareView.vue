<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { listPublicModels, type PublicModel } from '../api/client'

const { t } = useI18n()

const models = ref<PublicModel[]>([])
const loading = ref(true)
const query = ref('')
const sortBy = ref<'name' | 'input' | 'output'>('name')
const per1M = ref(true)

onMounted(async () => {
  try {
    models.value = await listPublicModels()
  } catch {
    models.value = []
  } finally {
    loading.value = false
  }
})

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  let list = models.value
  if (q) {
    list = list.filter(
      (m) => m.id.toLowerCase().includes(q) || m.provider.toLowerCase().includes(q),
    )
  }
  const sorted = [...list]
  if (sortBy.value === 'name') {
    sorted.sort((a, b) => a.id.localeCompare(b.id))
  } else if (sortBy.value === 'input') {
    sorted.sort((a, b) => a.input_price_per_1k - b.input_price_per_1k)
  } else {
    sorted.sort((a, b) => a.output_price_per_1k - b.output_price_per_1k)
  }
  return sorted
})

function fmtPrice(per1k: number): string {
  if (!per1k) return t('models.free')
  const v = per1M.value ? per1k * 1000 : per1k
  return '$' + (v >= 0.01 ? v.toFixed(2) : v.toFixed(4))
}

function capLabel(cap: string): string {
  switch (cap) {
    case 'chat':
      return t('models.capText')
    case 'vision':
      return t('models.capVision')
    case 'image':
      return t('models.capImage')
    case 'video':
      return t('models.capVideo')
    case 'music':
    case 'tts':
      return t('models.capAudio')
    default:
      return cap
  }
}

function providerInitial(provider: string): string {
  return (provider || '?').slice(0, 1).toUpperCase()
}

async function copyId(id: string) {
  try {
    await navigator.clipboard.writeText(id)
    ElMessage.success(t('models.copied'))
  } catch {
    ElMessage.info(id)
  }
}
</script>

<template>
  <div class="page plaza">
    <header class="plaza-head">
      <div>
        <h1>{{ t('models.title') }}</h1>
        <p class="muted">{{ t('models.subtitle') }}</p>
      </div>
      <div class="plaza-toolbar">
        <input v-model="query" class="search" type="search" :placeholder="t('models.search')" />
        <select v-model="sortBy" class="select">
          <option value="name">{{ t('models.sortName') }}</option>
          <option value="input">{{ t('models.sortInput') }}</option>
          <option value="output">{{ t('models.sortOutput') }}</option>
        </select>
        <button
          class="unit-toggle"
          type="button"
          @click="per1M = !per1M"
        >
          {{ per1M ? t('models.unit1m') : t('models.unit1k') }}
        </button>
      </div>
    </header>

    <div class="plaza-count">
      {{ t('models.count', { n: filtered.length }) }}
    </div>

    <div v-if="loading" class="muted">…</div>
    <el-empty v-else-if="!filtered.length" :description="t('models.empty')" />
    <div v-else class="grid">
      <div v-for="m in filtered" :key="m.id" class="model-card">
        <div class="mc-head">
          <span class="mc-ico">{{ providerInitial(m.provider) }}</span>
          <div class="mc-namebox">
            <div class="mc-name">{{ m.id }}</div>
            <div class="mc-provider">{{ m.provider }}</div>
          </div>
          <button
            class="copy-btn"
            type="button"
            :title="t('models.copy')"
            @click="copyId(m.id)"
          >⧉</button>
        </div>
        <div class="mc-prices">
          <div class="mc-price">
            <div class="pl">{{ t('models.priceInput') }}</div>
            <div class="pv">{{ fmtPrice(m.input_price_per_1k) }}</div>
          </div>
          <div class="mc-price">
            <div class="pl">{{ t('models.priceOutput') }}</div>
            <div class="pv">{{ fmtPrice(m.output_price_per_1k) }}</div>
          </div>
          <div class="mc-price">
            <div class="pl">{{ t('models.priceUnit') }}</div>
            <div class="pv">{{ per1M ? '1M' : '1K' }}</div>
          </div>
        </div>
        <div class="mc-foot">
          <span v-for="c in m.capabilities" :key="c" class="mc-tag">{{ capLabel(c) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.plaza {
  max-width: 1080px;
  margin: 0 auto;
  padding: 40px 24px 64px;
}
.plaza-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
  margin-bottom: 18px;
}
.plaza-head h1 {
  margin: 0 0 6px;
  font-size: 28px;
}
.plaza-toolbar {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  color: var(--text);
  border-radius: 10px;
  padding: 8px 14px;
  font-size: 13px;
  min-width: 240px;
}
.select,
.unit-toggle {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  color: var(--text);
  border-radius: 10px;
  padding: 8px 12px;
  font-size: 13px;
  cursor: pointer;
}
.unit-toggle {
  color: var(--accent);
  font-weight: 600;
}
.plaza-count {
  font-size: 12.5px;
  color: var(--text-dim);
  margin-bottom: 14px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
}
.mc-namebox {
  flex: 1;
  min-width: 0;
}
.mc-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.copy-btn {
  border: 1px solid var(--border);
  background: var(--bg-hover);
  color: var(--text-dim);
  border-radius: 8px;
  width: 30px;
  height: 30px;
  cursor: pointer;
  font-size: 14px;
}
.copy-btn:hover {
  color: var(--accent);
  border-color: var(--accent);
}
</style>
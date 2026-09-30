<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as echarts from 'echarts'
import {
  publicRankings,
  type PublicRankings,
  type RankingPeriod,
} from '../api/client'

const { t } = useI18n()

const period = ref<RankingPeriod>('today')
const data = ref<PublicRankings | null>(null)
const loading = ref(false)

const chartRef = ref<HTMLElement | null>(null)
let chartInstance: echarts.ECharts | null = null

const PERIODS: { key: RankingPeriod; label: string }[] = [
  { key: 'today', label: 'rankings.tabToday' },
  { key: 'week', label: 'rankings.tabWeek' },
  { key: 'month', label: 'rankings.tabMonth' },
  { key: 'year', label: 'rankings.tabYear' },
]

// bucket labels must match the server windows (UTC aligned)
function bucketLabels(p: RankingPeriod): string[] {
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  if (p === 'today') {
    return Array.from({ length: 24 }, (_, i) => pad(i))
  }
  if (p === 'year') {
    const out: string[] = []
    for (let i = 11; i >= 0; i--) {
      const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - i, 1))
      out.push(`${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}`)
    }
    return out
  }
  const days = p === 'week' ? 7 : 30
  const out: string[] = []
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - i))
    out.push(`${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`)
  }
  return out
}

function fmtTokens(n: number): string {
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B'
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'
  return String(n)
}

function changeText(pct: number): { cls: string; text: string } {
  if (pct > 1) return { cls: 'up', text: `↑ ${pct.toFixed(0)}%` }
  if (pct < -1) return { cls: 'down', text: `↓ ${Math.abs(pct).toFixed(0)}%` }
  return { cls: 'flat', text: t('rankings.flat') }
}

async function load() {
  loading.value = true
  try {
    data.value = await publicRankings(period.value)
    await new Promise((r) => setTimeout(r, 30))
    renderChart()
  } catch {
    data.value = null
  } finally {
    loading.value = false
  }
}

const PALETTE = ['#8b7cf6', '#f472b6', '#a78bfa', '#60a5fa', '#34d399', '#fbbf24']

function renderChart() {
  if (!chartRef.value || !data.value) return
  const d = data.value
  const labels = bucketLabels(d.period)
  const top = d.models.slice(0, 5)
  const idx = new Map(top.map((m, i) => [m.model_id, i]))
  const series = top.map((m) => ({
    name: m.model_id,
    type: 'bar' as const,
    stack: 'total',
    barMaxWidth: 26,
    itemStyle: { color: PALETTE[top.indexOf(m) % PALETTE.length] },
    emphasis: { focus: 'series' as const },
    data: labels.map(() => 0),
  }))
  const pos = new Map(labels.map((l, i) => [l, i]))
  for (const b of d.buckets) {
    const mi = idx.get(b.model_id)
    const li = pos.get(b.bucket)
    if (mi === undefined || li === undefined) continue
    series[mi].data[li] = b.tokens
  }
  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value)
  }
  chartInstance.setOption(
    {
      grid: { left: 48, right: 8, top: 30, bottom: 28 },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        valueFormatter: (v: unknown) => fmtTokens(Number(v) || 0),
      },
      legend: {
        type: 'scroll',
        top: 0,
        textStyle: { color: 'var(--text-dim)', fontSize: 11 },
      },
      xAxis: {
        type: 'category',
        data: labels,
        axisLabel: { color: 'var(--text-dim)', fontSize: 10, interval: 'auto' },
        axisLine: { lineStyle: { color: 'var(--border)' } },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: 'var(--text-dim)', fontSize: 10, formatter: (v: number) => fmtTokens(v) },
        splitLine: { lineStyle: { color: 'var(--border)' } },
      },
      series,
    },
    true,
  )
}

function onResize() {
  chartInstance?.resize()
}

watch(period, () => void load())

onMounted(() => {
  window.addEventListener('resize', onResize)
  void load()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  if (chartInstance) {
    chartInstance.dispose()
    chartInstance = null
  }
})

const board = computed(() => data.value?.models ?? [])
</script>

<template>
  <div class="page ranks">
    <header class="ranks-head">
      <div>
        <h1>{{ t('rankings.title') }}</h1>
        <p class="muted">{{ t('rankings.subtitle') }}</p>
      </div>
      <div class="total-box">
        <div class="total-num">{{ data ? fmtTokens(data.total_tokens) : '—' }}</div>
        <div class="total-label">{{ t('rankings.totalTokens') }}</div>
      </div>
    </header>

    <div class="rank-tabs">
      <button
        v-for="p in PERIODS"
        :key="p.key"
        class="rank-tab"
        :class="{ active: period === p.key }"
        type="button"
        @click="period = p.key"
      >
        {{ t(p.label) }}
      </button>
    </div>

    <div class="ranks-body">
      <div class="chart-card">
        <h3>{{ t('rankings.hotTitle') }}</h3>
        <div v-show="board.length" ref="chartRef" class="chart"></div>
        <div v-if="!board.length" class="empty">{{ t('rankings.chartEmpty') }}</div>
      </div>

      <div class="board-card">
        <h3>{{ t('rankings.boardTitle') }}</h3>
        <div v-if="!board.length && !loading" class="empty">{{ t('rankings.empty') }}</div>
        <div v-else class="rank-list">
          <div v-for="(m, i) in board" :key="m.model_id" class="rank-row">
            <span class="no">{{ i + 1 }}</span>
            <span class="ico">{{ (m.provider || '?').slice(0, 1).toUpperCase() }}</span>
            <div class="nm">
              <div class="t" :title="m.model_id">{{ m.model_id }}</div>
              <div class="p">{{ m.provider }} · {{ fmtTokens(m.requests) }} {{ t('rankings.requests') }}</div>
            </div>
            <div class="tk">{{ fmtTokens(m.tokens) }}</div>
            <div class="chg" :class="changeText(m.change_pct).cls">
              {{ changeText(m.change_pct).text }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ranks {
  max-width: 1080px;
  margin: 0 auto;
  padding: 40px 24px 64px;
}
.ranks-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
}
.ranks-head h1 {
  margin: 0 0 6px;
  font-size: 28px;
}
.total-box {
  text-align: right;
}
.total-num {
  font-size: 40px;
  font-weight: 800;
  background: var(--accent-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  line-height: 1;
}
.total-label {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--text-dim);
}
.ranks-body {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 20px;
  align-items: start;
}
.chart-card,
.board-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 20px;
}
.chart-card h3,
.board-card h3 {
  margin: 0 0 14px;
  font-size: 15px;
}
.chart {
  width: 100%;
  height: 320px;
}
.empty {
  height: 320px;
  display: grid;
  place-items: center;
  color: var(--text-dim);
  font-size: 13px;
}
@media (max-width: 960px) {
  .ranks-body {
    grid-template-columns: 1fr;
  }
}
</style>
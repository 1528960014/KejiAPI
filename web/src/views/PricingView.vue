<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listPublicModels, type PublicModel } from '../api/client'

const { t } = useI18n()
const models = ref<PublicModel[]>([])
const loaded = ref(false)

function pricePer1k(value?: number): string {
  return value === undefined || value === null ? t('pricing.dash') : `$${value.toFixed(6)}`
}

onMounted(async () => {
  try {
    models.value = await listPublicModels()
  } catch {
    models.value = []
  } finally {
    loaded.value = true
  }
})
</script>

<template>
  <div class="page">
    <h2>{{ t('pricing.title') }}</h2>
    <div class="card">
      <el-table v-if="models.length" :data="models">
        <el-table-column prop="id" :label="t('pricing.model')" />
        <el-table-column prop="provider" :label="t('pricing.provider')" width="140" />
        <el-table-column :label="t('pricing.capabilities')" width="200">
          <template #default="{ row }">
            <el-tag
              v-for="cap in row.capabilities"
              :key="cap"
              size="small"
              style="margin-right: 4px"
            >
              {{ cap }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('pricing.input')" width="150">
          <template #default="{ row }">{{ pricePer1k(row.input_price_per_1k) }}</template>
        </el-table-column>
        <el-table-column :label="t('pricing.output')" width="150">
          <template #default="{ row }">{{ pricePer1k(row.output_price_per_1k) }}</template>
        </el-table-column>
      </el-table>
      <p v-else-if="loaded" class="muted">{{ t('pricing.empty') }}</p>
      <p class="muted note">{{ t('pricing.note') }}</p>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
.note {
  margin: 16px 0 0;
  font-size: 13px;
}
</style>
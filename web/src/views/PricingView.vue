<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getApiKey, listModels, type ModelInfo } from '../api/client'

const { t } = useI18n()
const models = ref<ModelInfo[]>([])
const loaded = ref(false)

onMounted(async () => {
  if (!getApiKey()) {
    loaded.value = true
    return
  }
  try {
    models.value = await listModels()
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
        <el-table-column prop="owned_by" :label="t('pricing.provider')" />
        <el-table-column :label="t('pricing.input')">
          <template #default>{{ t('pricing.dash') }}</template>
        </el-table-column>
        <el-table-column :label="t('pricing.output')">
          <template #default>{{ t('pricing.dash') }}</template>
        </el-table-column>
      </el-table>
      <p v-else-if="loaded" class="muted">{{ t('chat.noModels') }}</p>
      <p class="muted note">{{ t('pricing.coming') }}</p>
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
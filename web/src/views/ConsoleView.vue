<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { getApiKey } from '../api/client'

const { t } = useI18n()
const apiKey = getApiKey()

const example = `curl https://<your-host>/v1/chat/completions \\
  -H "Authorization: Bearer sk-xxxx" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "hi"}],
    "stream": true
  }'`
</script>

<template>
  <div class="page">
    <h2>{{ t('console.title') }}</h2>
    <div class="card">
      <h3>{{ t('console.quickstart') }}</h3>
      <p class="muted">{{ t('console.docs') }}</p>
      <pre class="code">{{ example }}</pre>
      <p v-if="!apiKey" class="muted">{{ t('chat.noKey') }}</p>
      <p class="muted">{{ t('console.usageComing') }}</p>
    </div>
  </div>
</template>

<style scoped>
h2 {
  margin: 0 0 16px;
}
h3 {
  margin: 0 0 8px;
}
.code {
  background: #0b0b0e;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 14px;
  overflow-x: auto;
  font-size: 13px;
  line-height: 1.6;
}
</style>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import i18n from '../i18n'

const { t } = useI18n()
type Feature = { title: string; desc: string }
const features: Feature[] = (i18n.global.messages.value['zh-CN'] as { home: { features: Feature[] } }).home.features
</script>

<template>
  <div class="page">
    <section class="hero">
      <h1>{{ t('home.heroTitle') }}</h1>
      <p class="muted">{{ t('home.heroSub') }}</p>
      <router-link to="/chat" class="cta">{{ t('home.start') }}</router-link>
    </section>
    <section class="features">
      <div v-for="f in features" :key="f.title" class="card feature">
        <h3>{{ f.title }}</h3>
        <p class="muted">{{ f.desc }}</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.hero {
  text-align: center;
  padding: 72px 0 48px;
}
.hero h1 {
  font-size: 40px;
  margin: 0 0 16px;
}
.cta {
  display: inline-block;
  margin-top: 24px;
  padding: 10px 28px;
  border-radius: 10px;
  background: var(--accent);
  color: #08252b;
  font-weight: 600;
}
.features {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 16px;
}
.feature h3 {
  margin: 0 0 8px;
}
.feature p {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
}
</style>
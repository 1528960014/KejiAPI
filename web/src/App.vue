<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

const { t } = useI18n()
const route = useRoute()

const nav = [
  { to: '/', label: 'nav.home' },
  { to: '/chat', label: 'nav.chat' },
  { to: '/pricing', label: 'nav.pricing' },
  { to: '/console', label: 'nav.console' },
]
</script>

<template>
  <header class="topbar">
    <div class="topbar-inner">
      <router-link to="/" class="brand">ModelHub</router-link>
      <nav>
        <router-link
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="nav-link"
          :class="{ active: route.path === item.to }"
        >
          {{ t(item.label) }}
        </router-link>
      </nav>
    </div>
  </header>
  <main class="main">
    <router-view />
  </main>
</template>

<style scoped>
.topbar {
  border-bottom: 1px solid var(--border);
  background: var(--bg-panel);
}
.topbar-inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: 12px 24px;
  display: flex;
  align-items: center;
  gap: 24px;
}
.brand {
  font-weight: 700;
  font-size: 18px;
  color: var(--text);
}
nav {
  display: flex;
  gap: 8px;
}
.nav-link {
  color: var(--text-dim);
  padding: 4px 10px;
  border-radius: 6px;
}
.nav-link.active,
.nav-link:hover {
  color: var(--text);
  background: var(--bg-hover);
}
.main {
  min-height: calc(100vh - 53px);
}
</style>
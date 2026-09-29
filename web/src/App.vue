<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { LOCALES, setLocale, type Locale } from './i18n'
import { getTheme, toggleTheme, type Theme } from './theme'
import { formatUsd } from './api/client'
import { isLoggedIn, loadMe, signOut, useSession } from './session'

const { t, locale } = useI18n()
const route = useRoute()
const session = useSession()

const nav = [
  { to: '/chat', label: 'nav.chat' },
  { to: '/workbench', label: 'nav.workbench' },
  { to: '/drama', label: 'nav.drama' },
  { to: '/pricing', label: 'nav.pricing' },
  { to: '/recharge', label: 'nav.recharge' },
  { to: '/console', label: 'nav.console' },
  { to: '/orgs', label: 'nav.orgs' },
  { to: '/admin', label: 'nav.admin' },
]

// the chat page (and login) render their own full-bleed layout
const hideChrome = computed(() => route.path === '/chat' || route.path === '/login')

const theme = ref<Theme>(getTheme())

onMounted(() => {
  void loadMe(true)
})

watch(
  () => route.fullPath,
  () => {
    if (isLoggedIn()) void loadMe(true)
  },
)

function onLocaleChange(e: Event) {
  const next = (e.target as HTMLSelectElement).value as Locale
  setLocale(next)
  locale.value = next
}

function switchTheme() {
  theme.value = toggleTheme()
}

async function handleSignOut() {
  await signOut()
  ElMessage.info(t('login.loggedOut'))
}
</script>

<template>
  <header v-if="!hideChrome" class="topbar">
    <div class="topbar-inner">
      <router-link to="/chat" class="brand">
        <span class="brand-mark">▲</span>
        <span class="brand-name">ModelHub</span>
      </router-link>
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
      <div class="topbar-actions">
        <template v-if="session.user">
          <span class="user" :title="session.user.email">
            <span class="user-email">{{ session.user.email }}</span>
            <span class="user-balance">
              {{ formatUsd(session.user.balance_micro, session.user.balance_usd) }}
            </span>
          </span>
          <button class="icon-btn" type="button" @click="handleSignOut">
            {{ t('account.logout') }}
          </button>
        </template>
        <router-link v-else to="/login" class="icon-btn login-link">
          {{ t('login.title') }}
        </router-link>
        <select class="icon-btn locale-select" :value="locale" @change="onLocaleChange">
          <option v-for="l in LOCALES" :key="l.code" :value="l.code">{{ l.label }}</option>
        </select>
        <button class="icon-btn" type="button" :title="t('theme.toggle')" @click="switchTheme">
          {{ theme === 'dark' ? '☀' : '🌙' }}
        </button>
      </div>
    </div>
  </header>
  <main class="main" :class="{ bare: hideChrome }">
    <router-view />
  </main>
</template>

<style scoped>
.topbar {
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--bg-panel) 82%, transparent);
  backdrop-filter: blur(12px);
  position: sticky;
  top: 0;
  z-index: 100;
}
.topbar-inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: 10px 24px;
  display: flex;
  align-items: center;
  gap: 20px;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  text-decoration: none;
}
.brand-mark {
  display: inline-grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  color: #0b0b12;
  font-size: 12px;
  font-weight: 800;
}
.brand-name {
  font-weight: 800;
  font-size: 17px;
  letter-spacing: 0.2px;
  background: linear-gradient(90deg, var(--text), var(--accent));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
nav {
  display: flex;
  gap: 4px;
  flex: 1;
  flex-wrap: wrap;
}
.nav-link {
  color: var(--text-dim);
  padding: 5px 12px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  transition: all 0.15s ease;
}
.nav-link.active,
.nav-link:hover {
  color: var(--text);
  background: var(--bg-hover);
}
.nav-link.active {
  color: var(--accent);
}
.topbar-actions {
  display: flex;
  gap: 8px;
}
.icon-btn {
  border: 1px solid var(--border);
  background: var(--bg-panel);
  color: var(--text);
  border-radius: 8px;
  padding: 4px 10px;
  font-size: 13px;
  cursor: pointer;
  line-height: 1.4;
}
.icon-btn:hover {
  background: var(--bg-hover);
}
.locale-select {
  padding: 2px 4px;
}
.login-link {
  text-decoration: none;
}
.user {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.25;
  font-size: 12px;
  max-width: 180px;
}
.user-email {
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.user-balance {
  color: var(--text-dim);
}
.main {
  min-height: calc(100vh - 53px);
}
.main.bare {
  min-height: 100vh;
}
</style>
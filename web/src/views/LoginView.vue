<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { login, register } from '../api/client'
import { loadMe } from '../session'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const mode = ref<'login' | 'register'>('login')
const email = ref('')
const password = ref('')
const busy = ref(false)

const codeMessage: Record<string, string> = {
  invalid_credentials: 'login.errInvalidCredentials',
  email_exists: 'login.errEmailExists',
  weak_password: 'login.errWeakPassword',
  invalid_email: 'login.errInvalidEmail',
}

function errorMessage(err: unknown): string {
  const code = (
    err as { response?: { data?: { error?: { code?: string } } } }
  )?.response?.data?.error?.code
  const key = code ? codeMessage[code] : undefined
  return key ? t(key) : t('login.errNetwork')
}

async function submit() {
  if (busy.value) return
  if (!email.value.trim() || !password.value) {
    ElMessage.warning(t('login.errRequired'))
    return
  }
  busy.value = true
  try {
    if (mode.value === 'login') {
      await login(email.value.trim(), password.value)
    } else {
      await register(email.value.trim(), password.value)
    }
    await loadMe(true)
    const raw = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    const redirect = raw.startsWith('/') && !raw.startsWith('//') ? raw : '/console'
    router.push(redirect)
  } catch (err) {
    ElMessage.error(errorMessage(err))
  } finally {
    busy.value = false
  }
}

function switchMode() {
  mode.value = mode.value === 'login' ? 'register' : 'login'
}
</script>

<template>
  <div class="auth">
    <div class="auth-bg" aria-hidden="true">
      <div class="blob blob-a"></div>
      <div class="blob blob-b"></div>
      <div class="grid-overlay"></div>
    </div>

    <div class="auth-shell">
      <div class="brand-panel">
        <router-link to="/" class="brand">
          <span class="brand-mark">▲</span>
          <span class="brand-name">ModelHub</span>
        </router-link>
        <h1 class="hero">{{ t('login.heroTitle') }}</h1>
        <p class="hero-sub">{{ t('login.heroSub') }}</p>
        <ul class="features">
          <li v-for="(f, i) in [
            { icon: '⚡', title: t('login.f1Title'), desc: t('login.f1Desc') },
            { icon: '🔒', title: t('login.f2Title'), desc: t('login.f2Desc') },
            { icon: '💬', title: t('login.f3Title'), desc: t('login.f3Desc') },
          ]" :key="i">
            <span class="f-icon">{{ f.icon }}</span>
            <div>
              <div class="f-title">{{ f.title }}</div>
              <div class="f-desc">{{ f.desc }}</div>
            </div>
          </li>
        </ul>
      </div>

      <div class="panel">
        <div class="card form-card">
          <div class="seg">
            <button
              type="button"
              class="seg-btn"
              :class="{ active: mode === 'login' }"
              @click="mode = 'login'"
            >
              {{ t('login.title') }}
            </button>
            <button
              type="button"
              class="seg-btn"
              :class="{ active: mode === 'register' }"
              @click="mode = 'register'"
            >
              {{ t('login.registerTitle') }}
            </button>
          </div>

          <el-form class="form" @submit.prevent="submit">
            <label class="field">
              <span class="field-label">{{ t('login.email') }}</span>
              <el-input
                v-model="email"
                type="email"
                size="large"
                placeholder="user@modelhub.dev"
              />
            </label>
            <label class="field">
              <span class="field-label">{{ t('login.password') }}</span>
              <el-input
                v-model="password"
                type="password"
                show-password
                size="large"
                :placeholder="t('login.passwordHint')"
                @keyup.enter="submit"
              />
            </label>
            <button class="cta" type="submit" :disabled="busy">
              <span v-if="busy" class="spinner"></span>
              {{ mode === 'login' ? t('login.submit') : t('login.registerSubmit') }}
            </button>
          </el-form>

          <p class="switch">
            <a href="#" @click.prevent="switchMode">
              {{ mode === 'login' ? t('login.switchToRegister') : t('login.switchToLogin') }}
            </a>
          </p>
          <p class="back">
            <router-link to="/">{{ t('login.backHome') }}</router-link>
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth {
  position: relative;
  min-height: calc(100vh - 53px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  overflow: hidden;
}
.auth-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.35;
}
.blob-a {
  width: 480px;
  height: 480px;
  background: #22d3ee;
  top: -120px;
  left: -120px;
  animation: float 14s ease-in-out infinite alternate;
}
.blob-b {
  width: 520px;
  height: 520px;
  background: #818cf8;
  bottom: -160px;
  right: -120px;
  animation: float 18s ease-in-out infinite alternate-reverse;
}
@keyframes float {
  from {
    transform: translate3d(0, 0, 0) scale(1);
  }
  to {
    transform: translate3d(60px, 40px, 0) scale(1.15);
  }
}
.grid-overlay {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--border) 1px, transparent 1px),
    linear-gradient(90deg, var(--border) 1px, transparent 1px);
  background-size: 48px 48px;
  opacity: 0.14;
  mask-image: radial-gradient(ellipse at center, black 30%, transparent 75%);
}
.auth-shell {
  position: relative;
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 48px;
  align-items: center;
  width: 100%;
  max-width: 960px;
}
@media (max-width: 820px) {
  .auth-shell {
    grid-template-columns: 1fr;
    gap: 28px;
  }
  .brand-panel {
    display: none;
  }
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
}
.brand-mark {
  display: inline-grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  color: #0b0b12;
  font-size: 16px;
  font-weight: 800;
}
.brand-name {
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0.2px;
  background: linear-gradient(90deg, var(--text), var(--accent));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.hero {
  margin: 32px 0 12px;
  font-size: 30px;
  line-height: 1.25;
  font-weight: 800;
}
.hero-sub {
  margin: 0 0 28px;
  color: var(--text-dim);
  font-size: 14px;
  line-height: 1.7;
  max-width: 420px;
}
.features {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.features li {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  background: color-mix(in srgb, var(--bg-panel) 72%, transparent);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 12px 14px;
  backdrop-filter: blur(8px);
}
.f-icon {
  font-size: 18px;
  line-height: 1.4;
}
.f-title {
  font-size: 14px;
  font-weight: 600;
}
.f-desc {
  font-size: 12px;
  color: var(--text-dim);
  margin-top: 2px;
  line-height: 1.5;
}
.form-card {
  padding: 32px;
  border-radius: 20px;
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.35);
}
.seg {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px;
  background: var(--bg-hover);
  border-radius: 12px;
  padding: 4px;
  margin-bottom: 24px;
}
.seg-btn {
  border: none;
  background: transparent;
  color: var(--text-dim);
  font-size: 14px;
  font-weight: 600;
  padding: 9px 0;
  border-radius: 9px;
  cursor: pointer;
  transition: all 0.18s ease;
}
.seg-btn.active {
  background: var(--bg-panel);
  color: var(--text);
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.22);
}
.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.field-label {
  font-size: 13px;
  color: var(--text-dim);
  font-weight: 500;
}
.cta {
  margin-top: 6px;
  width: 100%;
  border: none;
  border-radius: 12px;
  padding: 12px 0;
  font-size: 15px;
  font-weight: 700;
  color: #0b0b12;
  background: linear-gradient(135deg, #22d3ee, #818cf8);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.cta:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 10px 28px rgba(34, 211, 238, 0.35);
}
.cta:disabled {
  opacity: 0.65;
  cursor: not-allowed;
}
.spinner {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid rgba(11, 11, 18, 0.35);
  border-top-color: #0b0b12;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
.switch,
.back {
  margin: 16px 0 0;
  text-align: center;
  font-size: 13px;
  color: var(--text-dim);
}
.switch a,
.back a {
  color: var(--accent);
  text-decoration: none;
  font-weight: 500;
}
</style>

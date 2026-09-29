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
</script>

<template>
  <div class="page">
    <div class="card box">
      <h2>{{ mode === 'login' ? t('login.title') : t('login.registerTitle') }}</h2>
      <el-form @submit.prevent="submit">
        <el-form-item :label="t('login.email')">
          <el-input v-model="email" type="email" placeholder="user@modelhub.dev" />
        </el-form-item>
        <el-form-item :label="t('login.password')">
          <el-input
            v-model="password"
            type="password"
            show-password
            :placeholder="t('login.passwordHint')"
          />
        </el-form-item>
        <el-button type="primary" native-type="submit" :loading="busy">
          {{ mode === 'login' ? t('login.submit') : t('login.registerSubmit') }}
        </el-button>
      </el-form>
      <p class="muted switch">
        <a
          href="#"
          @click.prevent="mode = mode === 'login' ? 'register' : 'login'"
        >
          {{ mode === 'login' ? t('login.switchToRegister') : t('login.switchToLogin') }}
        </a>
      </p>
      <p class="muted back"><router-link to="/">{{ t('login.backHome') }}</router-link></p>
    </div>
  </div>
</template>

<style scoped>
.box {
  max-width: 420px;
  margin: 48px auto;
}
h2 {
  margin: 0 0 8px;
}
.switch,
.back {
  margin: 12px 0 0;
  font-size: 13px;
}
a {
  color: var(--accent);
  text-decoration: none;
}
</style>
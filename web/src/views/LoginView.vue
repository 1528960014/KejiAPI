<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'

const { t } = useI18n()
const email = ref('')
const password = ref('')

function submit() {
  ElMessage.info(t('login.coming'))
}
</script>

<template>
  <div class="page">
    <div class="card box">
      <h2>{{ t('login.title') }}</h2>
      <p class="muted">{{ t('login.coming') }}</p>
      <el-form @submit.prevent="submit">
        <el-form-item :label="t('login.email')">
          <el-input v-model="email" />
        </el-form-item>
        <el-form-item :label="t('login.password')">
          <el-input v-model="password" type="password" show-password />
        </el-form-item>
        <el-button type="primary" native-type="submit">{{ t('login.submit') }}</el-button>
      </el-form>
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
</style>
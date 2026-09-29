import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.json'
import enUS from './locales/en-US.json'

const stored = localStorage.getItem('modelhub-locale')
const locale = stored === 'en-US' ? 'en-US' : 'zh-CN'

export function setLocale(next: 'zh-CN' | 'en-US') {
  localStorage.setItem('modelhub-locale', next)
}

export default createI18n({
  legacy: false,
  locale,
  fallbackLocale: 'en-US',
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})
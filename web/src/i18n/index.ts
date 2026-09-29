import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.json'
import enUS from './locales/en-US.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'
import ru from './locales/ru.json'
import es from './locales/es.json'

export type Locale = 'zh-CN' | 'en-US' | 'ja' | 'ko' | 'ru' | 'es'

// Locale options for the switcher; label is the native language name.
export const LOCALES: { code: Locale; label: string }[] = [
  { code: 'zh-CN', label: '中文' },
  { code: 'en-US', label: 'English' },
  { code: 'ja', label: '日本語' },
  { code: 'ko', label: '한국어' },
  { code: 'ru', label: 'Русский' },
  { code: 'es', label: 'Español' },
]

const stored = localStorage.getItem('modelhub-locale')
const locale: Locale = stored && LOCALES.some((l) => l.code === stored) ? (stored as Locale) : 'zh-CN'

export function setLocale(next: Locale) {
  localStorage.setItem('modelhub-locale', next)
}

export default createI18n({
  legacy: false,
  locale,
  fallbackLocale: 'en-US',
  messages: { 'zh-CN': zhCN, 'en-US': enUS, ja, ko, ru, es },
})

import { createI18n } from 'vue-i18n'
import ru from './locales/ru.json'
import en from './locales/en.json'

const saved = localStorage.getItem('locale') || 'ru'

const i18n = createI18n({
  legacy: false,
  locale: saved,
  fallbackLocale: 'ru',
  messages: { ru, en }
})

export default i18n

export function translateApiError(errorCode) {
  const key = `api.${errorCode}`
  const { tm, rt } = i18n.global
  const msg = tm(key)
  if (msg && typeof msg === 'string') return msg
  if (msg && msg.default) return rt(msg)
  return errorCode
}

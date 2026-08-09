// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

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

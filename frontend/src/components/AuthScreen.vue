<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

<template>
  <div class="row justify-content-center mt-5">
    <div class="col-12 col-md-6 col-lg-4" style="min-width: 320px;">
      <div class="d-flex gap-2 mb-3">
        <button @click="switchLocale" class="btn btn-outline-secondary btn-lg flex-fill d-flex align-items-center justify-content-center gap-2">
          <span style="font-size: 1.3rem;">🌐</span>
          <span class="fw-bold">{{ locale === 'ru' ? 'Русский' : 'English' }}</span>
        </button>
        <button @click="toggleTheme" class="btn btn-outline-secondary btn-lg flex-fill d-flex align-items-center justify-content-center gap-2">
          <span style="font-size: 1.3rem;">{{ isDark ? '🌙' : '☀️' }}</span>
          <span class="fw-bold">{{ isDark ? $t('auth.themeDark') : $t('auth.themeLight') }}</span>
        </button>
      </div>

      <div class="card shadow">
        <div class="card-body p-4">
          <h4 class="mb-4 text-center fw-bold">{{ store.view === 'setup' ? $t('auth.setupTitle') : $t('auth.loginTitle') }}</h4>
          
          <input v-model="username" class="form-control mb-3" maxlength="64" :placeholder="$t('auth.username')">
          <input v-model="password" type="password" class="form-control mb-4" maxlength="72" :placeholder="$t('auth.password')" @keyup.enter="submit">
          
          <button @click="submit" class="btn btn-primary w-100 fw-bold">
            {{ store.view === 'setup' ? $t('auth.createAccount') : $t('auth.login') }}
          </button>
          
          <p v-if="error" class="text-danger mt-3 mb-0 text-center">{{ error }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../store.js'
import { translateApiError } from '../i18n.js'

const { t, locale } = useI18n()

const username = ref('')
const password = ref('')
const error = ref('')
const isDark = ref(localStorage.getItem('theme') === 'dark')

if (isDark.value) document.documentElement.setAttribute('data-bs-theme', 'dark')

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.setAttribute('data-bs-theme', isDark.value ? 'dark' : 'light')
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function switchLocale() {
  const next = locale.value === 'ru' ? 'en' : 'ru'
  locale.value = next
  localStorage.setItem('locale', next)
}

async function submit() {
  if(!username.value || !password.value) { error.value = t('auth.enterCredentials'); return }
  try {
    const endpoint = store.view === 'setup' ? '/setup' : '/login'
    const res = await api.post(endpoint, { username: username.value, password: password.value })
    
    store.token = res.data.token
    localStorage.setItem('token', res.data.token)
    store.view = 'dashboard'
    actions.loadData()
  } catch (e) { 
    if (e.response?.data?.error) {
      error.value = translateApiError(e.response.data.error)
    } else {
      error.value = store.view === 'setup' ? t('auth.setupError') : t('auth.wrongCredentials')
    }
  }
}
</script>

<template>
  <div class="row justify-content-center mt-5">
    <div class="col-12 col-md-6 col-lg-4" style="min-width: 320px;">
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

const { t } = useI18n()

const username = ref('')
const password = ref('')
const error = ref('')

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

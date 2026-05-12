<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'

// Импортируем наши модули
import AuthScreen from './components/AuthScreen.vue'
import StaticTab from './components/StaticTab.vue'
import LeasesTab from './components/LeasesTab.vue'

const view = ref('loading')
const tab = ref('static')
const networkError = ref(false)
const token = ref(localStorage.getItem('token') || '')
const darkMode = ref(localStorage.getItem('theme') === 'dark')

const hosts = ref([])
const leases = ref([])
const searchQuery = ref('') 
const isDnsmasqActive = ref(false)

// Данные для передачи из Leases в Static форму
const hostFormFromLeases = ref({})

const api = axios.create({ baseURL: '/api' })
api.interceptors.request.use(config => {
  if (token.value) config.headers.Authorization = `Bearer ${token.value}`
  return config
})

function toggleTheme() {
  darkMode.value = !darkMode.value
  localStorage.setItem('theme', darkMode.value ? 'dark' : 'light')
  document.documentElement.setAttribute('data-bs-theme', darkMode.value ? 'dark' : 'light')
}

async function checkStatus() {
  document.documentElement.setAttribute('data-bs-theme', darkMode.value ? 'dark' : 'light')
  try {
    const res = await api.get('/status')
    networkError.value = false
    isDnsmasqActive.value = res.data.dnsmasq_active
    if (res.data.setup_required) view.value = 'setup'
    else if (token.value) { view.value = 'dashboard'; loadData() }
    else view.value = 'login'
  } catch (e) { networkError.value = true; view.value = 'error' }
}

function handleLoginSuccess(newToken) {
    token.value = newToken
    localStorage.setItem('token', newToken)
    view.value = 'dashboard'
    loadData()
}

async function loadData() {
  try { 
      const hRes = await api.get('/hosts'); hosts.value = hRes.data 
      const lRes = await api.get('/leases'); leases.value = lRes.data 
      checkStatus() 
  } catch (e) { 
      if(e.response?.status === 401) logout() 
      else networkError.value = true
  }
}

function copyToStatic(lease) {
    hostFormFromLeases.value = {
        mac: lease.mac, 
        ip: lease.ip, 
        hostname: lease.hostname !== '*' ? lease.hostname : '',
        file: hosts.value.length > 0 ? hosts.value[0].file : '' // Дефолтный файл
    }
    tab.value = 'static'
    window.scrollTo({ top: 0, behavior: 'smooth' })
}

async function applyConfig() {
  try { 
      await api.post('/reload')
      alert('Конфигурация проверена и успешно применена!') 
      checkStatus()
  } catch (e) { 
      if (e.response && e.response.status === 400) alert(e.response.data.error)
      else alert("Ошибка перезагрузки: " + (e.response?.data?.error || ""))
  }
}

async function downloadBackup() {
    try {
        const res = await api.get('/backup', { responseType: 'blob' })
        const url = window.URL.createObjectURL(new Blob([res.data]))
        const link = document.createElement('a')
        link.href = url
        link.setAttribute('download', `dnsmasq_backup.zip`)
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
    } catch (e) { alert("Ошибка резервной копии") }
}

function logout() {
  token.value = ''; localStorage.removeItem('token'); view.value = 'login'
}

onMounted(checkStatus)
</script>

<template>
  <div class="container mt-4">
    <!-- Header -->
    <div class="d-flex justify-content-between align-items-center mb-4">
        <h2 class="fw-bold d-flex align-items-center">
            <span class="text-primary me-2">🛡️ Intermask</span><small class="text-muted fs-6 me-3">v1.9.1</small>
            <span class="badge" :class="isDnsmasqActive ? 'bg-success' : 'bg-danger'" style="font-size: 0.6rem; vertical-align: middle;">
                {{ isDnsmasqActive ? '🟢 Dnsmasq Работает' : '🔴 Dnsmasq Остановлен' }}
            </span>
        </h2>
        <div>
             <button @click="toggleTheme" class="btn btn-outline-secondary me-2">{{ darkMode ? '☀️' : '🌙' }}</button>
             <button v-if="token" @click="logout" class="btn btn-outline-danger">Выйти</button>
        </div>
    </div>

    <!-- Error State -->
    <div v-if="view === 'error' || networkError" class="alert alert-danger text-center">
        <h4>🔌 Сервер недоступен</h4> <button @click="checkStatus" class="btn btn-sm btn-danger">Повторить</button>
    </div>

    <!-- Auth Component -->
    <AuthScreen v-if="view === 'login' || view === 'setup'" :view="view" :api="api" @login-success="handleLoginSuccess" />

    <!-- Dashboard Component -->
    <div v-if="view === 'dashboard' && !networkError">
      
      <!-- Toolbar -->
      <div class="row mb-3 align-items-center g-2">
         <div class="col-12 col-md-auto">
             <div class="btn-group w-100">
                 <button @click="tab='static'" :class="['btn', tab==='static' ? 'btn-primary' : 'btn-outline-primary']">Статика</button>
                 <button @click="tab='leases'" :class="['btn', tab==='leases' ? 'btn-primary' : 'btn-outline-primary']">Аренды</button>
             </div>
         </div>
         <div class="col-12 col-md"><input v-model="searchQuery" type="text" class="form-control" placeholder="🔍 Поиск..."></div>
         <div class="col-12 col-md-auto text-md-end d-flex gap-2 justify-content-end">
             <button @click="downloadBackup" class="btn btn-outline-info fw-bold" title="Скачать архив">💾 Бэкап</button>
             <button @click="applyConfig" class="btn btn-warning fw-bold text-dark">🔄 Применить</button>
         </div>
      </div>

      <!-- Content Tabs -->
      <StaticTab v-if="tab === 'static'" :hosts="hosts" :api="api" :searchQuery="searchQuery" :newHostForm="hostFormFromLeases" @reload="loadData" />
      <LeasesTab v-if="tab === 'leases'" :leases="leases" :hosts="hosts" :searchQuery="searchQuery" @copy-to-static="copyToStatic" />
    </div>
  </div>
</template>

<style>
.fade-in { animation: fadeIn 0.3s ease-in-out; }
@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
</style>

<script setup>
import { onMounted, onUnmounted } from 'vue'
import { store, actions } from './store.js'

import AuthScreen from './components/AuthScreen.vue'
import StaticView from './components/static/StaticView.vue'
import LeasesTab from './components/leases/LeasesTab.vue'

let arpInterval = null

function toggleTheme() {
  const isDark = document.documentElement.getAttribute('data-bs-theme') === 'dark'
  document.documentElement.setAttribute('data-bs-theme', isDark ? 'light' : 'dark')
  localStorage.setItem('theme', isDark ? 'light' : 'dark')
}

onMounted(() => {
    if (localStorage.getItem('theme') === 'dark') {
        document.documentElement.setAttribute('data-bs-theme', 'dark')
    }
    actions.checkStatus()
    arpInterval = setInterval(() => { actions.loadArp() }, 30000)
})

onUnmounted(() => { clearInterval(arpInterval) })
</script>

<template>
  <div class="container mt-4">
    
    <!-- ШАПКА -->
    <div class="d-flex justify-content-between align-items-center mb-4">
        <h2 class="fw-bold d-flex align-items-center mb-0">
            <span class="text-primary me-2">🛡️ Intermasq</span>
            <span class="badge" :class="store.isDnsmasqActive ? 'bg-success' : 'bg-danger'" style="font-size: 0.6rem; margin-top: 5px;">
                {{ store.isDnsmasqActive ? '🟢 Работает' : '🔴 Остановлен' }}
            </span>
        </h2>
        
        <!-- НОВОЕ МЕНЮ (Только если авторизован) -->
        <div v-if="store.token" class="dropdown">
            <button class="btn btn-outline-secondary dropdown-toggle" type="button" data-bs-toggle="dropdown">
                ⚙️ Меню
            </button>
            <ul class="dropdown-menu dropdown-menu-end shadow-sm">
                <li><h6 class="dropdown-header">Плагины</h6></li>
                
                <!-- Генерация пунктов меню для каждого найденного плагина -->
                <li v-for="p in store.plugins" :key="p.id">
                    <a class="dropdown-item" href="#" @click.prevent="store.tab = 'plugin-' + p.id">
                        🧩 {{ p.name }}
                    </a>
                </li>
                <li v-if="store.plugins.length === 0"><span class="dropdown-item text-muted small">Нет плагинов</span></li>

                <li><hr class="dropdown-divider"></li>
                <li><h6 class="dropdown-header">Система</h6></li>
                <li><a class="dropdown-item" href="#" @click.prevent="toggleTheme">🌓 Сменить тему</a></li>
                <li><a class="dropdown-item" href="/swagger/index.html" target="_blank">📖 API Документация</a></li>
                <li><a class="dropdown-item" href="#" @click.prevent="actions.restartSystem()">🔄 Рестарт Intermasq</a></li>
                <li><hr class="dropdown-divider"></li>
                <li><a class="dropdown-item text-danger" href="#" @click.prevent="actions.logout()">🚪 Выйти</a></li>
            </ul>
        </div>
    </div>

    <!-- ОШИБКА СЕТИ -->
    <div v-if="store.view === 'error'" class="alert alert-danger text-center">
        <h4>🔌 Сервер недоступен</h4> 
        <button @click="actions.checkStatus()" class="btn btn-sm btn-danger">Повторить подключение</button>
    </div>

    <!-- АВТОРИЗАЦИЯ -->
    <AuthScreen v-if="store.view === 'login' || store.view === 'setup'" />

    <!-- ПАНЕЛЬ УПРАВЛЕНИЯ -->
    <div v-if="store.view === 'dashboard'">
        
        <!-- 1. Главные вкладки (Статика / Аренды) -->
        <div v-if="!store.tab.startsWith('plugin-')">
            <div class="row mb-3 align-items-center g-2">
                <div class="col-12 col-md-auto">
                    <div class="btn-group w-100">
                        <button @click="store.tab='static'" :class="['btn', store.tab==='static' ? 'btn-primary' : 'btn-outline-primary']">Статика</button>
                        <button @click="store.tab='leases'" :class="['btn', store.tab==='leases' ? 'btn-primary' : 'btn-outline-primary']">Аренды</button>
                    </div>
                </div>
                <div class="col-12 col-md">
                    <input v-model="store.searchQuery" type="text" class="form-control" placeholder="🔍 Поиск по MAC, IP, Имени...">
                </div>
                <div class="col-12 col-md-auto text-md-end d-flex gap-2 justify-content-end">
                    <button @click="actions.downloadBackup()" class="btn btn-outline-info fw-bold" title="Скачать архив">💾 Бэкап</button>
                    <button @click="actions.applyConfig()" class="btn btn-warning fw-bold text-dark">🔄 Применить</button>
                </div>
            </div>

            <!-- Подключенные компоненты -->
            <StaticView v-if="store.tab === 'static'" />
            <LeasesTab v-if="store.tab === 'leases'" />
        </div>

        <!-- 2. Окно ПЛАГИНА (показывается, если выбран плагин в меню) -->
        <div v-if="store.tab.startsWith('plugin-')" class="card shadow-sm h-100 fade-in">
            <div class="card-header bg-dark text-white d-flex justify-content-between align-items-center">
                <span>🧩 Плагин</span>
                <button class="btn btn-sm btn-outline-light" @click="store.tab='static'">✕ Закрыть</button>
            </div>
            <div class="card-body p-0" style="height: 80vh;">
                <!-- Магия: загружаем интерфейс плагина через внутренний прокси -->
                <iframe :src="'/plugins/' + store.tab.replace('plugin-', '') + '/'" style="width: 100%; height: 100%; border: none;"></iframe>
            </div>
        </div>
        
    </div>
  </div>
</template>

<style>
.fade-in { animation: fadeIn 0.3s ease-in-out; }
@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
</style>

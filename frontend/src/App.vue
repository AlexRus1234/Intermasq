<script setup>
import { onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from './store.js'

import AuthScreen from './components/AuthScreen.vue'
import StaticView from './components/static/StaticView.vue'
import LeasesTab from './components/leases/LeasesTab.vue'
import AuditTab from './components/audit/AuditTab.vue'
import DnsmasqConfig from './components/config/DnsmasqConfig.vue'

const { locale } = useI18n()

let arpInterval = null

function toggleTheme() {
  const isDark = document.documentElement.getAttribute('data-bs-theme') === 'dark'
  document.documentElement.setAttribute('data-bs-theme', isDark ? 'light' : 'dark')
  localStorage.setItem('theme', isDark ? 'light' : 'dark')
}

function switchLocale() {
  const next = locale.value === 'ru' ? 'en' : 'ru'
  locale.value = next
  localStorage.setItem('locale', next)
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
  <div class="container-fluid mt-4 px-4 px-lg-5">
    
    <div class="d-flex justify-content-between align-items-center mb-4">
        <h2 class="fw-bold d-flex align-items-center mb-0">
            <span class="text-primary me-2">🛡️ Intermasq</span>
            <span class="badge" :class="store.isDnsmasqActive ? 'bg-success' : 'bg-danger'" style="font-size: 0.6rem; margin-top: 5px;">
                {{ store.isDnsmasqActive ? '🟢 ' + $t('app.statusActive') : '🔴 ' + $t('app.statusStopped') }}
            </span>
        </h2>
        
        <div v-if="store.token" class="dropdown">
            <button class="btn btn-outline-secondary dropdown-toggle" type="button" data-bs-toggle="dropdown">
                ⚙️ {{ $t('app.menu') }}
            </button>
            <ul class="dropdown-menu dropdown-menu-end shadow-sm">
                <li><h6 class="dropdown-header">{{ $t('app.plugins') }}</h6></li>
                
                <li v-for="p in store.plugins" :key="p.id">
                    <a class="dropdown-item" href="#" @click.prevent="store.tab = 'plugin-' + p.id">
                        🧩 {{ p.name }}
                    </a>
                </li>
                <li v-if="store.plugins.length === 0"><span class="dropdown-item text-muted small">{{ $t('app.noPlugins') }}</span></li>

                <li><hr class="dropdown-divider"></li>
                <li><h6 class="dropdown-header">{{ $t('app.system') }}</h6></li>
                <li><a class="dropdown-item" href="#" @click.prevent="switchLocale">🌐 {{ locale === 'ru' ? 'English' : 'Русский' }}</a></li>
                <li><a class="dropdown-item" href="#" @click.prevent="toggleTheme">🌓 {{ $t('app.toggleTheme') }}</a></li>
                <li><a class="dropdown-item" href="/swagger/index.html" target="_blank">📖 {{ $t('app.apiDocs') }}</a></li>
                <li><a class="dropdown-item" href="#" @click.prevent="actions.restartSystem()">🔄 {{ $t('app.restart') }}</a></li>
                <li><hr class="dropdown-divider"></li>
                <li><a class="dropdown-item text-danger" href="#" @click.prevent="actions.logout()">🚪 {{ $t('app.logout') }}</a></li>
            </ul>
        </div>
    </div>

    <div v-if="store.view === 'error'" class="alert alert-danger text-center">
        <h4>🔌 {{ $t('app.serverUnavailable') }}</h4> 
        <button @click="actions.checkStatus()" class="btn btn-sm btn-danger">{{ $t('app.retryConnection') }}</button>
    </div>

    <AuthScreen v-if="store.view === 'login' || store.view === 'setup'" />

    <div v-if="store.view === 'dashboard'">
        
        <div v-if="!store.tab.startsWith('plugin-')">
            <div class="row mb-3 align-items-center g-2">
                <div class="col-12 col-md-auto">
                    <div class="btn-group w-100">
                        <button @click="store.tab='static'" :class="['btn', store.tab==='static' ? 'btn-primary' : 'btn-outline-primary']">{{ $t('app.tabStatic') }}</button>
                        <button @click="store.tab='leases'" :class="['btn', store.tab==='leases' ? 'btn-primary' : 'btn-outline-primary']">{{ $t('app.tabLeases') }}</button>
                        <button @click="store.tab='config'" :class="['btn', store.tab==='config' ? 'btn-primary' : 'btn-outline-primary']">⚙️ {{ $t('app.tabConfig') }}</button>
                        <button @click="store.tab='audit'" :class="['btn', store.tab==='audit' ? 'btn-primary' : 'btn-outline-primary']">{{ $t('app.tabAudit') }}</button>
                    </div>
                </div>
                <div v-if="store.tab !== 'config'" class="col-12 col-md">
                    <input v-model="store.searchQuery" type="text" class="form-control" :placeholder="$t('app.searchPlaceholder')">
                </div>
                <div class="col-12 col-md-auto text-md-end d-flex gap-2 justify-content-end">
                    <button @click="actions.downloadBackup()" class="btn btn-outline-info fw-bold" :title="$t('app.downloadArchive')">💾 {{ $t('app.backup') }}</button>
                    <button @click="actions.downloadCSV()" class="btn btn-outline-success fw-bold" :title="$t('app.csvExportTooltip')">📥 {{ $t('app.csvExport') }}</button>
                    <button @click="actions.applyConfig()" class="btn btn-warning fw-bold text-dark">🔄 {{ $t('app.apply') }}</button>
                </div>
            </div>

            <StaticView v-if="store.tab === 'static'" />
            <LeasesTab v-if="store.tab === 'leases'" />
            <DnsmasqConfig v-if="store.tab === 'config'" />
            <AuditTab v-if="store.tab === 'audit'" />
        </div>

        <!-- 2. ОКНО ПЛАГИНА (полная ширина viewport, поверх приложения) -->
        <div v-if="store.tab.startsWith('plugin-')" class="plugin-overlay fade-in">
            <div class="card shadow-sm w-100 border-0">
                <div class="card-header bg-dark text-white d-flex justify-content-between align-items-center py-3">
                    <span class="fw-bold fs-5">🧩 {{ $t('app.plugin') }}</span>
                    <button class="btn btn-sm btn-outline-light px-3" @click="store.tab='static'">✕ {{ $t('app.close') }}</button>
                </div>
                <div class="card-body p-0 w-100" style="height: calc(100vh - 120px); min-height: 600px;">
                    <iframe
                        :src="'/plugins/' + store.tab.replace('plugin-', '') + '/'"
                        style="width: 100%; height: 100%; border: none; display: block;">
                    </iframe>
                </div>
            </div>
        </div>
        
    </div>
  </div>
</template>

<style>
.fade-in { animation: fadeIn 0.3s ease-in-out; }
@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

.plugin-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 1050;
    background: var(--bs-body-bg, #fff);
    padding: 1rem;
    overflow: auto;
}</style>

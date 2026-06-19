<template>
  <div class="fade-in">
    <HostForm 
        :selectedFile="selectedFile"
        :editData="editData"
        @cancel-edit="cancelEdit"
    />

    <div class="d-flex justify-content-between align-items-center mb-3">
        <ul class="nav nav-tabs mb-0 flex-grow-1 border-bottom-0">
            <li class="nav-item">
                <a class="nav-link" :class="{active: selectedFile === 'all'}" href="#" @click.prevent="selectedFile='all'">{{ $t('hosts.allFiles') }}</a>
            </li>
            <li class="nav-item" v-for="file in uniqueFiles" :key="file">
                <a class="nav-link" :class="{active: selectedFile === file}" href="#" @click.prevent="selectedFile=file">
                    {{ file.split('/').pop() }}
                </a>
            </li>
        </ul>
        
        <button v-if="selectedFile !== 'all' && hasBackup" @click="rollbackFile" class="btn btn-sm btn-outline-warning ms-2" :title="$t('hosts.rollbackTooltip')">
            ⏪ {{ $t('hosts.rollback') }}
        </button>
    </div>

    <HostTable 
        :selectedFile="selectedFile"
        @edit-host="startEdit"
    />
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { translateApiError } from '../../i18n.js'
import HostForm from './HostForm.vue'
import HostTable from './HostTable.vue'

const { t } = useI18n()

const selectedFile = ref('all')
const editData = ref(null)

const cleanPath = (path) => path ? path.split('|')[0] : ''

const uniqueFiles = computed(() => {
    return Array.from(new Set(store.hosts.map(h => cleanPath(h.file)))).sort()
})

const hasBackup = computed(() => {
    if (selectedFile.value === 'all') return false
    return store.hosts.some(h => cleanPath(h.file) === selectedFile.value && h.file.includes('|has_bak'))
})

function startEdit(host) {
    editData.value = { ...host, file: cleanPath(host.file) }
    window.scrollTo({ top: 0, behavior: 'smooth' })
}

function cancelEdit() {
    editData.value = null
}

watch(() => store.transferData, (val) => {
    if (val && val.mac) {
        editData.value = null
        store.transferData = null
    }
}, { deep: true })

async function rollbackFile() {
    if(!confirm(t('confirm.rollback', { file: selectedFile.value.split('/').pop() }))) return
    try { 
        await api.post('/rollback', { file: selectedFile.value })
        actions.loadData()
        alert(t('alert.rollbackSuccess'))
    } catch (e) { 
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.rollbackError')
        alert(msg)
    }
}
</script>

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

        <div class="d-flex gap-2">
            <button v-if="selectedFile !== 'all'" @click="showHistory = true" class="btn btn-sm btn-outline-secondary" :title="$t('history.iconTooltip')">
                🕒 {{ $t('history.icon') }}
            </button>
            <button v-if="selectedFile !== 'all' && hasBackup" @click="rollbackFile" class="btn btn-sm btn-outline-warning" :title="$t('hosts.rollbackTooltip')">
                ⏪ {{ $t('hosts.rollback') }}
            </button>
        </div>
    </div>

    <HostTable
        :selectedFile="selectedFile"
        @edit-host="startEdit"
    />

    <HistoryModal
        :show="showHistory"
        :file="selectedFile"
        @close="showHistory = false"
        @restored="actions.loadData()"
    />
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { toast } from '../../toast.js'
import { translateApiError } from '../../i18n.js'
import HostForm from './HostForm.vue'
import HostTable from './HostTable.vue'
import HistoryModal from '../history/HistoryModal.vue'

const { t } = useI18n()

const selectedFile = ref('all')
const editData = ref(null)
const showHistory = ref(false)

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

async function rollbackFile() {
    if(!confirm(t('confirm.rollback', { file: selectedFile.value.split('/').pop() }))) return
    try {
        await api.post('/rollback', { file: selectedFile.value })
        actions.loadData()
        toast.success(t('alert.rollbackSuccess'))
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.rollbackError')
        toast.error(msg)
    }
}
</script>

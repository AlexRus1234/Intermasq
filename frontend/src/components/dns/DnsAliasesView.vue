<template>
  <div class="fade-in">
    <AliasForm
        :selectedFile="selectedFile"
        :editData="editData"
        @cancel-edit="cancelEdit"
    />

    <div class="d-flex justify-content-between align-items-center mb-3">
        <ul class="nav nav-tabs mb-0 flex-grow-1 border-bottom-0">
            <li class="nav-item">
                <a class="nav-link" :class="{active: selectedFile === 'all'}" href="#" @click.prevent="selectedFile='all'">{{ $t('dns.allFiles') }}</a>
            </li>
            <li class="nav-item" v-for="file in uniqueFiles" :key="file">
                <a class="nav-link" :class="{active: selectedFile === file}" href="#" @click.prevent="selectedFile=file">
                    {{ file.split('/').pop() }}
                </a>
            </li>
        </ul>

        <button v-if="selectedFile !== 'all' && hasBackup" @click="rollbackFile" class="btn btn-sm btn-outline-warning ms-2" :title="$t('dns.rollbackTooltip')">
            ⏪ {{ $t('dns.rollback') }}
        </button>
    </div>

    <AliasTable
        :selectedFile="selectedFile"
        @edit-alias="startEdit"
    />
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { translateApiError } from '../../i18n.js'
import AliasForm from './AliasForm.vue'
import AliasTable from './AliasTable.vue'

const { t } = useI18n()

const selectedFile = ref('all')
const editData = ref(null)

const cleanPath = (path) => path ? path.split('|')[0] : ''

const uniqueFiles = computed(() => {
    return Array.from(new Set(store.aliases.map(a => cleanPath(a.file)))).sort()
})

const hasBackup = computed(() => {
    if (selectedFile.value === 'all') return false
    return store.aliases.some(a => cleanPath(a.file) === selectedFile.value && a.file.includes('|has_bak'))
})

function startEdit(alias) {
    editData.value = { ...alias, file: cleanPath(alias.file) }
    window.scrollTo({ top: 0, behavior: 'smooth' })
}

function cancelEdit() {
    editData.value = null
}

async function rollbackFile() {
    if (!confirm(t('confirm.rollback', { file: selectedFile.value.split('/').pop() }))) return
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

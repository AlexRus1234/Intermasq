<template>
  <div class="fade-in">
    <!-- ФОРМА (Одиночная или Массовая) -->
    <HostForm 
        :selectedFile="selectedFile"
        :editData="editData"
        @cancel-edit="cancelEdit"
    />

    <!-- ВКЛАДКИ ФАЙЛОВ И КНОПКА ОТКАТА -->
    <div class="d-flex justify-content-between align-items-center mb-3">
        <ul class="nav nav-tabs mb-0 flex-grow-1 border-bottom-0">
            <li class="nav-item">
                <a class="nav-link" :class="{active: selectedFile === 'all'}" href="#" @click.prevent="selectedFile='all'">Все файлы</a>
            </li>
            <li class="nav-item" v-for="file in uniqueFiles" :key="file">
                <a class="nav-link" :class="{active: selectedFile === file}" href="#" @click.prevent="selectedFile=file">
                    {{ file.split('/').pop() }}
                </a>
            </li>
        </ul>
        
        <button v-if="selectedFile !== 'all' && hasBackup" @click="rollbackFile" class="btn btn-sm btn-outline-warning ms-2" title="Отменить последнее изменение файла">
            ⏪ Откат
        </button>
    </div>

    <!-- ТАБЛИЦА ХОСТОВ -->
    <HostTable 
        :selectedFile="selectedFile"
        @edit-host="startEdit"
    />
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { store, api, actions } from '../../store.js'
import HostForm from './HostForm.vue'
import HostTable from './HostTable.vue'

// Локальное состояние: какой файл сейчас выбран в фильтре
const selectedFile = ref('all')

// Локальное состояние: данные для редактирования
const editData = ref(null)

// Очищаем путь от нашего технического суффикса |has_bak
const cleanPath = (path) => path ? path.split('|')[0] : ''

// Собираем уникальные имена файлов из store.hosts
const uniqueFiles = computed(() => {
    return Array.from(new Set(store.hosts.map(h => cleanPath(h.file)))).sort()
})

// Проверяем, есть ли бэкап у выбранного файла (для кнопки Откат)
const hasBackup = computed(() => {
    if (selectedFile.value === 'all') return false
    return store.hosts.some(h => cleanPath(h.file) === selectedFile.value && h.file.includes('|has_bak'))
})

// Когда нажимаем "Редактировать" в таблице
function startEdit(host) {
    editData.value = { ...host, file: cleanPath(host.file) }
    window.scrollTo({ top: 0, behavior: 'smooth' })
}

// Отмена редактирования
function cancelEdit() {
    editData.value = null
}

// Если пришли данные с вкладки "Аренды" (кнопка 'В статику')
watch(() => store.transferData, (val) => {
    if (val && val.mac) {
        editData.value = null // Сбрасываем режим редактирования
        // Передаем данные в форму через трюк (создадим фейковый editData)
        // В HostForm мы перехватим это как новые данные, а не редактирование
        store.transferData = null // Очищаем после использования
    }
}, { deep: true })

async function rollbackFile() {
    if(!confirm(`Отменить последнее изменение в файле ${selectedFile.value.split('/').pop()}?`)) return
    try { 
        await api.post('/rollback', { file: selectedFile.value })
        actions.loadData()
        alert('Откат выполнен успешно!')
    } catch (e) { alert(e.response?.data?.error || "Ошибка отката") }
}
</script>

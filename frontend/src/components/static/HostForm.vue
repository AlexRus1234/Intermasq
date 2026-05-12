<template>
  <div :class="['card mb-4 p-3 shadow-sm', isEditing ? 'border-warning' : 'border-primary']">
      <div class="d-flex justify-content-between align-items-center mb-2">
          <h6 :class="isEditing ? 'text-warning text-dark' : 'text-primary'" class="mb-0 fw-bold">
              {{ isEditing ? '✏️ Редактирование' : (selectedFile === 'all' ? '➕ Новое устройство' : '➕ Добавить в ' + selectedFile.split('/').pop()) }}
          </h6>
          
          <div v-if="!isEditing" class="form-check form-switch mb-0">
              <input class="form-check-input" type="checkbox" id="importMode" v-model="isImportMode">
              <label class="form-check-label small text-muted" for="importMode">Импорт списком</label>
          </div>
          
          <button v-if="isEditing" @click="$emit('cancel-edit')" class="btn btn-sm btn-outline-secondary">✕ Отмена</button>
      </div>
      
      <!-- ОДИНОЧНОЕ ДОБАВЛЕНИЕ / РЕДАКТИРОВАНИЕ -->
      <div v-if="!isImportMode" class="row g-2">
        <div class="col-md-3"><input v-model="form.mac" placeholder="MAC (aa:bb...)" class="form-control"></div>
        <div class="col-md-3"><input v-model="form.ip" placeholder="IP (172.20...)" class="form-control"></div>
        <div class="col-md-3"><input v-model="form.hostname" placeholder="Hostname" class="form-control"></div>
        <div class="col-md-3">
            <div class="input-group">
                <input v-model="form.file" :readonly="selectedFile !== 'all' && !isEditing" :class="['form-control', (selectedFile !== 'all' && !isEditing) ? 'bg-light' : '']" placeholder="Файл (/etc/...)">
                
                <!-- ИСПРАВЛЕННАЯ СТРОКА -->
                <button @click="saveHost" class="btn fw-bold" :class="isEditing ? 'btn-warning' : 'btn-success'">
                    {{ isEditing ? 'Сохранить' : 'Добавить' }}
                </button>
            </div>
        </div>
      </div>

      <!-- МАССОВЫЙ ИМПОРТ -->
      <div v-if="isImportMode" class="row g-2 fade-in">
          <div class="col-12">
              <textarea v-model="bulkText" class="form-control font-monospace" rows="4" placeholder="Вставьте текст (MAC IP Hostname) через пробел или Tab. Каждое устройство с новой строки."></textarea>
          </div>
          <div class="col-12 d-flex justify-content-between align-items-center">
              <span class="text-muted small">Распознано: <strong>{{ parsedBulkHosts.length }}</strong> устройств</span>
              <div class="input-group" style="width: auto;">
                  <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" placeholder="Файл назначения">
                  <button @click="saveBulkHosts" class="btn btn-success fw-bold" :disabled="parsedBulkHosts.length === 0">Импортировать</button>
              </div>
          </div>
      </div>
  </div>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { store, api, actions } from '../../store.js'

const props = defineProps(['selectedFile', 'editData'])
const emit = defineEmits(['cancel-edit'])

const isImportMode = ref(false)
const bulkText = ref('')
const originalMac = ref('')
const originalFile = ref('')
const form = ref({ mac: '', ip: '', hostname: '', file: '' })

const isEditing = computed(() => originalMac.value !== '')

watch(() => props.selectedFile, (newFile) => {
    if (!isEditing.value) {
        form.value.file = newFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : newFile
    }
}, { immediate: true })

watch(() => props.editData, (newData) => {
    if (newData) {
        isImportMode.value = false
        originalMac.value = newData.mac
        originalFile.value = newData.file
        form.value = { mac: newData.mac, ip: newData.ip, hostname: newData.hostname, file: newData.file }
    } else {
        originalMac.value = ''
        originalFile.value = ''
        form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
        form.value.file = props.selectedFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : props.selectedFile
    }
})

watch(() => store.transferData, (data) => {
    if (data) {
        isImportMode.value = false
        emit('cancel-edit')
        form.value.mac = data.mac
        form.value.ip = data.ip
        form.value.hostname = data.hostname
    }
})

async function saveHost() {
  const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i;
  if (!macRegex.test(form.value.mac)) { alert('Неверный формат MAC'); return; }
  if (!form.value.file) { alert('Укажите имя файла'); return; }
  
  try {
    if (isEditing.value) {
        await api.delete(`/hosts/${originalMac.value}?file=${encodeURIComponent(originalFile.value)}`)
    }
    await api.post('/hosts', form.value)
    
    emit('cancel-edit')
    form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
    actions.loadData()
  } catch (e) { alert(e.response?.data?.error || "Ошибка сохранения") }
}

const parsedBulkHosts = computed(() => {
    return bulkText.value.split('\n').map(line => {
        const p = line.trim().split(/\s+/)
        if(p.length >= 3) {
            return { 
                mac: p.find(x => /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i.test(x)) || '', 
                ip: p.find(x => /^(\d{1,3}\.){3}\d{1,3}$/.test(x)) || '', 
                hostname: p.find(x => !x.includes(':') && !x.includes('.')) || p[p.length-1] 
            }
        }
        return null
    }).filter(e => e && e.mac && e.ip)
})

async function saveBulkHosts() {
    if (parsedBulkHosts.value.length === 0 || !form.value.file) return
    try { 
        await api.post('/hosts/bulk', { file: form.value.file, hosts: parsedBulkHosts.value })
        bulkText.value = ''
        actions.loadData()
        alert(`Успешно импортировано ${parsedBulkHosts.value.length} устройств!`)
    } catch (e) { alert(e.response?.data?.error || "Ошибка импорта") }
}
</script>

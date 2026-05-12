<template>
  <div>
    <!-- Add/Edit/Import Form -->
    <div :class="['card mb-4 p-3 shadow-sm', isEditing ? 'border-warning' : 'border-primary']">
        <div class="d-flex justify-content-between align-items-center mb-2">
            <h6 :class="isEditing ? 'text-warning text-dark' : 'text-primary'" class="mb-0 fw-bold">
                {{ isEditing ? '✏️ Редактирование' : (selectedFile === 'all' ? '➕ Устройство' : '➕ В ' + selectedFile.split('/').pop()) }}
            </h6>
            <!-- Переключатель режима импорта -->
            <div v-if="!isEditing" class="form-check form-switch mb-0">
                <input class="form-check-input" type="checkbox" id="importMode" v-model="isImportMode">
                <label class="form-check-label small" for="importMode">Массовый импорт</label>
            </div>
            <button v-if="isEditing" @click="cancelEdit" class="btn btn-sm btn-outline-secondary">✕ Отмена</button>
        </div>
        
        <!-- Одиночный режим -->
        <div v-if="!isImportMode" class="row g-2">
          <div class="col-md-3"><input v-model="newHost.mac" placeholder="MAC (aa:bb...)" class="form-control"></div>
          <div class="col-md-3"><input v-model="newHost.ip" placeholder="IP (172.20...)" class="form-control"></div>
          <div class="col-md-3"><input v-model="newHost.hostname" placeholder="Hostname" class="form-control"></div>
          <div class="col-md-3">
              <div class="input-group">
                  <input v-model="newHost.file" :readonly="selectedFile !== 'all' && !isEditing" :class="['form-control', (selectedFile !== 'all' && !isEditing) ? 'bg-light' : '']" placeholder="Файл (/etc/...)">
                  <button @click="saveHost" :class="['btn', isEditing ? 'btn-warning' : 'btn-success']">{{ isEditing ? 'Сохранить' : 'Добавить' }}</button>
              </div>
          </div>
        </div>

        <!-- Массовый режим -->
        <div v-if="isImportMode" class="row g-2 fade-in">
            <div class="col-12">
                <textarea v-model="bulkText" class="form-control text-monospace" rows="4" placeholder="Вставьте текст (MAC IP Hostname) через пробел или Tab. Каждое устройство с новой строки."></textarea>
                <div class="form-text">Пример: AA:BB:CC:11:22:33 192.168.1.10 my-server</div>
            </div>
            <div class="col-12 d-flex justify-content-between align-items-center">
                <span class="text-muted small">Распознано: {{ parsedBulkHosts.length }} устройств</span>
                <div class="input-group" style="width: auto;">
                    <input v-model="newHost.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" placeholder="Файл назначения">
                    <button @click="saveBulkHosts" class="btn btn-success" :disabled="parsedBulkHosts.length===0">Импортировать</button>
                </div>
            </div>
        </div>
    </div>

    <!-- File Tabs & Bulk Delete -->
    <div class="d-flex justify-content-between align-items-center mb-3">
        <ul class="nav nav-tabs mb-0 flex-grow-1 border-bottom-0">
            <li class="nav-item"><a class="nav-link" :class="{active: selectedFile === 'all'}" href="#" @click.prevent="selectFileFilter('all')">Все файлы</a></li>
            <li class="nav-item" v-for="file in uniqueFiles" :key="file">
                <a class="nav-link" :class="{active: selectedFile === file}" href="#" @click.prevent="selectFileFilter(file)">
                    {{ file.split('/').pop() }}
                </a>
            </li>
        </ul>
        
        <!-- Кнопка Отката (появляется если у выбранного файла есть бэкап) -->
        <button v-if="selectedFile !== 'all' && hasBackup" @click="rollbackFile" class="btn btn-sm btn-outline-warning ms-2 fade-in" title="Отменить последнее изменение файла">
            ⏪ Откат (Undo)
        </button>

        <button v-if="selectedHosts.length > 0" @click="bulkDelete" class="btn btn-sm btn-danger ms-2 fade-in">🗑️ Удалить ({{ selectedHosts.length }})</button>
    </div>

    <!-- Table -->
    <div class="card shadow-sm border-top-0" style="border-top-left-radius: 0;">
        <table class="table table-hover mb-0 align-middle">
        <thead class="table-light">
            <tr>
                <th style="width: 40px;"><input type="checkbox" class="form-check-input" v-model="allSelected"></th>
                <th>Онлайн</th> <!-- НОВАЯ КОЛОНКА -->
                <th @click="sortBy('mac')" style="cursor:pointer">MAC ↕</th>
                <th @click="sortBy('ip')" style="cursor:pointer">IP ↕</th>
                <th @click="sortBy('hostname')" style="cursor:pointer">Hostname ↕</th>
                <th v-if="selectedFile==='all'">Файл</th>
                <th class="text-end">Действия</th>
            </tr>
        </thead>
        <tbody>
            <tr v-for="h in sortedHosts" :key="h.mac" :class="{'table-active': isSelected(h)}">
                <td><input type="checkbox" class="form-check-input" :checked="isSelected(h)" @change="toggleSelection(h)"></td>
                
                <!-- НОВЫЙ ИНДИКАТОР ARP -->
                <td class="text-center">
                    <span v-if="arp[h.mac.toLowerCase()]" title="Устройство в сети" class="text-success">🟢</span>
                    <span v-else title="Офлайн" class="text-muted" style="opacity: 0.3;">🔴</span>
                </td>

                <td class="font-monospace">{{ h.mac }}</td>
                <td class="fw-bold text-primary"><span v-if="searchQuery && h.ip.includes(searchQuery)" class="bg-warning text-dark px-1 rounded">{{ h.ip }}</span><span v-else>{{ h.ip }}</span></td>
                <td>{{ h.hostname }}</td>
                <td v-if="selectedFile==='all'" class="small text-muted">{{ h.file.split('|')[0].split('/').pop() }}</td>
                <td class="text-end">
                    <div class="btn-group">
                        <button @click="editHost(h)" class="btn btn-sm btn-outline-secondary" title="Редактировать">✏️</button>
                        <button @click="deleteHost(h.mac, h.file.split('|')[0])" class="btn btn-sm btn-outline-danger" title="Удалить">✕</button>
                    </div>
                </td>
            </tr>
            <tr v-if="sortedHosts.length === 0"><td colspan="7" class="text-center p-4 text-muted">{{ searchQuery ? 'Ничего не найдено' : 'Пусто' }}</td></tr>
        </tbody>
        </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const props = defineProps(['hosts', 'searchQuery', 'api', 'newHostForm', 'arp'])
const emit = defineEmits(['reload'])

const sortKey = ref('ip')
const sortAsc = ref(true)
const selectedFile = ref('all') 
const selectedHosts = ref([])

const newHost = ref(props.newHostForm || { mac: '', ip: '', hostname: '', file: '' })
watch(() => props.newHostForm, (val) => { if(val.mac) newHost.value = { ...val } }, { deep: true })

const isEditing = ref(false)
const originalHost = ref(null)

// НОВОЕ: Состояние массового импорта
const isImportMode = ref(false)
const bulkText = ref('')

function selectFileFilter(file) {
    selectedFile.value = file
    selectedHosts.value = []
    if (file !== 'all') newHost.value.file = file
}

function sortBy(key) {
  if (sortKey.value === key) sortAsc.value = !sortAsc.value
  else { sortKey.value = key; sortAsc.value = true }
}

// Очищаем пути от маркера |has_bak
const cleanFilePath = (path) => path.split('|')[0]

// Уникальные файлы (ищем хотя бы одну запись с маркером |has_bak для кнопки Отката)
const uniqueFiles = computed(() => Array.from(new Set(props.hosts.map(h => cleanFilePath(h.file)))).sort())

const hasBackup = computed(() => {
    if (selectedFile.value === 'all') return false
    return props.hosts.some(h => cleanFilePath(h.file) === selectedFile.value && h.file.includes('|has_bak'))
})

const sortedHosts = computed(() => {
  let data = [...props.hosts]
  if (selectedFile.value !== 'all') data = data.filter(h => cleanFilePath(h.file) === selectedFile.value)
  if (props.searchQuery) {
      const q = props.searchQuery.toLowerCase()
      data = data.filter(h => (h.mac && h.mac.toLowerCase().includes(q)) || (h.ip && h.ip.toLowerCase().includes(q)) || (h.hostname && h.hostname.toLowerCase().includes(q)) || (selectedFile.value === 'all' && h.file && cleanFilePath(h.file).toLowerCase().includes(q)))
  }
  return data.sort((a, b) => {
    let valA = a[sortKey.value] || ''; let valB = b[sortKey.value] || '';
    if (sortKey.value === 'ip') {
       const numA = (valA.split('.') || []).map(Number); const numB = (valB.split('.') || []).map(Number);
       if(numA.length!==4 || numB.length!==4) return 0;
       for(let i=0; i<4; i++) { if (numA[i] !== numB[i]) return sortAsc.value ? numA[i] - numB[i] : numB[i] - numA[i]; }
       return 0;
    }
    if (valA < valB) return sortAsc.value ? -1 : 1
    if (valA > valB) return sortAsc.value ? 1 : -1
    return 0
  })
})

// Парсинг текста для массового импорта
const parsedBulkHosts = computed(() => {
    const lines = bulkText.value.split('\n')
    const results = []
    const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i
    
    for (const line of lines) {
        const parts = line.trim().split(/\s+/) // Разделяем по пробелу или табуляции
        if (parts.length >= 3) {
            let entry = { mac: '', ip: '', hostname: '' }
            for (const p of parts) {
                if (macRegex.test(p)) entry.mac = p
                else if (/^(\d{1,3}\.){3}\d{1,3}$/.test(p)) entry.ip = p
                else entry.hostname = p
            }
            if (entry.mac && entry.ip && entry.hostname) results.push(entry)
        }
    }
    return results
})

const allSelected = computed({
  get: () => sortedHosts.value.length > 0 && selectedHosts.value.length === sortedHosts.value.length,
  set: (val) => {
    if (val) selectedHosts.value = sortedHosts.value.map(h => ({ mac: h.mac, file: cleanFilePath(h.file) }))
    else selectedHosts.value = []
  }
})

function toggleSelection(host) {
  const idx = selectedHosts.value.findIndex(h => h.mac === host.mac)
  if (idx > -1) selectedHosts.value.splice(idx, 1)
  else selectedHosts.value.push({ mac: host.mac, file: cleanFilePath(host.file) })
}

function isSelected(host) { return selectedHosts.value.some(h => h.mac === host.mac) }

function editHost(host) {
    isEditing.value = true; isImportMode.value = false
    originalHost.value = { mac: host.mac, file: cleanFilePath(host.file) }
    newHost.value = { mac: host.mac, ip: host.ip, hostname: host.hostname, file: cleanFilePath(host.file) }
    window.scrollTo({ top: 0, behavior: 'smooth' })
}

function cancelEdit() {
    isEditing.value = false; originalHost.value = null
    newHost.value = { mac: '', ip: '', hostname: '', file: selectedFile.value === 'all' ? cleanFilePath(props.hosts[0]?.file || '') : selectedFile.value }
}

async function saveHost() {
  const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/;
  if (!macRegex.test(newHost.value.mac)) { alert('Неверный формат MAC'); return; }
  if (!newHost.value.file) { alert('Укажите имя файла'); return; }
  
  try {
    if (isEditing.value) await props.api.delete(`/hosts/${originalHost.value.mac}?file=${encodeURIComponent(originalHost.value.file)}`)
    await props.api.post('/hosts', newHost.value)
    cancelEdit(); emit('reload')
  } catch (e) { alert(e.response?.data?.error || "Ошибка сохранения") }
}

// НОВОЕ: Сохранение массового импорта
async function saveBulkHosts() {
    if (parsedBulkHosts.value.length === 0) return
    if (!newHost.value.file) { alert('Укажите файл назначения'); return }
    try {
        await props.api.post('/hosts/bulk', { file: newHost.value.file, hosts: parsedBulkHosts.value })
        bulkText.value = ''; emit('reload')
        alert(`Импортировано ${parsedBulkHosts.value.length} устройств!`)
    } catch (e) { alert(e.response?.data?.error || "Ошибка импорта") }
}

async function deleteHost(mac, file) {
  if(!confirm('Удалить запись ' + mac + '?')) return
  try { await props.api.delete(`/hosts/${mac}?file=${encodeURIComponent(file)}`); emit('reload') } catch (e) { alert("Ошибка удаления") }
}

async function bulkDelete() {
  if(!confirm(`Удалить ${selectedHosts.value.length} записей?`)) return
  try {
    await Promise.all(selectedHosts.value.map(h => props.api.delete(`/hosts/${h.mac}?file=${encodeURIComponent(h.file)}`)))
    selectedHosts.value = []; emit('reload')
  } catch (e) { alert("Ошибка массового удаления") }
}

// НОВОЕ: Вызов отката
async function rollbackFile() {
    if(!confirm('Внимание! Это отменит последнее добавление или удаление в этом файле. Продолжить?')) return
    try {
        await props.api.post('/rollback', { file: selectedFile.value })
        emit('reload')
        alert('Файл восстановлен из бэкапа!')
    } catch (e) { alert(e.response?.data?.error || "Ошибка отката") }
}
</script>

<template>
  <div class="card shadow-sm border-top-0" style="border-top-left-radius: 0;">
      
      <!-- Кнопка массового удаления -->
      <div v-if="selectedHosts.length > 0" class="bg-danger text-white p-2 d-flex justify-content-between align-items-center fade-in">
          <span class="fw-bold ms-2">Выбрано элементов: {{ selectedHosts.length }}</span>
          <button @click="bulkDelete" class="btn btn-sm btn-light text-danger fw-bold">🗑️ Удалить выбранные</button>
      </div>

      <table class="table table-hover mb-0 align-middle">
      <thead class="table-light">
          <tr>
              <th style="width: 40px;"><input type="checkbox" class="form-check-input" v-model="allSelected"></th>
              <th style="width: 50px;">Онлайн</th>
              <th @click="sortBy('mac')" style="cursor:pointer">MAC ↕</th>
              <th @click="sortBy('ip')" style="cursor:pointer">IP ↕</th>
              <th @click="sortBy('hostname')" style="cursor:pointer">Hostname ↕</th>
              <th v-if="selectedFile==='all'">Файл</th>
              <th class="text-end">Действия</th>
          </tr>
      </thead>
      <tbody>
          <tr v-for="h in sortedHosts" :key="h.mac" :class="{'table-active': isSelected(h)}">
              <td>
                  <input type="checkbox" class="form-check-input" :checked="isSelected(h)" @change="toggleSelection(h)">
              </td>
              <td class="text-center">
                  <span v-if="store.arpTable[h.mac.toLowerCase()]" title="В сети" class="text-success">🟢</span>
                  <span v-else class="text-muted" style="opacity: 0.3;">🔴</span>
              </td>
              <td class="font-monospace">{{ h.mac }}</td>
              <td class="fw-bold text-primary">
                  <span v-if="store.searchQuery && h.ip.includes(store.searchQuery)" class="bg-warning text-dark px-1 rounded">{{ h.ip }}</span>
                  <span v-else>{{ h.ip }}</span>
              </td>
              <td>{{ h.hostname }}</td>
              <td v-if="selectedFile==='all'" class="small text-muted">{{ cleanPath(h.file).split('/').pop() }}</td>
              <td class="text-end">
                  <div class="btn-group">
                      <button @click="$emit('edit-host', h)" class="btn btn-sm btn-outline-secondary" title="Редактировать">✏️</button>
                      <button @click="deleteHost(h.mac, cleanPath(h.file))" class="btn btn-sm btn-outline-danger" title="Удалить">✕</button>
                  </div>
              </td>
          </tr>
          <tr v-if="sortedHosts.length === 0">
              <td colspan="7" class="text-center p-4 text-muted">
                  {{ store.searchQuery ? 'По запросу ничего не найдено' : 'Список пуст' }}
              </td>
          </tr>
      </tbody>
      </table>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { store, api, actions } from '../../store.js'

const props = defineProps(['selectedFile'])
const emit = defineEmits(['edit-host'])

const sortKey = ref('ip')
const sortAsc = ref(true)
const selectedHosts = ref([])

// Очищаем выбор при смене файла-фильтра
watch(() => props.selectedFile, () => { selectedHosts.value = [] })

const cleanPath = (path) => path ? path.split('|')[0] : ''

// Сортировка
function sortBy(key) {
  if (sortKey.value === key) sortAsc.value = !sortAsc.value
  else { sortKey.value = key; sortAsc.value = true }
}

// Фильтрация (по файлу и поиску) + Сортировка
const sortedHosts = computed(() => {
  let data = [...store.hosts]
  
  if (props.selectedFile !== 'all') {
      data = data.filter(h => cleanPath(h.file) === props.selectedFile)
  }
  
  if (store.searchQuery) {
      const q = store.searchQuery.toLowerCase()
      data = data.filter(h => 
          (h.mac && h.mac.toLowerCase().includes(q)) || 
          (h.ip && h.ip.toLowerCase().includes(q)) || 
          (h.hostname && h.hostname.toLowerCase().includes(q))
      )
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

// Чекбоксы (Массовый выбор)
const allSelected = computed({
  get: () => sortedHosts.value.length > 0 && selectedHosts.value.length === sortedHosts.value.length,
  set: (val) => {
    if (val) selectedHosts.value = sortedHosts.value.map(h => ({ mac: h.mac, file: cleanPath(h.file) }))
    else selectedHosts.value = []
  }
})

function toggleSelection(host) {
  const idx = selectedHosts.value.findIndex(h => h.mac === host.mac)
  if (idx > -1) selectedHosts.value.splice(idx, 1)
  else selectedHosts.value.push({ mac: host.mac, file: cleanPath(host.file) })
}

function isSelected(host) {
    return selectedHosts.value.some(h => h.mac === host.mac)
}

// Удаление
async function deleteHost(mac, file) {
  if(!confirm(`Удалить запись ${mac}?`)) return
  try { 
      await api.delete(`/hosts/${mac}?file=${encodeURIComponent(file)}`)
      actions.loadData() 
  } catch (e) { alert("Ошибка удаления") }
}

async function bulkDelete() {
  if(!confirm(`Точно удалить ${selectedHosts.value.length} записей?`)) return
  try {
    await Promise.all(selectedHosts.value.map(h => api.delete(`/hosts/${h.mac}?file=${encodeURIComponent(h.file)}`)))
    selectedHosts.value = []
    actions.loadData()
  } catch (e) { alert("Ошибка массового удаления") }
}
</script>

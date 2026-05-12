<template>
  <div class="card shadow-sm fade-in">
      <div class="card-header bg-info text-dark d-flex justify-content-between align-items-center">
          <span class="fw-bold">Активные устройства в сети (DHCP Leases)</span>
          
          <!-- Переключатель "Скрыть известные" -->
          <div class="form-check form-switch mb-0">
              <input class="form-check-input" type="checkbox" id="hideSwitch" v-model="showOnlyNewLeases">
              <label class="form-check-label small fw-bold" for="hideSwitch">Скрыть известные в Статике</label>
          </div>
      </div>

      <table class="table table-hover mb-0 align-middle">
          <thead class="table-light">
              <tr>
                  <th style="width: 50px;" class="text-center">ARP</th>
                  <th>IP Адрес</th>
                  <th>MAC Адрес</th>
                  <th>Имя хоста (Hostname)</th>
                  <th class="text-end">Действие</th>
              </tr>
          </thead>
          <tbody>
              <tr v-for="l in filteredLeases" :key="l.ip">
                  <!-- Индикатор онлайн (зеленая точка) -->
                  <td class="text-center">
                      <span v-if="store.arpTable[l.mac.toLowerCase()]" title="В сети" class="text-success">🟢</span>
                      <span v-else class="text-muted" style="opacity: 0.3;" title="Офлайн (Только запись об аренде)">🔴</span>
                  </td>
                  
                  <td class="fw-bold text-dark">{{ l.ip }}</td>
                  <td class="font-monospace">{{ l.mac }}</td>
                  
                  <td>
                      <span v-if="l.hostname !== '*'" class="badge bg-secondary">{{ l.hostname }}</span>
                      <span v-else class="text-muted fst-italic small">Неизвестно</span>
                  </td>
                  
                  <!-- Кнопка "В статику" -->
                  <td class="text-end">
                      <button @click="copyToStatic(l)" class="btn btn-sm btn-outline-primary fw-bold" title="Сделать адрес статическим">
                          ➕ В статику
                      </button>
                  </td>
              </tr>
              
              <!-- Заглушка, если список пуст -->
              <tr v-if="filteredLeases.length === 0">
                  <td colspan="5" class="text-center p-5 text-muted">
                      <div v-if="store.searchQuery">
                          По запросу <strong>"{{ store.searchQuery }}"</strong> ничего не найдено.
                      </div>
                      <div v-else-if="showOnlyNewLeases">
                          🎉 Все подключенные устройства уже занесены в Статику!
                      </div>
                      <div v-else>
                          Нет активных DHCP аренд.
                      </div>
                  </td>
              </tr>
          </tbody>
      </table>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { store } from '../../store.js'

// Локальное состояние для тумблера
const showOnlyNewLeases = ref(false)

// Функция передачи данных в форму статики
function copyToStatic(lease) {
    // Кладем данные в глобальный store
    store.transferData = {
        mac: lease.mac,
        ip: lease.ip,
        hostname: lease.hostname !== '*' ? lease.hostname : ''
    }
    // Переключаем вкладку в App.vue
    store.tab = 'static'
}

// Фильтрация и сортировка данных из глобального Store
const filteredLeases = computed(() => {
    let data = [...store.leases]
    
    // 1. Фильтр "Скрыть известные"
    if (showOnlyNewLeases.value) {
        // Собираем все MAC адреса из статики (в нижнем регистре для точности)
        const knownMacs = new Set(store.hosts.map(h => h.mac.toLowerCase()))
        data = data.filter(lease => !knownMacs.has(lease.mac.toLowerCase()))
    }
    
    // 2. Фильтр "Живой поиск" из шапки
    if (store.searchQuery) {
        const q = store.searchQuery.toLowerCase()
        data = data.filter(l => 
            (l.mac && l.mac.toLowerCase().includes(q)) || 
            (l.ip && l.ip.toLowerCase().includes(q)) || 
            (l.hostname && l.hostname.toLowerCase().includes(q))
        )
    }
    
    // 3. Сортировка по IP адресу по возрастанию
    return data.sort((a, b) => {
        let valA = a['ip'] || ''; 
        let valB = b['ip'] || '';
        
        const numA = (valA.split('.') || []).map(Number); 
        const numB = (valB.split('.') || []).map(Number);
        
        if (numA.length !== 4 || numB.length !== 4) return 0;
        
        for (let i = 0; i < 4; i++) { 
            if (numA[i] !== numB[i]) return numA[i] - numB[i]; 
        }
        return 0;
    })
})
</script>

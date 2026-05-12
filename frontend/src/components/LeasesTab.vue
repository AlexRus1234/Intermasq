<template>
  <div class="card shadow-sm">
      <div class="card-header bg-info text-dark d-flex justify-content-between align-items-center">
          <span>Активные устройства в сети</span>
          <div class="form-check form-switch mb-0">
              <input class="form-check-input" type="checkbox" id="hideSwitch" v-model="showOnlyNewLeases">
              <label class="form-check-label" for="hideSwitch">Скрыть известные</label>
          </div>
      </div>
      <table class="table table-hover mb-0 align-middle">
          <thead><tr>
              <th style="width: 40px;">Онлайн</th>
              <th>IP</th><th>MAC</th><th>Hostname</th><th>Действие</th>
          </tr></thead>
          <tbody>
              <tr v-for="l in filteredLeases" :key="l.ip">
                  <!-- ИНДИКАТОР -->
                  <td class="text-center">
                      <span v-if="arp[l.mac.toLowerCase()]" title="В сети" class="text-success">🟢</span>
                      <span v-else title="Офлайн" class="text-muted" style="opacity: 0.3;">🔴</span>
                  </td>
                  <td class="fw-bold">{{ l.ip }}</td>
                  <td class="font-monospace">{{ l.mac }}</td>
                  <td>{{ l.hostname || '*' }}</td>
                  <td><button @click="$emit('copy-to-static', l)" class="btn btn-sm btn-outline-primary">➕ В статику</button></td>
              </tr>
              <tr v-if="filteredLeases.length === 0">
                  <td colspan="5" class="text-center p-4 text-muted">{{ searchQuery ? 'Ничего не найдено' : (showOnlyNewLeases ? 'Все устройства уже в статике' : 'Нет данных') }}</td>
              </tr>
          </tbody>
      </table>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
const props = defineProps(['leases', 'hosts', 'searchQuery', 'arp'])
const emit = defineEmits(['copy-to-static'])
const showOnlyNewLeases = ref(false)

const filteredLeases = computed(() => {
    let data = [...props.leases]
    if (showOnlyNewLeases.value) {
        const knownMacs = new Set(props.hosts.map(h => h.mac.toLowerCase()))
        data = data.filter(lease => !knownMacs.has(lease.mac.toLowerCase()))
    }
    if (props.searchQuery) {
        const q = props.searchQuery.toLowerCase()
        data = data.filter(l => (l.mac && l.mac.toLowerCase().includes(q)) || (l.ip && l.ip.toLowerCase().includes(q)) || (l.hostname && l.hostname.toLowerCase().includes(q)))
    }
    return data.sort((a, b) => {
        let valA = a['ip'] || ''; let valB = b['ip'] || '';
        const numA = (valA.split('.') || []).map(Number); const numB = (valB.split('.') || []).map(Number);
        if(numA.length!==4 || numB.length!==4) return 0;
        for(let i=0; i<4; i++) { if (numA[i] !== numB[i]) return numA[i] - numB[i]; }
        return 0;
    })
})
</script>

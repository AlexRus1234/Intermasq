<template>
  <div class="card shadow-sm fade-in">
      <div class="card-header bg-info text-dark d-flex justify-content-between align-items-center">
          <span class="fw-bold">{{ $t('leases.title') }}</span>
          
          <div class="d-flex gap-3 align-items-center">
              <div class="form-check form-switch mb-0">
                  <input class="form-check-input" type="checkbox" id="hideSwitch" v-model="showOnlyNewLeases">
                  <label class="form-check-label small fw-bold" for="hideSwitch">{{ $t('leases.hideKnown') }}</label>
              </div>
              <button v-if="selectedLeases.length > 0" @click="bulkAddToStatic" class="btn btn-sm btn-primary fw-bold">
                  ➕ {{ $t('leases.bulkToStatic', { n: selectedLeases.length }) }}
              </button>
          </div>
      </div>

      <div v-if="selectedLeases.length > 0" class="alert alert-warning d-flex align-items-center gap-2 mb-0 rounded-0 small">
          <span>⚠️</span>
          <span>{{ $t('leases.toStaticHint', 'Bulk "add to static" writes raw dhcp-host lines without dnsmasq --test validation. Click "Apply" afterwards to activate.') }}</span>
      </div>

      <table class="table table-hover mb-0 align-middle">
          <thead class="table-light">
              <tr>
                  <th style="width: 40px;" class="text-center">
                      <input type="checkbox" @change="toggleAll($event)">
                  </th>
                  <th style="width: 50px;" class="text-center">ARP</th>
                  <th>{{ $t('leases.ipAddress') }}</th>
                  <th>{{ $t('leases.macAddress') }}</th>
                  <th>{{ $t('leases.hostname') }}</th>
                  <th class="text-end">{{ $t('leases.action') }}</th>
              </tr>
          </thead>
          <tbody>
              <tr v-for="l in filteredLeases" :key="l.ip" :class="{ 'table-active': selectedLeases.includes(l.mac) }">
                  <td class="text-center">
                      <input type="checkbox" :checked="selectedLeases.includes(l.mac)" @change="toggleLease(l.mac)" :disabled="knownMacs.has(l.mac.toLowerCase())">
                  </td>
                  <td class="text-center">
                      <span v-if="store.arpTable[l.mac.toLowerCase()]" :title="$t('leases.onlineTooltip')" class="text-success">🟢</span>
                      <span v-else :title="$t('leases.offlineTooltip')" class="text-muted" style="opacity: 0.3;">🔴</span>
                  </td>
                  
                  <td class="fw-bold text-dark">{{ l.ip }}</td>
                  <td class="font-monospace">{{ l.mac }}</td>
                  
                  <td>
                      <span v-if="l.hostname !== '*'" class="badge bg-secondary">{{ l.hostname }}</span>
                      <span v-else class="text-muted fst-italic small">{{ $t('leases.unknown') }}</span>
                  </td>
                  
                  <td class="text-end">
                      <span v-if="knownMacs.has(l.mac.toLowerCase())" class="badge bg-success">✓ {{ $t('leases.inStatic') }}</span>
                      <button v-else @click="copyToStatic(l)" class="btn btn-sm btn-outline-primary fw-bold" :title="$t('leases.toStaticTooltip')">
                          ➕ {{ $t('leases.toStatic') }}
                      </button>
                  </td>
              </tr>
              
              <tr v-if="filteredLeases.length === 0">
                  <td colspan="6" class="text-center p-5 text-muted">
                      <div v-if="store.searchQuery">
                          {{ $t('leases.searchEmpty', { query: store.searchQuery }) }}
                      </div>
                      <div v-else-if="showOnlyNewLeases">
                          🎉 {{ $t('leases.allInStatic') }}
                      </div>
                      <div v-else>
                          {{ $t('leases.noLeases') }}
                      </div>
                  </td>
              </tr>
          </tbody>
      </table>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { store, actions } from '../../store.js'
import { toast } from '../../toast.js'

const showOnlyNewLeases = ref(false)
const selectedLeases = ref([])

const knownMacs = computed(() => new Set(store.hosts.map(h => h.mac.toLowerCase())))

function copyToStatic(lease) {
    store.transferData = {
        mac: lease.mac,
        ip: lease.ip,
        hostname: lease.hostname !== '*' ? lease.hostname : ''
    }
    store.tab = 'static'
}

function toggleLease(mac) {
    const idx = selectedLeases.value.indexOf(mac)
    if (idx >= 0) selectedLeases.value.splice(idx, 1)
    else selectedLeases.value.push(mac)
}

function toggleAll(event) {
    if (event.target.checked) {
        selectedLeases.value = filteredLeases.value
            .filter(l => !knownMacs.value.has(l.mac.toLowerCase()))
            .map(l => l.mac)
    } else {
        selectedLeases.value = []
    }
}

async function bulkAddToStatic() {
    const macs = new Set(selectedLeases.value.map(m => m.toLowerCase()))
    const leases = store.leases.filter(l => macs.has(l.mac.toLowerCase()))
    const file = store.hosts.length > 0 ? (store.hosts[0].file.includes('|') ? store.hosts[0].file.split('|')[0] : store.hosts[0].file) : null
    if (!file) { toast.error('No target file found'); return }
    const ok = await actions.bulkLeaseToStatic(leases, file)
    if (ok) selectedLeases.value = []
}

const filteredLeases = computed(() => {
    let data = [...store.leases]
    
    if (showOnlyNewLeases.value) {
        const knownMacs = new Set(store.hosts.map(h => h.mac.toLowerCase()))
        data = data.filter(lease => !knownMacs.has(lease.mac.toLowerCase()))
    }
    
    if (store.searchQuery) {
        const q = store.searchQuery.toLowerCase()
        data = data.filter(l => 
            (l.mac && l.mac.toLowerCase().includes(q)) || 
            (l.ip && l.ip.toLowerCase().includes(q)) || 
            (l.hostname && l.hostname.toLowerCase().includes(q))
        )
    }
    
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

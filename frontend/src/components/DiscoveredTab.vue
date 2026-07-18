<template>
  <div class="fade-in">
    <!-- Section 1: Unknown ARP devices (online but neither in static nor in leases) -->
    <div class="card shadow-sm mb-3 border-warning">
      <div class="card-header bg-warning text-dark d-flex justify-content-between align-items-center">
          <span class="fw-bold">🔍 {{ $t('discovery.unknownArp', 'Unknown devices (ARP, not configured)') }} · {{ store.newDevices.length }}</span>
          <button @click="actions.loadNewDevices()" class="btn btn-sm btn-dark">🔄 {{ $t('newDevices.refresh') }}</button>
      </div>
      <div class="card-body p-0">
          <table class="table table-hover mb-0 align-middle" v-if="store.newDevices.length > 0">
              <thead class="table-light">
                  <tr>
                      <th>{{ $t('newDevices.mac') }}</th>
                      <th>{{ $t('newDevices.vendor') }}</th>
                      <th class="text-end">{{ $t('newDevices.action') }}</th>
                  </tr>
              </thead>
              <tbody>
                  <tr v-for="d in store.newDevices" :key="d.mac">
                      <td class="font-monospace fw-bold">{{ d.mac }}</td>
                      <td>
                          <span v-if="d.vendor" class="badge bg-info text-dark">{{ d.vendor }}</span>
                          <span v-else class="text-muted">--</span>
                      </td>
                      <td class="text-end">
                          <button @click="addToStatic(d)" class="btn btn-sm btn-outline-primary fw-bold">
                              ➕ {{ $t('newDevices.add') }}
                          </button>
                      </td>
                  </tr>
              </tbody>
          </table>
          <div v-else class="text-center p-4 text-muted small">
              {{ $t('newDevices.empty') }}
          </div>
      </div>
    </div>

    <!-- Section 2: New DHCP leases (not yet in static) -->
    <div class="card shadow-sm border-info">
      <div class="card-header bg-info text-dark d-flex justify-content-between align-items-center">
          <span class="fw-bold">📡 {{ $t('discovery.newLeases', 'New DHCP leases') }} · {{ newLeases.length }}</span>
          <div class="d-flex gap-2 align-items-center">
              <button v-if="selectedLeases.length > 0" @click="bulkAddToStatic" class="btn btn-sm btn-primary fw-bold">
                  ➕ {{ $t('leases.bulkToStatic', { n: selectedLeases.length }) }}
              </button>
              <button @click="actions.loadData()" class="btn btn-sm btn-dark">🔄 {{ $t('newDevices.refresh') }}</button>
          </div>
      </div>
      <div class="card-body p-0">
          <table class="table table-hover mb-0 align-middle" v-if="newLeases.length > 0">
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
                  <tr v-for="l in filteredNewLeases" :key="l.mac">
                      <td class="text-center">
                          <input type="checkbox" :checked="selectedLeases.includes(l.mac)" @change="toggleLease(l.mac)">
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
                          <button @click="copyToStatic(l)" class="btn btn-sm btn-outline-primary fw-bold" :title="$t('leases.toStaticTooltip')">
                              ➕ {{ $t('leases.toStatic') }}
                          </button>
                      </td>
                  </tr>
                  <tr v-if="filteredNewLeases.length === 0">
                      <td colspan="6" class="text-center p-4 text-muted small">
                          <div v-if="store.searchQuery">{{ $t('leases.searchEmpty', { query: store.searchQuery }) }}</div>
                          <div v-else>🎉 {{ $t('leases.allInStatic') }}</div>
                      </td>
                  </tr>
              </tbody>
          </table>
          <div v-else class="text-center p-4 text-muted small">
              🎉 {{ $t('leases.allInStatic') }}
          </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { store, actions } from '../store.js'

const selectedLeases = ref([])

const knownMacs = computed(() => new Set(store.hosts.map(h => h.mac.toLowerCase())))

// Leases whose MAC is NOT already in any static config — these are the only
// ones eligible for the "Add to static" action, so the second section is the
// spiritual successor to the old Leases tab's "Hide known" switch.
const newLeases = computed(() =>
    store.leases.filter(l => !knownMacs.value.has(l.mac.toLowerCase()))
)

const filteredNewLeases = computed(() => {
    let data = newLeases.value
    if (store.searchQuery) {
        const q = store.searchQuery.toLowerCase()
        data = data.filter(l =>
            (l.mac && l.mac.toLowerCase().includes(q)) ||
            (l.ip && l.ip.toLowerCase().includes(q)) ||
            (l.hostname && l.hostname.toLowerCase().includes(q))
        )
    }
    return data
})

function addToStatic(device) {
    store.transferData = { mac: device.mac, ip: '', hostname: '', tags: [] }
    store.tab = 'static'
}

function copyToStatic(lease) {
    store.transferData = {
        mac: lease.mac,
        ip: lease.ip,
        hostname: lease.hostname !== '*' ? lease.hostname : '',
        tags: []
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
        selectedLeases.value = filteredNewLeases.value.map(l => l.mac)
    } else {
        selectedLeases.value = []
    }
}

async function bulkAddToStatic() {
    const macs = new Set(selectedLeases.value.map(m => m.toLowerCase()))
    const leases = store.leases.filter(l => macs.has(l.mac.toLowerCase()))
    const file = store.hosts.length > 0
        ? (store.hosts[0].file.includes('|') ? store.hosts[0].file.split('|')[0] : store.hosts[0].file)
        : null
    if (!file) { alert('No target file found'); return }
    const ok = await actions.bulkLeaseToStatic(leases, file)
    if (ok) selectedLeases.value = []
}
</script>

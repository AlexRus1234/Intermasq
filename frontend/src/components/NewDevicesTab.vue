<template>
  <div class="card shadow-sm fade-in">
      <div class="card-header bg-secondary text-white d-flex justify-content-between align-items-center">
          <span class="fw-bold">{{ $t('newDevices.title') }}</span>
          <button @click="actions.loadNewDevices()" class="btn btn-sm btn-light">{{ $t('newDevices.refresh') }}</button>
      </div>

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

      <div v-else class="text-center p-5 text-muted">
          {{ $t('newDevices.empty') }}
      </div>
  </div>
</template>

<script setup>
import { store, actions } from '../store.js'

function addToStatic(device) {
    store.transferData = {
        mac: device.mac,
        ip: '',
        hostname: ''
    }
    store.tab = 'static'
}
</script>

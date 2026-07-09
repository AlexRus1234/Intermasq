<template>
  <div class="card shadow-sm fade-in">
      <div class="card-header bg-dark text-white">
          <span class="fw-bold">{{ $t('audit.title') }}</span>
      </div>

      <div v-if="store.auditLog.length === 0" class="text-center p-5 text-muted">
          {{ $t('audit.empty') }}
      </div>

      <div class="table-responsive" v-if="store.auditLog.length > 0">
          <table class="table table-hover mb-0 align-middle">
              <thead class="table-light">
                  <tr>
                      <th>{{ $t('audit.time') }}</th>
                      <th>{{ $t('audit.user') }}</th>
                      <th>{{ $t('audit.action') }}</th>
                      <th>{{ $t('audit.mac') }}</th>
                      <th>{{ $t('audit.hostname') }}</th>
                      <th>{{ $t('audit.ip') }}</th>
                      <th>{{ $t('audit.file') }}</th>
                  </tr>
              </thead>
              <tbody>
                  <tr v-for="(entry, idx) in store.auditLog" :key="idx">
                      <td class="small text-muted">{{ formatTime(entry.timestamp) }}</td>
                      <td>{{ entry.user }}</td>
                      <td>
                          <span :class="actionClass(entry.action)" class="badge">{{ entry.action }}</span>
                      </td>
                      <td class="font-monospace small">{{ entry.mac }}</td>
                      <td>{{ entry.hostname }}</td>
                      <td>{{ entry.ip }}</td>
                      <td class="small text-muted">{{ entry.file ? entry.file.split('/').pop() : '' }}</td>
                  </tr>
              </tbody>
          </table>
      </div>
  </div>
</template>

<script setup>
import { store } from '../../store.js'

function formatTime(ts) {
    if (!ts) return ''
    try { return new Date(ts).toLocaleString() } catch { return ts }
}

function actionClass(action) {
    switch (action) {
        case 'add': case 'bulk_add': return 'bg-success'
        case 'delete': case 'bulk_delete': return 'bg-danger'
        case 'rollback': return 'bg-warning text-dark'
        case 'reload': return 'bg-info'
        default: return 'bg-secondary'
    }
}
</script>

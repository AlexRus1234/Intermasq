<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

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
                           <span :class="actionClass(entry.action)" class="badge">{{ actionLabel(entry.action) }}</span>
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
import { useI18n } from 'vue-i18n'

const { t, te } = useI18n()

function formatTime(ts) {
    if (!ts) return ''
    try { return new Date(ts).toLocaleString() } catch { return ts }
}

function actionLabel(action) {
    const key = 'audit.action_' + action
    return te(key) ? t(key) : action
}

function actionClass(action) {
    switch (action) {
        case 'add': case 'bulk_add': return 'bg-success'
        case 'delete': case 'bulk_delete': case 'config_delete_file': return 'bg-danger'
        case 'rollback': case 'restore': return 'bg-warning text-dark'
        case 'reload': return 'bg-info'
        case 'config_update': case 'config_create_file': case 'config_write_raw': return 'bg-primary'
        case 'backup_restore': return 'bg-info'
        case 'user_create': case 'user_delete': case 'password_change': return 'bg-secondary'
        default: return 'bg-secondary'
    }
}
</script>

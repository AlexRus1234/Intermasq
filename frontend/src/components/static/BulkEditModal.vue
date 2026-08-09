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
  <div v-if="show" class="modal-backdrop fade-in" @click.self="$emit('close')">
    <div class="modal-dialog modal-lg modal-dialog-centered d-block">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">✏️ {{ $t('hosts.bulkEditTitle', 'Edit hosts') }}</h5>
          <button type="button" class="btn-close" @click="$emit('close')"></button>
        </div>
        <div class="modal-body">
          <p class="text-muted small">{{ $t('hosts.bulkEditDesc', { count: hosts.length }) }}</p>

          <h6>IP {{ $t('hosts.transform', 'transform') }}</h6>
          <div class="row g-2 mb-3">
            <div class="col-md-6">
              <input v-model="ipForm.old_prefix" :placeholder="$t('hosts.oldPrefixPh', '10.0.0  или  10.0.0.0/24')" class="form-control">
            </div>
            <div class="col-md-6">
              <input v-model="ipForm.new_prefix" :placeholder="$t('hosts.newPrefixPh', '10.0.1  или  10.0.1.0/24')" class="form-control">
            </div>
          </div>

          <h6>Hostname {{ $t('hosts.transform', 'transform') }}</h6>
          <div class="row g-2 mb-3">
            <div class="col-md-6">
              <input v-model="hostForm.strip_old" :placeholder="$t('hosts.stripSuffixPh', '-old')" class="form-control">
            </div>
            <div class="col-md-6">
              <input v-model="hostForm.suffix" :placeholder="$t('hosts.addSuffixPh', '-new')" class="form-control">
            </div>
          </div>

          <div v-if="preview.length > 0" class="mt-3">
            <h6>{{ $t('hosts.preview', 'Preview') }}</h6>
            <table class="table table-sm table-bordered">
              <thead><tr><th>MAC</th><th>IP →</th><th>Hostname →</th></tr></thead>
              <tbody>
                <tr v-for="p in preview" :key="p.mac">
                  <td class="font-monospace small">{{ p.mac }}</td>
                  <td class="small">{{ p.oldIp }} → <strong>{{ p.newIp || '—' }}</strong></td>
                  <td class="small">{{ p.oldHost }} → <strong>{{ p.newHost || '—' }}</strong></td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="modal-footer">
          <button @click="$emit('close')" class="btn btn-outline-secondary">{{ $t('hosts.cancel', 'Cancel') }}</button>
          <button @click="submit" class="btn btn-warning" :disabled="!canSubmit">{{ $t('hosts.apply', 'Apply') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { actions } from '../../store.js'

const props = defineProps(['show', 'hosts'])
const emit = defineEmits(['close', 'done'])

const ipForm = ref({ old_prefix: '', new_prefix: '' })
const hostForm = ref({ strip_old: '', suffix: '' })

const preview = computed(() => {
  return props.hosts.slice(0, 5).map(h => {
    const host = store_hosts.hosts.find(x => x.mac === h.mac)
    const oldIp = host?.ip || ''
    const oldHost = host?.hostname || ''
    let newIp = oldIp
    if (ipForm.value.old_prefix && ipForm.value.new_prefix) {
      if (ipForm.value.old_prefix.includes('/')) {
        // CIDR — не считаем на клиенте, оставляем заглушку
        newIp = '(computed)'
      } else if (oldIp.startsWith(ipForm.value.old_prefix)) {
        newIp = ipForm.value.new_prefix + oldIp.slice(ipForm.value.old_prefix.length)
      }
    }
    let newHost = oldHost
    if (hostForm.value.strip_old) newHost = newHost.replace(new RegExp(hostForm.value.strip_old + '$'), '')
    if (hostForm.value.suffix) newHost = newHost + hostForm.value.suffix
    return { mac: h.mac, oldIp, newIp, oldHost, newHost }
  })
})

import { store as store_hosts } from '../../store.js'

const canSubmit = computed(() =>
  (ipForm.value.old_prefix && ipForm.value.new_prefix) ||
  hostForm.value.strip_old ||
  hostForm.value.suffix
)

async function submit() {
  const result = await actions.bulkEdit(props.hosts, ipForm.value, hostForm.value)
  if (result) emit('done')
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(0,0,0,0.5);
  z-index: 1050;
  padding: 1rem;
}
</style>

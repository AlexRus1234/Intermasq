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
          <h5 class="modal-title">⚙️ {{ $t('templates.title', 'Templates') }}</h5>
          <button type="button" class="btn-close" @click="$emit('close')"></button>
        </div>
        <div class="modal-body">
          <div v-if="store.templates.length === 0" class="text-muted text-center p-3">
            {{ $t('templates.empty', 'No templates yet') }}
          </div>
          <ul v-else class="list-group mb-3">
            <li v-for="t in store.templates" :key="t.id" class="list-group-item d-flex justify-content-between align-items-center">
              <div>
                <strong>{{ t.name }}</strong>
                <div class="small text-muted">
                  {{ t.ip_range }} · {{ t.hostname_pattern }} · {{ t.target_file.split('/').pop() }}
                </div>
              </div>
              <button @click="remove(t.id)" class="btn btn-sm btn-outline-danger">✕</button>
            </li>
          </ul>

          <hr>
          <h6>{{ $t('templates.new', 'New template') }}</h6>
          <div class="row g-2 mt-1">
            <div class="col-md-6"><input v-model="form.name" data-testid="tpl-name" :placeholder="$t('templates.namePlaceholder', 'Name')" class="form-control"></div>
            <div class="col-md-6">
              <select v-if="store.dhcpRanges.length > 0" v-model="form.ip_range" class="form-select">
                <option value="">— {{ $t('templates.manualRange', 'manual CIDR') }} —</option>
                <option v-for="r in store.dhcpRanges" :key="r" :value="r">{{ r }}</option>
              </select>
              <input v-else v-model="form.ip_range" data-testid="tpl-ip-range" placeholder="10.0.0.0/24" class="form-control">
              <input v-if="store.dhcpRanges.length > 0 && form.ip_range && !store.dhcpRanges.includes(form.ip_range)" v-model="form.ip_range" placeholder="10.0.0.0/24" class="form-control form-control-sm mt-1">
            </div>
            <div class="col-md-6"><input v-model="form.hostname_pattern" data-testid="tpl-hostname-pattern" placeholder="device-{NNN}" class="form-control"></div>
            <div class="col-md-6"><input v-model="form.target_file" data-testid="tpl-target-file" :placeholder="$t('templates.filePlaceholder', '/etc/dnsmasq.d/hosts.conf')" class="form-control"></div>
          </div>
        </div>
        <div class="modal-footer">
          <button @click="$emit('close')" class="btn btn-outline-secondary">{{ $t('hosts.cancel', 'Cancel') }}</button>
          <button @click="create" class="btn btn-success" :disabled="!canCreate">{{ $t('templates.create', 'Create') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'

const { t } = useI18n()
defineProps(['show'])
const emit = defineEmits(['close'])

const form = ref({ name: '', ip_range: '', hostname_pattern: '', target_file: '' })

const canCreate = computed(() =>
  form.value.name && form.value.ip_range && form.value.hostname_pattern && form.value.target_file
)

async function create() {
  const ok = await actions.createTemplate({ ...form.value })
  if (ok) {
    form.value = { name: '', ip_range: '', hostname_pattern: '', target_file: '' }
  }
}

async function remove(id) {
  if (!confirm(t('confirm.deleteTemplate', 'Delete template?'))) return
  await actions.deleteTemplate(id)
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

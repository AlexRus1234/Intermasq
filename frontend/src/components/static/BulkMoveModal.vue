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
    <div class="modal-dialog modal-dialog-centered d-block">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">📦 {{ $t('hosts.bulkMoveTitle', 'Move hosts') }}</h5>
          <button type="button" class="btn-close" @click="$emit('close')"></button>
        </div>
        <div class="modal-body">
          <p class="text-muted small">{{ $t('hosts.bulkMoveDesc', { count: hosts.length }) }}</p>
          <label class="form-label">{{ $t('hosts.targetFile', 'Target file') }}</label>
          <select v-model="target" class="form-select mb-2">
            <option v-for="f in uniqueFiles" :key="f" :value="f">{{ f.split('/').pop() }}</option>
          </select>
          <input v-model="customTarget" :placeholder="$t('hosts.customFilePlaceholder', '/etc/dnsmasq.d/new.conf')" class="form-control">
        </div>
        <div class="modal-footer">
          <button @click="$emit('close')" class="btn btn-outline-secondary">{{ $t('hosts.cancel', 'Cancel') }}</button>
          <button @click="submit" class="btn btn-primary" :disabled="!resolvedTarget">{{ $t('hosts.move', 'Move') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { store, actions } from '../../store.js'

const props = defineProps(['show', 'hosts'])
const emit = defineEmits(['close', 'done'])

const target = ref('')
const customTarget = ref('')

const uniqueFiles = computed(() => Array.from(new Set(store.hosts.map(h => (h.file || '').split('|')[0]))).sort())

const resolvedTarget = computed(() => customTarget.value.trim() || target.value)

async function submit() {
  const t = resolvedTarget.value
  if (!t) return
  const result = await actions.bulkMove(props.hosts, t)
  if (result) {
    emit('done')
  }
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

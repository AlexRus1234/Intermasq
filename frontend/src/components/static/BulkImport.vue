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
    <div class="row g-2 fade-in">
        <div class="col-12">
            <textarea v-model="text" class="form-control text-monospace" rows="4" :placeholder="$t('hosts.bulkPlaceholder')"></textarea>
        </div>
        <div class="col-12 d-flex justify-content-between align-items-center">
            <span class="text-muted small">{{ $t('hosts.parsed') }} {{ parsedCount }}</span>
            <div class="input-group" style="width: auto;">
                <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" :placeholder="$t('hosts.destFile')">
                <button @click="$emit('import', text)" class="btn btn-success" :disabled="parsedCount === 0">{{ $t('hosts.importBtn') }}</button>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, computed } from 'vue'
const props = defineProps(['form', 'selectedFile'])
defineEmits(['import'])

const text = ref('')
const parsedCount = computed(() => {
    return text.value.split('\n').filter(line => /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})/i.test(line.trim())).length
})
</script>

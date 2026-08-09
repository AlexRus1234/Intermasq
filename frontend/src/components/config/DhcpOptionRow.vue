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
  <div class="border rounded p-2 mb-1 bg-light-subtle">
    <div class="row g-2 align-items-center">
      <div class="col-md-2">
        <input v-model="fields.tag" @input="emit" class="form-control form-control-sm font-monospace" :placeholder="$t('config.optionTag')" :disabled="!directive.active">
      </div>
      <div class="col-md-3">
        <select v-model="fields.optionKey" @change="onPresetChange" class="form-select form-select-sm" :disabled="!directive.active">
          <option v-for="p in presets" :key="p.key" :value="p.key">{{ p.label }} ({{ p.key }})</option>
          <option value="__custom">{{ $t('config.optionCustom') }}</option>
        </select>
      </div>
      <div v-if="fields.optionKey === '__custom'" class="col-md-2">
        <input v-model="fields.customOption" @input="emit" class="form-control form-control-sm font-monospace" placeholder="option:name" :disabled="!directive.active">
      </div>
      <div :class="fields.optionKey === '__custom' ? 'col-md-3' : 'col-md-5'">
        <input v-model="fields.value" @input="emit" class="form-control form-control-sm" :placeholder="currentPresetHint" :disabled="!directive.active">
      </div>
      <div class="col-auto">
        <div class="form-check form-switch">
          <input class="form-check-input" type="checkbox" v-model="directive.active">
        </div>
      </div>
      <div class="col-auto">
        <button @click="$emit('remove')" class="btn btn-sm btn-outline-danger">🗑</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { reactive, computed, watch } from 'vue'
import { DHCP_OPTION_PRESETS } from './directives.js'

const props = defineProps({ directive: Object })
defineEmits(['remove'])

const presets = DHCP_OPTION_PRESETS

const fields = reactive({ tag: '', optionKey: 'option:router', customOption: '', value: '' })

const currentPresetHint = computed(() => {
  if (fields.optionKey === '__custom') return ''
  const p = presets.find(x => x.key === fields.optionKey)
  return p ? p.valueHint : ''
})

function effectiveOption() {
  if (fields.optionKey === '__custom') return fields.customOption.trim()
  return fields.optionKey
}

// Parse a raw dhcp-option value into structured fields.
// Accepted forms:
//   option:router,192.168.1.1
//   tag:iot,option:dns-server,1.1.1.1
//   6,8.8.8.8                  (numeric — mapped to named preset if known)
//   vendor-class,"MSFT 5.0"    (quoted value with commas is out of scope)
function parseInto(target, raw) {
  target.tag = ''
  target.optionKey = 'option:router'
  target.customOption = ''
  target.value = ''
  const parts = (raw || '').split(',').map(s => s.trim())
  if (parts.length === 0) return
  let rest = parts
  if (rest[0] && (rest[0].startsWith('set:') || rest[0].startsWith('tag:'))) {
    target.tag = rest[0]
    rest = rest.slice(1)
  }
  if (rest.length === 0) return
  const opt = rest[0]
  rest = rest.slice(1)
  // Map numeric option to a named preset if we know it, otherwise keep raw.
  const byNumber = presets.find(p => p.number === opt)
  if (byNumber) {
    target.optionKey = byNumber.key
  } else if (presets.some(p => p.key === opt) || opt.startsWith('option:') || opt.startsWith('vendor:') || opt.startsWith('encap:')) {
    if (presets.some(p => p.key === opt)) {
      target.optionKey = opt
    } else {
      target.optionKey = '__custom'
      target.customOption = opt
    }
  } else if (/^\d+$/.test(opt)) {
    target.optionKey = '__custom'
    target.customOption = opt
  } else {
    // Unknown textual form like "router" without prefix — treat as custom.
    target.optionKey = '__custom'
    target.customOption = opt
  }
  target.value = rest.join(',')
}

function onPresetChange() {
  // When switching back to a known preset, clear the custom field so it does
  // not leak into the emitted value.
  if (fields.optionKey !== '__custom') fields.customOption = ''
  emit()
}

function emit() {
  const parts = []
  if (fields.tag) parts.push(fields.tag)
  const opt = effectiveOption()
  if (opt) parts.push(opt)
  if (fields.value) parts.push(fields.value)
  props.directive.value = parts.join(',')
}

watch(() => props.directive.value, (v) => {
  const probe = {}
  parseInto(probe, v)
  // Avoid clobbering the editor while the user is typing: only re-parse if
  // the round-tripped value actually differs.
  if (probe.tag !== fields.tag || probe.optionKey !== fields.optionKey ||
      probe.customOption !== fields.customOption || probe.value !== fields.value) {
    parseInto(fields, v)
  }
}, { immediate: true })
</script>

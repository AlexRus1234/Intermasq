<template>
  <div class="border rounded p-2 mb-1 bg-light-subtle">
    <div class="row g-2 align-items-center">
      <div class="col">
        <input v-model="fields.tag" @input="emit" class="form-control form-control-sm" :placeholder="$t('config.rangeTag')" :disabled="!directive.active">
      </div>
      <div class="col">
        <input v-model="fields.start" @input="emit" class="form-control form-control-sm" :placeholder="$t('config.rangeStart')" :disabled="!directive.active">
      </div>
      <div class="col">
        <input v-model="fields.end" @input="emit" class="form-control form-control-sm" :placeholder="$t('config.rangeEnd')" :disabled="!directive.active">
      </div>
      <div class="col">
        <input v-model="fields.mask" @input="emit" class="form-control form-control-sm" :placeholder="$t('config.rangeMask')" :disabled="!directive.active">
      </div>
      <div class="col">
        <input v-model="fields.lease" @input="emit" class="form-control form-control-sm" :placeholder="$t('config.rangeLease')" :disabled="!directive.active">
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
import { reactive, watch } from 'vue'

const props = defineProps({ directive: Object })
defineEmits(['remove'])

const fields = reactive({ tag: '', start: '', end: '', mask: '', lease: '' })

function parseInto(target, value) {
  const parts = (value || '').split(',').map(s => s.trim())
  target.tag = ''
  target.start = ''
  target.end = ''
  target.mask = ''
  target.lease = ''
  let rest = parts
  if (rest[0] && (rest[0].startsWith('set:') || rest[0].startsWith('tag:'))) {
    target.tag = rest[0].replace(/^(set:|tag:)/, '')
    rest = rest.slice(1)
  }
  if (rest.length >= 1) target.start = rest[0]
  if (rest.length >= 2) target.end = rest[1]
  if (rest.length >= 3) {
    if (/^\d+[smhdw]?$/.test(rest[2]) || rest[2] === 'infinite') {
      target.lease = rest[2]
      if (rest.length >= 4) target.mask = rest[3]
    } else {
      target.mask = rest[2]
      if (rest.length >= 4) target.lease = rest[3]
    }
  }
}

function emit() {
  const parts = []
  if (fields.tag) parts.push('set:' + fields.tag)
  if (fields.start) parts.push(fields.start)
  if (fields.end) parts.push(fields.end)
  if (fields.mask) parts.push(fields.mask)
  if (fields.lease) parts.push(fields.lease)
  props.directive.value = parts.join(',')
}

watch(() => props.directive.value, (v) => {
  const probe = {}
  parseInto(probe, v)
  if (JSON.stringify(probe) !== JSON.stringify(fields)) {
    parseInto(fields, v)
  }
}, { immediate: true })
</script>

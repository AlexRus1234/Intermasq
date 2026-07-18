<template>
  <div class="row mb-2 align-items-center g-2">
    <div class="col-md-3 col-lg-2"><code>server</code></div>
    <div class="col-md-4 col-lg-3">
      <div class="input-group">
        <span class="input-group-text font-monospace small">/</span>
        <input v-model="fields.domain" @input="emit" class="form-control form-control-sm font-monospace" :placeholder="$t('config.forwardDomainHint')" :disabled="!directive.active">
        <span class="input-group-text font-monospace small">/</span>
      </div>
    </div>
    <div class="col">
      <input v-model="fields.upstream" @input="emit" class="form-control form-control-sm font-monospace" :placeholder="$t('config.forwardUpstreamHint')" :disabled="!directive.active">
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
</template>

<script setup>
import { reactive, watch } from 'vue'

const props = defineProps({ directive: Object })
defineEmits(['remove'])

const fields = reactive({ domain: '', upstream: '' })

// Parse a server= value into domain + upstream.
//   server=8.8.8.8                 -> domain='', upstream='8.8.8.8'        (global upstream)
//   server=/corp/10.0.0.53         -> domain='corp', upstream='10.0.0.53'  (per-domain)
//   server=/.corp/10.0.0.53        -> domain='.corp', upstream='10.0.0.53' (wildcard suffix)
//   server=/corp/#                 -> domain='corp', upstream='#'          (drop, do not forward)
function parseInto(target, raw) {
  target.domain = ''
  target.upstream = ''
  const v = (raw || '').trim()
  if (!v) return
  if (v.startsWith('/')) {
    const secondSlash = v.indexOf('/', 1)
    if (secondSlash > 0) {
      target.domain = v.slice(1, secondSlash)
      target.upstream = v.slice(secondSlash + 1).trim()
      return
    }
  }
  target.upstream = v
}

function emit() {
  if (fields.domain) {
    props.directive.value = `/${fields.domain}/${fields.upstream}`
  } else {
    props.directive.value = fields.upstream
  }
}

watch(() => props.directive.value, (v) => {
  const probe = {}
  parseInto(probe, v)
  if (probe.domain !== fields.domain || probe.upstream !== fields.upstream) {
    parseInto(fields, v)
  }
}, { immediate: true })
</script>

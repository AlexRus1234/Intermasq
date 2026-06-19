<template>
  <div :class="['card mb-4 p-3 shadow-sm', isEditing ? 'border-warning' : 'border-primary']">
      <div class="d-flex justify-content-between align-items-center mb-2">
          <h6 :class="isEditing ? 'text-warning text-dark' : 'text-primary'" class="mb-0 fw-bold">
              {{ isEditing ? '✏️ ' + $t('hosts.editing') : (selectedFile === 'all' ? '➕ ' + $t('hosts.newDevice') : '➕ ' + $t('hosts.addTo') + ' ' + selectedFile.split('/').pop()) }}
          </h6>
          
          <div v-if="!isEditing" class="form-check form-switch mb-0">
              <input class="form-check-input" type="checkbox" id="importMode" v-model="isImportMode">
              <label class="form-check-label small text-muted" for="importMode">{{ $t('hosts.importList') }}</label>
          </div>
          
          <button v-if="isEditing" @click="$emit('cancel-edit')" class="btn btn-sm btn-outline-secondary">✕ {{ $t('hosts.cancel') }}</button>
      </div>
      
      <div v-if="!isImportMode" class="row g-2">
        <div class="col-md-3"><input v-model="form.mac" :placeholder="'MAC (aa:bb...)'" class="form-control"></div>
        <div class="col-md-3"><input v-model="form.ip" placeholder="IP (172.20...)" class="form-control"></div>
        <div class="col-md-3"><input v-model="form.hostname" placeholder="Hostname" class="form-control"></div>
        <div class="col-md-3">
            <div class="input-group">
                <input v-model="form.file" :readonly="selectedFile !== 'all' && !isEditing" :class="['form-control', (selectedFile !== 'all' && !isEditing) ? 'bg-light' : '']" :placeholder="$t('hosts.filePlaceholder')">
                <button @click="saveHost" class="btn fw-bold" :class="isEditing ? 'btn-warning' : 'btn-success'">
                    {{ isEditing ? $t('hosts.save') : $t('hosts.add') }}
                </button>
            </div>
        </div>
      </div>

      <div v-if="isImportMode" class="row g-2 fade-in">
          <div class="col-12">
              <textarea v-model="bulkText" class="form-control font-monospace" rows="4" :placeholder="$t('hosts.bulkPlaceholder')"></textarea>
          </div>
          <div class="col-12 d-flex justify-content-between align-items-center">
              <span class="text-muted small">{{ $t('hosts.parsed') }} <strong>{{ parsedBulkHosts.length }}</strong> {{ $t('hosts.devices') }}</span>
              <div class="input-group" style="width: auto;">
                  <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" :placeholder="$t('hosts.destFile')">
                  <button @click="saveBulkHosts" class="btn btn-success fw-bold" :disabled="parsedBulkHosts.length === 0">{{ $t('hosts.importBtn') }}</button>
              </div>
          </div>
      </div>
  </div>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { translateApiError } from '../../i18n.js'

const { t } = useI18n()

const props = defineProps(['selectedFile', 'editData'])
const emit = defineEmits(['cancel-edit'])

const isImportMode = ref(false)
const bulkText = ref('')
const originalMac = ref('')
const originalFile = ref('')
const form = ref({ mac: '', ip: '', hostname: '', file: '' })

const isEditing = computed(() => originalMac.value !== '')

watch(() => props.selectedFile, (newFile) => {
    if (!isEditing.value) {
        form.value.file = newFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : newFile
    }
}, { immediate: true })

watch(() => props.editData, (newData) => {
    if (newData) {
        isImportMode.value = false
        originalMac.value = newData.mac
        originalFile.value = newData.file
        form.value = { mac: newData.mac, ip: newData.ip, hostname: newData.hostname, file: newData.file }
    } else {
        originalMac.value = ''
        originalFile.value = ''
        form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
        form.value.file = props.selectedFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : props.selectedFile
    }
})

watch(() => store.transferData, (data) => {
    if (data) {
        isImportMode.value = false
        emit('cancel-edit')
        form.value.mac = data.mac
        form.value.ip = data.ip
        form.value.hostname = data.hostname
    }
})

async function saveHost() {
  const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i;
  if (!macRegex.test(form.value.mac)) { alert(t('alert.invalidMac')); return; }
  if (!form.value.file) { alert(t('alert.fileRequired')); return; }
  
  try {
    if (isEditing.value) {
        await api.delete(`/hosts/${originalMac.value}?file=${encodeURIComponent(originalFile.value)}`)
    }
    await api.post('/hosts', form.value)
    
    emit('cancel-edit')
    form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
    actions.loadData()
  } catch (e) { 
    const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.saveError')
    alert(msg)
  }
}

const parsedBulkHosts = computed(() => {
    return bulkText.value.split('\n').map(line => {
        const p = line.trim().split(/\s+/)
        if(p.length >= 3) {
            return { 
                mac: p.find(x => /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i.test(x)) || '', 
                ip: p.find(x => /^(\d{1,3}\.){3}\d{1,3}$/.test(x)) || '', 
                hostname: p.find(x => !x.includes(':') && !x.includes('.')) || p[p.length-1] 
            }
        }
        return null
    }).filter(e => e && e.mac && e.ip)
})

async function saveBulkHosts() {
    if (parsedBulkHosts.value.length === 0 || !form.value.file) return
    try { 
        await api.post('/hosts/bulk', { file: form.value.file, hosts: parsedBulkHosts.value })
        bulkText.value = ''
        actions.loadData()
        alert(t('alert.importSuccess', { count: parsedBulkHosts.value.length }))
    } catch (e) { 
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.importError')
        alert(msg)
    }
}
</script>

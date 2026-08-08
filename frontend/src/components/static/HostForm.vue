<template>
  <div :class="['card mb-4 p-3 shadow-sm', isEditing ? 'border-warning' : 'border-primary']">
      <div class="d-flex justify-content-between align-items-center mb-2">
          <h6 :class="isEditing ? 'text-warning text-dark' : 'text-primary'" class="mb-0 fw-bold">
              {{ isEditing ? '✏️ ' + $t('hosts.editing') : (selectedFile === 'all' ? '➕ ' + $t('hosts.newDevice') : '➕ ' + $t('hosts.addTo') + ' ' + selectedFile.split('/').pop()) }}
          </h6>
          
          <div v-if="!isEditing">
              <select v-model="importMode" class="form-select form-select-sm" style="width: auto;">
                  <option value="single">{{ $t('hosts.add') }}</option>
                  <option value="text">{{ $t('hosts.importList') }}</option>
                  <option value="csv">{{ $t('hosts.csvMode') }}</option>
              </select>
          </div>
          
          <button v-if="isEditing" @click="$emit('cancel-edit')" class="btn btn-sm btn-outline-secondary">✕ {{ $t('hosts.cancel') }}</button>
      </div>
      
      <div v-if="importMode === 'single'" class="row g-2">
          <div class="col-12 d-flex align-items-center gap-2 mb-1">
              <select v-model="selectedTemplateId" @change="onTemplateChange" class="form-select form-select-sm" style="width: auto;">
                  <option value="">{{ $t('hosts.noTemplate', 'No template (manual)') }}</option>
                  <option v-for="tpl in store.templates" :key="tpl.id" :value="tpl.id">{{ tpl.name }}</option>
              </select>
              <button @click="showTemplatesModal = true" class="btn btn-sm btn-outline-secondary" type="button">⚙️</button>
          </div>
           <div class="col-md-3"><input v-model="form.mac" :placeholder="'MAC (aa:bb...)'" class="form-control"></div>
           <div class="col-md-3">
               <div class="input-group">
                   <input v-model="form.ip" :placeholder="$t('hosts.ipOptional', 'IP (optional)')" class="form-control">
                   <button @click="autoIP" class="btn btn-outline-secondary" :disabled="autoIPLoading" :title="$t('hosts.autoIpTooltip', 'Auto pick free IP')" type="button">
                       <span v-if="autoIPLoading">…</span><span v-else>🎲</span>
                   </button>
               </div>
               <div v-if="showRangeInput" class="mt-1">
                   <select v-if="store.dhcpRanges.length > 0" v-model="ipRange" class="form-select form-select-sm">
                       <option v-for="r in store.dhcpRanges" :key="r" :value="r">{{ r }}</option>
                       <option value="">— {{ $t('hosts.manualCidr', 'manual CIDR') }} —</option>
                   </select>
                   <input v-if="store.dhcpRanges.length === 0 || ipRange === ''" v-model="manualRange" :placeholder="$t('hosts.ipRangePlaceholder', 'CIDR 10.0.0.0/24')" class="form-control form-control-sm mt-1">
               </div>
           </div>
           <div class="col-md-3"><input v-model="form.hostname" :placeholder="$t('hosts.hostnameOptional', 'Hostname (optional)')" class="form-control"></div>
          <div class="col-md-3">
              <input v-model="tagsInput" :placeholder="$t('hosts.tagsPlaceholder', 'set:iot,set:guest')" class="form-control font-monospace" :title="$t('hosts.tagsTitle', 'DHCP tags (comma separated)')">
              <div class="form-text small">{{ $t('hosts.tagsHint', 'tags let dhcp-option=tag:... target this host') }}</div>
          </div>
          <div class="col-md-3">
              <div class="input-group">
                  <input v-model="form.file" :readonly="selectedFile !== 'all' && !isEditing" :class="['form-control', (selectedFile !== 'all' && !isEditing) ? 'bg-light' : '']" :placeholder="$t('hosts.filePlaceholder')">
                  <button @click="saveHost" class="btn fw-bold" :class="isEditing ? 'btn-warning' : 'btn-success'">
                      {{ isEditing ? $t('hosts.save') : $t('hosts.add') }}
                  </button>
              </div>
          </div>
      </div>

      <div v-if="importMode === 'text'" class="row g-2 fade-in">
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

      <div v-if="importMode === 'csv'" class="row g-2 fade-in">
          <div class="col-12">
              <input type="file" accept=".csv" @change="onCsvFileSelected" class="form-control" ref="csvInput">
          </div>
          <div class="col-12 d-flex justify-content-between align-items-center">
              <span class="text-muted small">{{ csvFileName || $t('hosts.csvMode') }}</span>
              <div class="input-group" style="width: auto;">
                  <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" :placeholder="$t('hosts.destFile')">
                  <button @click="importCSV" class="btn btn-success fw-bold" :disabled="!csvFile">{{ $t('hosts.importBtn') }}</button>
              </div>
          </div>
      </div>
  </div>

  <TemplatesModal :show="showTemplatesModal" @close="showTemplatesModal = false" />
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { translateApiError } from '../../i18n.js'
import { toast } from '../../toast.js'
import TemplatesModal from './TemplatesModal.vue'

const { t } = useI18n()

const props = defineProps(['selectedFile', 'editData'])
const emit = defineEmits(['cancel-edit'])

const importMode = ref('single')
const csvFile = ref(null)
const csvFileName = ref('')
const csvInput = ref(null)
const bulkText = ref('')
const originalMac = ref('')
const originalFile = ref('')
const form = ref({ mac: '', ip: '', hostname: '', file: '', tags: [] })
const tagsInput = ref('')
const ipRange = ref('')
const manualRange = ref('')
const showRangeInput = ref(false)
const autoIPLoading = ref(false)
const selectedTemplateId = ref('')
const showTemplatesModal = ref(false)

const isEditing = computed(() => originalMac.value !== '')

function parseTagsInput(raw) {
    // Accept host-assignment tags such as "set:iot" or "set:iot, set:guest".
    return raw
        .split(/[,\n]/)
        .map(s => s.trim())
        .filter(s => s !== '')
}

watch(() => props.selectedFile, (newFile) => {
    if (!isEditing.value) {
        form.value.file = newFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : newFile
    }
}, { immediate: true })

watch(() => props.editData, (newData) => {
    if (newData) {
        importMode.value = 'single'
        originalMac.value = newData.mac
        originalFile.value = newData.file
        form.value = { mac: newData.mac, ip: newData.ip, hostname: newData.hostname, file: newData.file, tags: [...(newData.tags || [])] }
        tagsInput.value = (newData.tags || []).join(',')
    } else {
        originalMac.value = ''
        originalFile.value = ''
        form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
        form.value.tags = []
        tagsInput.value = ''
        form.value.file = props.selectedFile === 'all' ? (store.hosts[0]?.file.split('|')[0] || '') : props.selectedFile
    }
})

watch(() => store.transferData, (data) => {
    if (data) {
        importMode.value = 'single'
        emit('cancel-edit')
        form.value.mac = data.mac
        form.value.ip = data.ip
        form.value.hostname = data.hostname
        form.value.tags = []
        tagsInput.value = ''
        store.transferData = null
    }
}, { immediate: true })

async function autoIP() {
  if (store.dhcpRanges.length === 0) await actions.loadDhcpRanges()
  let range = (ipRange.value && ipRange.value !== '') ? ipRange.value : manualRange.value.trim()
  if (!range) {
    const tpl = store.templates.find(t => t.id === selectedTemplateId.value)
    if (tpl) {
      range = tpl.ip_range
    }
  }
  if (!range && store.dhcpRanges.length > 0) {
    range = store.dhcpRanges[0]
    ipRange.value = range
  }
  if (!range) {
    showRangeInput.value = true
    return
  }
  autoIPLoading.value = true
  try {
    const res = await api.get('/hosts/next-ip', { params: { range } })
    form.value.ip = res.data.ip
    showRangeInput.value = false
  } catch (e) {
    const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.autoIpError', 'Failed to get free IP')
    toast.error(msg)
    showRangeInput.value = true
  } finally {
    autoIPLoading.value = false
  }
}

async function onTemplateChange() {
  if (!selectedTemplateId.value) return
  const tpl = store.templates.find(t => t.id === selectedTemplateId.value)
  if (!tpl) return
  form.value.file = tpl.target_file
  if (!form.value.mac) return
  const result = await actions.applyTemplate(form.value.mac, tpl.id)
  if (result) {
    form.value.ip = result.ip
    form.value.hostname = result.hostname
    form.value.file = result.file
  }
}

async function saveHost() {
  const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i;
    if (!macRegex.test(form.value.mac)) { toast.error(t('alert.invalidMac')); return; }
    if (!form.value.file) { toast.error(t('alert.fileRequired')); return; }
  // IP и hostname опциональны: dnsmasq допускает dhcp-host=<mac>,
  // dhcp-host=<mac>,<hostname>, dhcp-host=<mac>,<ip>, dhcp-host=<mac>,<hostname>,<ip>.
  // Валидация формата (если поле заполнено) делается на бэке.
  const tags = parseTagsInput(tagsInput.value)
  for (const tag of tags) {
    if (!/^(set|id):[a-zA-Z0-9_][a-zA-Z0-9_-]*$/.test(tag)) {
        toast.error(t('alert.invalidTag', 'Host tags must be set:NAME or id:NAME') + ': ' + tag)
      return
    }
  }

  try {
    if (isEditing.value) {
        await api.delete(`/hosts/${originalMac.value}?file=${encodeURIComponent(originalFile.value)}`)
    }
    await api.post('/hosts', { ...form.value, tags })

    emit('cancel-edit')
    form.value.mac = ''; form.value.ip = ''; form.value.hostname = ''
    form.value.tags = []
    tagsInput.value = ''
    actions.loadData()
  } catch (e) {
    const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.saveError')
    toast.error(msg)
  }
}

const parsedBulkHosts = computed(() => {
    // Поддерживаемые форматы одной строки (пробел как разделитель):
    //   <mac>                                → infinite lease
    //   <mac> <hostname>                     → DNS-имя, IP от DHCP
    //   <mac> <ip>                           → статический IP без DNS
    //   <mac> <hostname> <ip>                → полная запись (порядок любой,
    //                                            но hostname без точек/двоеточий)
    //   <mac> <ip> <hostname>                → то же самое
    return bulkText.value.split('\n').map(line => {
        const p = line.trim().split(/\s+/).filter(Boolean)
        if (p.length === 0) return null
        const mac = p.find(x => /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/i.test(x))
        if (!mac) return null
        const ip = p.find(x => x !== mac && /^(\d{1,3}\.){3}\d{1,3}$/.test(x))
        const remaining = p.filter(x => x !== mac && x !== ip)
        const hostname = remaining.length > 0 ? remaining[0] : ''
        return { mac, ip: ip || '', hostname: hostname || '' }
    }).filter(e => e && e.mac)
})

async function saveBulkHosts() {
    if (parsedBulkHosts.value.length === 0 || !form.value.file) return
    try { 
        await api.post('/hosts/bulk', { file: form.value.file, hosts: parsedBulkHosts.value })
        bulkText.value = ''
        actions.loadData()
        toast.success(t('alert.importSuccess', { count: parsedBulkHosts.value.length }))
    } catch (e) { 
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.importError')
        toast.error(msg)
    }
}

function onCsvFileSelected(e) {
    csvFile.value = e.target.files[0]
    csvFileName.value = csvFile.value ? csvFile.value.name : ''
}

async function importCSV() {
    if (!csvFile.value || !form.value.file) return
    await actions.importCSV(csvFile.value, form.value.file)
    csvFile.value = null
    csvFileName.value = ''
    if (csvInput.value) csvInput.value.value = ''
}
</script>

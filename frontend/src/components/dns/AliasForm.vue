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
  <div :class="['card mb-4 p-3 shadow-sm', isEditing ? 'border-warning' : 'border-primary']">
      <div class="d-flex justify-content-between align-items-center mb-2">
          <h6 :class="isEditing ? 'text-warning text-dark' : 'text-primary'" class="mb-0 fw-bold">
              {{ isEditing ? '✏️ ' + $t('dns.editing') : (selectedFile === 'all' ? '➕ ' + $t('dns.newAlias') : '➕ ' + $t('dns.addTo') + ' ' + selectedFile.split('/').pop()) }}
          </h6>

          <div v-if="!isEditing">
              <select v-model="importMode" class="form-select form-select-sm" style="width: auto;">
                  <option value="single">{{ $t('dns.add') }}</option>
                  <option value="text">{{ $t('dns.importList') }}</option>
                  <option value="csv">{{ $t('dns.csvMode') }}</option>
              </select>
          </div>

          <button v-if="isEditing" @click="$emit('cancel-edit')" class="btn btn-sm btn-outline-secondary">✕ {{ $t('dns.cancel') }}</button>
      </div>

      <div v-if="importMode === 'single'" class="row g-2">
          <div class="col-md-2">
              <select v-model="form.type" class="form-control">
                  <option value="A">A</option>
                  <option value="CNAME">CNAME</option>
                  <option value="PTR">PTR</option>
                  <option value="TXT">TXT</option>
              </select>
          </div>
          <div class="col-md-3"><input v-model="form.domain" :placeholder="$t('dns.domainPlaceholder')" class="form-control"></div>
          <div class="col-md-3">
              <input v-model="form.target" :placeholder="targetPlaceholder" class="form-control">
          </div>
          <div class="col-md-4">
              <div class="input-group">
                  <input v-model="form.file" :readonly="selectedFile !== 'all' && !isEditing" :class="['form-control', (selectedFile !== 'all' && !isEditing) ? 'bg-light' : '']" :placeholder="$t('dns.filePlaceholder')">
                  <button @click="saveAlias" class="btn fw-bold" :class="isEditing ? 'btn-warning' : 'btn-success'">
                      {{ isEditing ? $t('dns.save') : $t('dns.add') }}
                  </button>
              </div>
          </div>
          <div class="col-12">
              <span class="text-muted small">
                  <code>{{ directivePreview }}</code>
              </span>
          </div>
      </div>

      <div v-if="importMode === 'text'" class="row g-2 fade-in">
          <div class="col-12">
              <textarea v-model="bulkText" class="form-control font-monospace" rows="5" :placeholder="$t('dns.bulkPlaceholder')"></textarea>
          </div>
          <div class="col-12 d-flex justify-content-between align-items-center">
              <span class="text-muted small">{{ $t('dns.parsed') }} <strong>{{ parsedBulkAliases.length }}</strong> {{ $t('dns.aliases') }}</span>
              <div class="input-group" style="width: auto;">
                  <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" :placeholder="$t('dns.destFile')">
                  <button @click="saveBulkAliases" class="btn btn-success fw-bold" :disabled="parsedBulkAliases.length === 0">{{ $t('dns.importBtn') }}</button>
              </div>
          </div>
      </div>

      <div v-if="importMode === 'csv'" class="row g-2 fade-in">
          <div class="col-12">
              <input type="file" accept=".csv" @change="onCsvFileSelected" class="form-control" ref="csvInput">
          </div>
          <div class="col-12 d-flex justify-content-between align-items-center">
              <span class="text-muted small">{{ csvFileName || $t('dns.csvMode') }}</span>
              <div class="input-group" style="width: auto;">
                  <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" :placeholder="$t('dns.destFile')">
                  <button @click="importCSV" class="btn btn-success fw-bold" :disabled="!csvFile">{{ $t('dns.importBtn') }}</button>
              </div>
          </div>
      </div>
  </div>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'
import { toast } from '../../toast.js'

const { t } = useI18n()

const props = defineProps(['selectedFile', 'editData'])
const emit = defineEmits(['cancel-edit'])

const importMode = ref('single')
const csvFile = ref(null)
const csvFileName = ref('')
const csvInput = ref(null)
const bulkText = ref('')
const originalType = ref('')
const originalDomain = ref('')
const originalFile = ref('')
const form = ref({ type: 'A', domain: '', target: '', file: '' })

const isEditing = computed(() => originalType.value !== '')

const targetPlaceholder = computed(() => {
    switch (form.value.type) {
        case 'A': return t('dns.targetIpPlaceholder')
        case 'TXT': return t('dns.targetTxtPlaceholder')
        case 'CNAME':
        case 'PTR':
        default: return t('dns.targetDomainPlaceholder')
    }
})

const directivePreview = computed(() => {
    const d = form.value.domain || '<domain>'
    const v = form.value.target || '<value>'
    switch (form.value.type) {
        case 'A': return `address=/${d}/${v}`
        case 'CNAME': return `cname=${d},${v}`
        case 'PTR': return `ptr-record=${d},${v}`
        case 'TXT': return `txt-record=${d},${v}`
        default: return ''
    }
})

watch(() => props.selectedFile, (newFile) => {
    if (!isEditing.value) {
        form.value.file = newFile === 'all' ? (store.aliases[0]?.file.split('|')[0] || '') : newFile
    }
}, { immediate: true })

watch(() => props.editData, (newData) => {
    if (newData) {
        importMode.value = 'single'
        originalType.value = newData.type
        originalDomain.value = newData.domain
        originalFile.value = newData.file
        form.value = { type: newData.type, domain: newData.domain, target: newData.target, file: newData.file }
    } else {
        originalType.value = ''
        originalDomain.value = ''
        originalFile.value = ''
        form.value.domain = ''; form.value.target = ''
        form.value.file = props.selectedFile === 'all' ? (store.aliases[0]?.file.split('|')[0] || '') : props.selectedFile
    }
})

async function saveAlias() {
    if (!form.value.domain || !form.value.target) { toast.error(t('alert.invalidData')); return }
    if (!form.value.file) { toast.error(t('alert.fileRequired')); return }

    try {
        if (isEditing.value) {
            await actions.deleteAlias(originalType.value, originalDomain.value, originalFile.value)
        }
        const ok = await actions.addAlias({ ...form.value })
        if (ok) {
            emit('cancel-edit')
            form.value.domain = ''; form.value.target = ''
        }
    } catch (e) {
        toast.error(t('alert.aliasAddError'))
    }
}

const domainRegex = /^[a-zA-Z0-9]([a-zA-Z0-9-.]*[a-zA-Z0-9])?$/
const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$/

const parsedBulkAliases = computed(() => {
    return bulkText.value.split('\n').map(line => {
        const raw = line.trim()
        if (!raw) return null
        // Поддерживаемые форматы (одна строка — одна запись):
        //   address=/nas.lan/192.168.1.10       → A
        //   cname=wiki,nas.lan                  → CNAME
        //   ptr-record=10.in-addr.arpa,nas.lan  → PTR
        //   txt-record=nas.lan,v=spf1 -all      → TXT
        //   A nas.lan 192.168.1.10              → A (свободный текст)
        //   CNAME wiki nas.lan                  → CNAME
        //   PTR 10.in-addr.arpa nas.lan         → PTR
        //   TXT nas.lan some-value              → TXT
        if (raw.startsWith('address=/')) {
            const m = raw.match(/^address=\/([^/]+)\/(.+)$/)
            if (m && ipRegex.test(m[2])) return { type: 'A', domain: m[1], target: m[2] }
        }
        if (raw.startsWith('cname=')) {
            const parts = raw.replace(/^cname=/, '').split(',')
            if (parts.length >= 2 && domainRegex.test(parts[0]) && domainRegex.test(parts[1])) {
                return { type: 'CNAME', domain: parts[0].trim(), target: parts[1].trim() }
            }
        }
        if (raw.startsWith('ptr-record=')) {
            const parts = raw.replace(/^ptr-record=/, '').split(',')
            if (parts.length >= 2 && domainRegex.test(parts[0]) && domainRegex.test(parts[parts.length - 1])) {
                return { type: 'PTR', domain: parts[0].trim(), target: parts[parts.length - 1].trim() }
            }
        }
        if (raw.startsWith('txt-record=')) {
            const rest = raw.replace(/^txt-record=/, '')
            const idx = rest.indexOf(',')
            if (idx > 0 && domainRegex.test(rest.slice(0, idx).trim())) {
                return { type: 'TXT', domain: rest.slice(0, idx).trim(), target: rest.slice(idx + 1).trim() }
            }
        }
        const p = raw.split(/\s+/)
        if (p.length >= 3) {
            const tp = p[0].toUpperCase()
            if (tp === 'A' && ipRegex.test(p[2]) && domainRegex.test(p[1])) {
                return { type: 'A', domain: p[1], target: p[2] }
            }
            if ((tp === 'CNAME' || tp === 'CN') && domainRegex.test(p[1]) && domainRegex.test(p[2])) {
                return { type: 'CNAME', domain: p[1], target: p[2] }
            }
            if (tp === 'PTR' && domainRegex.test(p[1]) && domainRegex.test(p[2])) {
                return { type: 'PTR', domain: p[1], target: p[2] }
            }
            if (tp === 'TXT' && domainRegex.test(p[1])) {
                // TXT-значение может содержать пробелы — склеим обратно.
                return { type: 'TXT', domain: p[1], target: p.slice(2).join(' ') }
            }
        }
        return null
    }).filter(e => e && e.domain && e.target)
})

async function saveBulkAliases() {
    if (parsedBulkAliases.value.length === 0 || !form.value.file) return
    const res = await actions.bulkAddAliases(parsedBulkAliases.value, form.value.file)
    if (res) {
        bulkText.value = ''
        toast.success(t('alert.importSuccess', { count: res.count }))
    }
}

function onCsvFileSelected(e) {
    csvFile.value = e.target.files[0]
    csvFileName.value = csvFile.value ? csvFile.value.name : ''
}

async function importCSV() {
    if (!csvFile.value || !form.value.file) return
    await actions.importAliasesCSV(csvFile.value, form.value.file)
    csvFile.value = null
    csvFileName.value = ''
    if (csvInput.value) csvInput.value.value = ''
}
</script>

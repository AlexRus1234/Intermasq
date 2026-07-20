<template>
  <div class="fade-in">
    <div class="d-flex justify-content-between align-items-center mb-3 flex-wrap gap-2">
      <ul class="nav nav-tabs mb-0 flex-grow-1 border-bottom-0">
        <li class="nav-item" v-for="f in files" :key="f.path">
          <a class="nav-link" :class="{active: selectedFile === f.path}" href="#" @click.prevent="selectFile(f.path)">
            {{ f.name }}
            <span v-if="f.has_bak" class="badge bg-warning ms-1" title="has .bak">⏪</span>
          </a>
        </li>
        <li class="nav-item">
          <a class="nav-link text-success" href="#" @click.prevent="openNewFileForm">+ {{ $t('config.newFile') }}</a>
        </li>
      </ul>
      <div class="d-flex gap-2">
        <button v-if="currentFile" @click="showHistory = true" class="btn btn-sm btn-outline-secondary" :title="$t('history.iconTooltip')">
          🕒 {{ $t('history.icon') }}
        </button>
        <button v-if="currentFile && currentFile.has_bak" @click="rollback" class="btn btn-sm btn-outline-warning">
          ⏪ {{ $t('config.rollback') }}
        </button>
        <button v-if="currentFile" @click="deleteFile" class="btn btn-sm btn-outline-danger" :title="$t('config.delete', 'Delete file')">
          🗑 {{ $t('config.delete', 'Delete') }}
        </button>
        <button @click="save" :disabled="!currentFile" class="btn btn-primary fw-bold">
          💾 {{ $t('config.save') }}
        </button>
      </div>
    </div>

    <div v-if="showNewFile" class="card mb-3 border-success">
      <div class="card-body">
        <div class="d-flex gap-2 align-items-center mb-2">
          <input v-model="newFileName" class="form-control" placeholder="filename.conf">
          <select v-model="newFileTemplate" class="form-select" style="max-width: 220px;" :title="$t('config.template')">
            <option value="empty">∅ {{ $t('config.templateEmpty') }}</option>
            <option v-for="tpl in nonEmptyTemplates" :key="tpl.id" :value="tpl.id">{{ tpl.id }}</option>
          </select>
          <button @click="createFile" class="btn btn-success">＋</button>
          <button @click="showNewFile = false" class="btn btn-outline-secondary">✕</button>
        </div>
        <pre v-if="selectedTemplatePreview" class="form-text bg-body-secondary border rounded p-2 mb-0" style="max-height: 200px; overflow:auto; font-size: 0.8em;">{{ selectedTemplatePreview }}</pre>
      </div>
    </div>

    <div v-if="!currentFile" class="alert alert-secondary">{{ $t('config.selectFile') }}</div>

    <div v-if="currentFile">
      <div v-if="groupedDirectives.length === 0" class="alert alert-light">{{ $t('config.noDirectives') }}</div>

      <div v-for="g in groupedDirectives" :key="g.group" class="card mb-3 shadow-sm">
        <div class="card-header bg-light fw-bold">{{ $t(GROUP_LABELS[g.group]) }}</div>
        <div class="card-body">
          <div v-for="d in g.directives" :key="d._uid" class="row mb-2 align-items-center g-2">
            <template v-if="schemaFor(d.key).type === 'bool'">
              <div class="col-auto">
                <div class="form-check form-switch">
                  <input class="form-check-input" type="checkbox" v-model="d.active" :id="'d-'+d._uid">
                  <label class="form-check-label" :for="'d-'+d._uid">{{ d.key }}</label>
                </div>
              </div>
              <div class="col"></div>
              <div class="col-auto">
                <button @click="removeDirective(d)" class="btn btn-sm btn-outline-danger">🗑</button>
              </div>
            </template>

            <template v-else-if="schemaFor(d.key).type === 'string'">
              <div class="col-md-4 col-lg-3">
                <code>{{ d.key }}</code>
              </div>
              <div class="col">
                <input v-model="d.value" class="form-control form-control-sm" :disabled="!d.active">
              </div>
              <div class="col-auto">
                <div class="form-check form-switch">
                  <input class="form-check-input" type="checkbox" v-model="d.active">
                </div>
              </div>
              <div class="col-auto">
                <button @click="removeDirective(d)" class="btn btn-sm btn-outline-danger">🗑</button>
              </div>
            </template>

            <template v-else-if="schemaFor(d.key).type === 'list'">
              <div class="col-md-3 col-lg-2">
                <code>{{ d.key }}</code>
              </div>
              <div class="col">
                <input v-model="d.value" class="form-control form-control-sm" :disabled="!d.active" placeholder="value">
              </div>
              <div class="col-auto">
                <div class="form-check form-switch">
                  <input class="form-check-input" type="checkbox" v-model="d.active">
                </div>
              </div>
              <div class="col-auto">
                <button @click="removeDirective(d)" class="btn btn-sm btn-outline-danger">🗑</button>
              </div>
            </template>

            <template v-else-if="schemaFor(d.key).type === 'dhcprange'">
              <div class="col-12">
                <DhcpRangeRow :directive="d" @remove="removeDirective(d)" />
              </div>
            </template>

            <template v-else-if="schemaFor(d.key).type === 'dhcpoption'">
              <div class="col-12">
                <DhcpOptionRow :directive="d" @remove="removeDirective(d)" />
              </div>
            </template>

            <template v-else-if="schemaFor(d.key).type === 'forwarding'">
              <div class="col-12">
                <ForwardingRow :directive="d" @remove="removeDirective(d)" />
              </div>
            </template>
          </div>

          <div class="mt-2 d-flex gap-2 flex-wrap">
            <button @click="addDirective(g.group, 'dns')" v-if="g.group==='dns'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addDirective') }}</button>
            <button @click="addDhcpRange" v-if="g.group==='dhcp'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addRange') }}</button>
            <button @click="addDhcpOption" v-if="g.group==='dhcp'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addOption') }}</button>
            <button @click="addDirective(g.group, 'dhcp')" v-if="g.group==='dhcp'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addDirective') }}</button>
            <button @click="addDirective(g.group, 'pxe')" v-if="g.group==='pxe'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addDirective') }}</button>
            <button @click="addDirective(g.group, 'log')" v-if="g.group==='log'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addDirective') }}</button>
            <button @click="addDirective(g.group, 'other')" v-if="g.group==='other'" class="btn btn-sm btn-outline-primary">+ {{ $t('config.addDirective') }}</button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="addingKey" class="card mb-3 border-primary">
      <div class="card-body d-flex gap-2 align-items-center">
        <select v-model="addingKeyName" class="form-select form-select-sm" style="max-width: 250px;">
          <option v-for="k in availableSchemaKeys(addingGroup)" :key="k" :value="k">{{ k }}</option>
        </select>
        <input v-model="addingCustomKey" class="form-control form-control-sm" style="max-width: 250px;" :placeholder="$t('config.customKey')">
        <button @click="confirmAdd" class="btn btn-sm btn-primary">＋</button>
        <button @click="cancelAdd" class="btn btn-sm btn-outline-secondary">✕</button>
      </div>
    </div>

    <HistoryModal
      :show="showHistory"
      :file="selectedFile"
      @close="showHistory = false"
      @restored="actions.loadConfig()"
    />
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, api, actions } from '../../store.js'
import { translateApiError } from '../../i18n.js'
import { DIRECTIVE_SCHEMA, GROUP_ORDER, GROUP_LABELS, schemaFor } from './directives.js'
import DhcpRangeRow from './DhcpRangeRow.vue'
import DhcpOptionRow from './DhcpOptionRow.vue'
import ForwardingRow from './ForwardingRow.vue'
import HistoryModal from '../history/HistoryModal.vue'

const { t } = useI18n()

const selectedFile = ref('')
const localDirectives = ref([])
let uidCounter = 0
const showNewFile = ref(false)
const newFileName = ref('')
const newFileTemplate = ref('empty')
const addingKey = ref(false)
const addingGroup = ref('')
const addingKeyName = ref('')
const addingCustomKey = ref('')
const showHistory = ref(false)

const files = computed(() => store.configSnapshot?.files || [])

const currentFile = computed(() => files.value.find(f => f.path === selectedFile.value))

const nonEmptyTemplates = computed(() => (store.configTemplates || []).filter(t => t.id !== 'empty'))

const selectedTemplatePreview = computed(() => {
  const t = (store.configTemplates || []).find(t => t.id === newFileTemplate.value)
  return t?.preview || ''
})

function selectFile(path) {
  selectedFile.value = path
}

watch(currentFile, (f) => {
  if (!f) { localDirectives.value = []; return }
  localDirectives.value = f.directives.map(d => ({ ...d, _uid: ++uidCounter }))
}, { immediate: true })

watch(() => store.configSnapshot, () => {
  if (!selectedFile.value && files.value.length > 0) {
    selectedFile.value = files.value[0].path
  }
}, { immediate: true })

const groupedDirectives = computed(() => {
  const groups = {}
  for (const d of localDirectives.value) {
    const g = schemaFor(d.key).group
    if (!groups[g]) groups[g] = []
    groups[g].push(d)
  }
  return GROUP_ORDER
    .map(g => ({ group: g, directives: groups[g] || [] }))
    .filter(g => g.directives.length > 0)
})

function removeDirective(d) {
  const i = localDirectives.value.findIndex(x => x._uid === d._uid)
  if (i >= 0) localDirectives.value.splice(i, 1)
}

function availableSchemaKeys(group) {
  return Object.entries(DIRECTIVE_SCHEMA)
    .filter(([_, s]) => s.group === group)
    .map(([k]) => k)
}

function addDirective(group, _kind) {
  addingKey.value = true
  addingGroup.value = group
  addingKeyName.value = ''
  addingCustomKey.value = ''
}

function confirmAdd() {
  const key = addingCustomKey.value.trim() || addingKeyName.value
  if (!key) return
  localDirectives.value.push({ key, value: '', active: true, _uid: ++uidCounter })
  addingKey.value = false
  addingCustomKey.value = ''
  addingKeyName.value = ''
}

function cancelAdd() {
  addingKey.value = false
  addingCustomKey.value = ''
  addingKeyName.value = ''
}

function addDhcpRange() {
  localDirectives.value.push({ key: 'dhcp-range', value: ',,,,', active: true, _uid: ++uidCounter })
}

function addDhcpOption() {
  localDirectives.value.push({ key: 'dhcp-option', value: 'option:router,', active: true, _uid: ++uidCounter })
}

async function save() {
  if (!currentFile.value) return
  if (!confirm(t('config.saveConfirm'))) return
  const payload = localDirectives.value.map(d => ({
    key: d.key,
    value: d.value,
    active: d.active
  }))
  const ok = await actions.saveConfig(selectedFile.value, payload)
  if (ok) {
    const f = files.value.find(f => f.path === selectedFile.value)
    if (f) {
      localDirectives.value = f.directives.map(d => ({ ...d, _uid: ++uidCounter }))
    }
    alert(t('alert.configSaveSuccess'))
  }
}

async function rollback() {
  if (!confirm(t('confirm.rollback', { file: currentFile.value.name }))) return
  try {
    await api.post('/rollback', { file: selectedFile.value })
    await actions.loadConfig()
    alert(t('alert.rollbackSuccess'))
  } catch (e) {
    const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.rollbackError')
    alert(msg)
  }
}

async function createFile() {
  let name = newFileName.value.trim()
  if (!name) return
  if (!name.endsWith('.conf')) name += '.conf'
  const ok = await actions.createConfigFile(name, newFileTemplate.value)
  if (ok) {
    showNewFile.value = false
    newFileName.value = ''
    newFileTemplate.value = 'empty'
    const f = files.value.find(f => f.name === name)
    if (f) selectFile(f.path)
  }
}

// deleteFile — physically removes the current .conf file from -conf-dir.
// The backend takes a snapshot into versioned history before deletion, so
// the file can still be recovered via the history modal if the operator
// changes their mind. We do NOT run `dnsmasq --test` after deletion —
// dnsmasq simply stops loading the absent file on next reload.
async function deleteFile() {
  if (!currentFile.value) return
  // Two-step confirm: a deleted file is a more permanent change than a
  // failed edit, and `.bak`-based rollback no longer applies (the file
  // itself is gone). The versioned-history recovery flow is less obvious.
  if (!confirm(t('confirm.deleteConfigFile', { file: currentFile.value.name }))) return
  const ok = await actions.deleteConfigFile(selectedFile.value)
  if (ok) {
    selectedFile.value = ''
    alert(t('alert.configDeleteSuccess', 'File deleted. Click "Apply" to activate.'))
  }
}

// Ленивая подгрузка каталога шаблонов при первом открытии формы создания
// файла. Если бекенд недоступен — fallback на ["empty"] в store.loadConfigTemplates.
let templatesLoaded = false
function openNewFileForm() {
  showNewFile.value = true
  newFileTemplate.value = 'empty'
  if (!templatesLoaded) {
    templatesLoaded = true
    actions.loadConfigTemplates()
  }
}
</script>

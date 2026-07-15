<template>
  <div class="card shadow-sm border-top-0" style="border-top-left-radius: 0;">
      <table class="table table-hover mb-0 align-middle">
      <thead class="table-light">
          <tr>
              <th style="width: 90px;">{{ $t('dns.type') }}</th>
              <th @click="sortBy('domain')" style="cursor:pointer">{{ $t('dns.domain') }} ↕</th>
              <th @click="sortBy('target')" style="cursor:pointer">{{ $t('dns.target') }} ↕</th>
              <th v-if="selectedFile==='all'">{{ $t('dns.fileCol') }}</th>
              <th class="text-end">{{ $t('dns.actions') }}</th>
          </tr>
      </thead>
      <tbody>
          <tr v-for="a in sortedAliases" :key="a.type + '|' + a.domain + '|' + cleanPath(a.file)">
              <td>
                  <span :class="['badge', a.type === 'A' ? 'bg-primary' : 'bg-info']">{{ a.type }}</span>
              </td>
              <td class="font-monospace">
                  <span v-if="store.searchQuery && a.domain.toLowerCase().includes(store.searchQuery.toLowerCase())" class="bg-warning text-dark px-1 rounded">{{ a.domain }}</span>
                  <span v-else>{{ a.domain }}</span>
              </td>
              <td class="fw-bold" :class="a.type === 'A' ? 'text-primary' : 'text-info'">{{ a.target }}</td>
              <td v-if="selectedFile==='all'" class="small text-muted">{{ cleanPath(a.file).split('/').pop() }}</td>
              <td class="text-end">
                  <div class="btn-group">
                      <button @click="$emit('edit-alias', a)" class="btn btn-sm btn-outline-secondary" :title="$t('dns.editTooltip')">✏️</button>
                      <button @click="deleteAlias(a)" class="btn btn-sm btn-outline-danger" :title="$t('dns.deleteTooltip')">✕</button>
                  </div>
              </td>
          </tr>
          <tr v-if="sortedAliases.length === 0">
              <td :colspan="selectedFile === 'all' ? 5 : 4" class="text-center p-4 text-muted">
                  {{ store.searchQuery ? $t('dns.searchEmpty') : $t('dns.empty') }}
              </td>
          </tr>
      </tbody>
      </table>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'

const { t } = useI18n()

const props = defineProps(['selectedFile'])
const emit = defineEmits(['edit-alias'])

const sortKey = ref('domain')
const sortAsc = ref(true)

const cleanPath = (path) => path ? path.split('|')[0] : ''

function sortBy(key) {
    if (sortKey.value === key) sortAsc.value = !sortAsc.value
    else { sortKey.value = key; sortAsc.value = true }
}

const sortedAliases = computed(() => {
    let data = [...store.aliases]

    if (props.selectedFile !== 'all') {
        data = data.filter(a => cleanPath(a.file) === props.selectedFile)
    }

    if (store.searchQuery) {
        const q = store.searchQuery.toLowerCase()
        data = data.filter(a =>
            (a.domain && a.domain.toLowerCase().includes(q)) ||
            (a.target && a.target.toLowerCase().includes(q)) ||
            (a.type && a.type.toLowerCase().includes(q))
        )
    }

    return data.sort((a, b) => {
        const valA = a[sortKey.value] || ''
        const valB = b[sortKey.value] || ''
        if (sortKey.value === 'target') {
            // Natural sort for IPs.
            if (/^(\d{1,3}\.){3}\d{1,3}$/.test(valA) && /^(\d{1,3}\.){3}\d{1,3}$/.test(valB)) {
                const numA = valA.split('.').map(Number)
                const numB = valB.split('.').map(Number)
                for (let i = 0; i < 4; i++) {
                    if (numA[i] !== numB[i]) return sortAsc.value ? numA[i] - numB[i] : numB[i] - numA[i]
                }
                return 0
            }
        }
        if (valA < valB) return sortAsc.value ? -1 : 1
        if (valA > valB) return sortAsc.value ? 1 : -1
        return 0
    })
})

async function deleteAlias(a) {
    if (!confirm(t('confirm.deleteAlias', { domain: a.domain }))) return
    await actions.deleteAlias(a.type, a.domain, cleanPath(a.file))
}
</script>

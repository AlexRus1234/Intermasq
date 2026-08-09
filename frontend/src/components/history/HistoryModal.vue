<template>
  <div v-if="show" class="modal-backdrop fade-in" @click.self="$emit('close')">
    <div class="modal-dialog modal-lg modal-dialog-centered d-block">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">🕒 {{ $t('history.title') }}</h5>
          <button type="button" class="btn-close" @click="$emit('close')"></button>
        </div>
        <div class="modal-body">
          <div v-if="loading" class="text-center p-3 text-muted">{{ $t('history.loading') }}</div>

          <div v-else-if="store.history.length === 0" class="text-muted text-center p-3">
            {{ $t('history.empty') }}
          </div>

          <template v-else>
            <div class="table-responsive">
              <table class="table table-sm align-middle">
                <thead>
                  <tr>
                    <th>{{ $t('history.version') }}</th>
                    <th>{{ $t('history.size') }}</th>
                    <th class="text-end">{{ $t('history.actions') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="v in store.history" :key="v.version" :data-version="v.version">
                    <td><code>{{ v.version }}</code></td>
                    <td class="text-muted">{{ v.size }} {{ $t('history.bytes') }}</td>
                    <td class="text-end">
                      <button class="btn btn-sm btn-outline-primary me-1" @click="diffCurrent(v.version)" :title="$t('history.diffVsCurrent')">
                        ≠
                      </button>
                      <button v-if="store.isAdmin" class="btn btn-sm btn-outline-warning" @click="restore(v.version)" :title="$t('history.restore')">
                        ⏪
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div v-if="store.historyDiff" class="mt-3">
              <hr>
              <h6>{{ $t('history.diffTitle') }}</h6>
              <pre class="history-diff"><code>{{ store.historyDiff }}</code></pre>
            </div>
          </template>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-secondary" @click="$emit('close')">{{ $t('app.close') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'

const props = defineProps({
  show: { type: Boolean, default: false },
  file: { type: String, default: '' }
})

const emit = defineEmits(['restored'])
const { t } = useI18n()
const loading = ref(false)

watch(() => props.show, async (v) => {
  if (v && props.file) {
    loading.value = true
    await actions.loadHistory(props.file)
    loading.value = false
  }
}, { immediate: true })

async function diffCurrent(version) {
  await actions.loadHistoryDiff(props.file, version, 'current')
}

async function restore(version) {
  if (!confirm(t('confirm.restore', { version }))) return
  const ok = await actions.restoreHistory(props.file, version)
  if (ok) {
    emit('restored')
    emit('close')
  }
}
</script>

<style scoped>
.history-diff {
  max-height: 360px;
  overflow: auto;
  background: var(--bs-dark, #212529);
  color: var(--bs-light, #f8f9fa);
  padding: 0.75rem;
  border-radius: 0.375rem;
  font-size: 0.8rem;
  line-height: 1.25;
  white-space: pre;
}
</style>

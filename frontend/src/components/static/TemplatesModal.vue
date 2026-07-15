<template>
  <div v-if="show" class="modal-backdrop fade-in" @click.self="$emit('close')">
    <div class="modal-dialog modal-lg modal-dialog-centered d-block">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">⚙️ {{ $t('templates.title', 'Templates') }}</h5>
          <button type="button" class="btn-close" @click="$emit('close')"></button>
        </div>
        <div class="modal-body">
          <div v-if="store.templates.length === 0" class="text-muted text-center p-3">
            {{ $t('templates.empty', 'No templates yet') }}
          </div>
          <ul v-else class="list-group mb-3">
            <li v-for="t in store.templates" :key="t.id" class="list-group-item d-flex justify-content-between align-items-center">
              <div>
                <strong>{{ t.name }}</strong>
                <div class="small text-muted">
                  {{ t.ip_range }} · {{ t.hostname_pattern }} · {{ t.target_file.split('/').pop() }}
                </div>
              </div>
              <button @click="remove(t.id)" class="btn btn-sm btn-outline-danger">✕</button>
            </li>
          </ul>

          <hr>
          <h6>{{ $t('templates.new', 'New template') }}</h6>
          <div class="row g-2 mt-1">
            <div class="col-md-6"><input v-model="form.name" :placeholder="$t('templates.namePlaceholder', 'Name')" class="form-control"></div>
            <div class="col-md-6"><input v-model="form.ip_range" placeholder="10.0.0.0/24" class="form-control"></div>
            <div class="col-md-6"><input v-model="form.hostname_pattern" placeholder="device-{NNN}" class="form-control"></div>
            <div class="col-md-6"><input v-model="form.target_file" :placeholder="$t('templates.filePlaceholder', '/etc/dnsmasq.d/hosts.conf')" class="form-control"></div>
          </div>
        </div>
        <div class="modal-footer">
          <button @click="$emit('close')" class="btn btn-outline-secondary">{{ $t('hosts.cancel', 'Cancel') }}</button>
          <button @click="create" class="btn btn-success" :disabled="!canCreate">{{ $t('templates.create', 'Create') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'

const { t } = useI18n()
defineProps(['show'])
const emit = defineEmits(['close'])

const form = ref({ name: '', ip_range: '', hostname_pattern: '', target_file: '' })

const canCreate = computed(() =>
  form.value.name && form.value.ip_range && form.value.hostname_pattern && form.value.target_file
)

async function create() {
  const ok = await actions.createTemplate({ ...form.value })
  if (ok) {
    form.value = { name: '', ip_range: '', hostname_pattern: '', target_file: '' }
  }
}

async function remove(id) {
  if (!confirm(t('confirm.deleteTemplate', 'Delete template?'))) return
  await actions.deleteTemplate(id)
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(0,0,0,0.5);
  z-index: 1050;
  padding: 1rem;
}
</style>

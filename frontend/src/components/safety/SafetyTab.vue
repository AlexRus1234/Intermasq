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
  <div class="fade-in">
    <!-- Section 1: Backup / Restore -->
    <div class="card shadow-sm mb-3 border-info">
      <div class="card-header bg-info text-dark fw-bold d-flex justify-content-between align-items-center">
          <span>💾 {{ $t('safety.backupTitle', 'Backup & Restore') }}</span>
      </div>
      <div class="card-body">
        <p class="text-muted small mb-3">{{ $t('safety.backupHint', 'A live snapshot of every .conf file in /etc/dnsmasq.d, downloaded as a ZIP. Restoring validates the archive with dnsmasq --test and backs up current files as .restore.bak before overwriting.') }}</p>
        <div class="d-flex gap-2 flex-wrap">
          <button @click="actions.downloadBackup()" class="btn btn-info fw-bold">💾 {{ $t('app.backup') }}</button>
          <button v-if="store.isAdmin" @click="uploadRestore()" class="btn btn-outline-warning fw-bold">📤 {{ $t('app.restore') }}</button>
        </div>
      </div>
    </div>

    <!-- Section 2: Templates -->
    <div class="card shadow-sm mb-3 border-secondary">
      <div class="card-header bg-secondary text-white fw-bold d-flex justify-content-between align-items-center">
          <span>⚙️ {{ $t('safety.templatesTitle', 'Templates') }}</span>
          <button @click="showTemplates = true" class="btn btn-sm btn-light">⚙️ {{ $t('safety.manage', 'Manage') }}</button>
      </div>
      <div class="card-body p-0">
        <div v-if="store.templates.length === 0" class="text-muted text-center p-3 small">
          {{ $t('templates.empty', 'No templates yet') }}
        </div>
        <ul v-else class="list-group list-group-flush">
          <li v-for="tpl in store.templates" :key="tpl.id" class="list-group-item d-flex justify-content-between align-items-center small">
            <div>
              <strong>{{ tpl.name }}</strong>
              <span class="text-muted ms-2 font-monospace">{{ tpl.ip_range }} · {{ tpl.hostname_pattern }} · {{ tpl.target_file.split('/').pop() }}</span>
            </div>
          </li>
        </ul>
      </div>
    </div>

    <!-- Section 3: Audit log -->
    <AuditTab />

    <TemplatesModal :show="showTemplates" @close="showTemplates = false" />
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { store, actions } from '../../store.js'
import AuditTab from '../audit/AuditTab.vue'
import TemplatesModal from '../static/TemplatesModal.vue'

const { t } = useI18n()
const showTemplates = ref(false)

function uploadRestore() {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.zip'
    input.onchange = async (e) => {
        const file = e.target.files[0]
        if (!file) return
        if (!confirm(t('safety.restoreConfirm', 'Restore configuration from backup? Current files will be backed up as .restore.bak'))) return
        await actions.restoreBackup(file)
    }
    input.click()
}
</script>

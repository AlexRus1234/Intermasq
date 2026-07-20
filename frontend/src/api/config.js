// dnsmasq config editor + versioned history + file deletion actions.
// Backed by handlers_config.go and handlers_safety.go.

import { store, api } from '../store.js'
import i18n, { translateApiError } from '../i18n.js'

const { t } = i18n.global

export async function loadConfig() {
    try {
        const res = await api.get('/config')
        store.configSnapshot = res.data
        const rRes = await api.get('/templates/ranges')
        store.dhcpRanges = rRes.data.ranges || []
    } catch (e) {
        if (e.response?.status === 401) {
            // Defer to store.js: clear token + bounce to login.
            store.token = ''
            localStorage.removeItem('token')
            store.view = 'login'
        }
    }
}

export async function saveConfig(file, directives) {
    try {
        const res = await api.put('/config', { file, directives })
        store.configSnapshot = res.data
        const rRes = await api.get('/templates/ranges')
        store.dhcpRanges = rRes.data.ranges || []
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.configSaveError')
        if (e.response?.data?.detail) {
            alert(msg + '\n\n' + e.response.data.detail)
        } else {
            alert(msg)
        }
        return false
    }
}

export async function createConfigFile(name, template = 'empty') {
    try {
        const res = await api.post('/config/file', { name, template })
        store.configSnapshot = res.data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.configCreateError')
        alert(msg)
        return false
    }
}

// deleteConfigFile — removes a .conf file from -conf-dir. Returns the
// updated ConfigSnapshot so the caller can drop the deleted file tab.
// The backend takes a backup into history before deletion, so a deleted
// file can still be recovered via the versioned-history modal (shown on
// the remaining files).
export async function deleteConfigFile(file) {
    try {
        const res = await api.delete('/config/file', { data: { file } })
        store.configSnapshot = res.data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.configDeleteError', 'Failed to delete file')
        alert(msg)
        return false
    }
}

export async function loadConfigTemplates() {
    try {
        const res = await api.get('/config/templates')
        store.configTemplates = res.data.templates || []
    } catch (e) {
        store.configTemplates = [{ id: 'empty', preview: '# === Managed by Intermasq ===\n' }]
    }
}

export async function loadDhcpRanges() {
    try {
        const res = await api.get('/templates/ranges')
        store.dhcpRanges = res.data.ranges || []
    } catch (e) {}
}

// ===== Versioned history =====

export async function loadHistory(file) {
    try {
        const res = await api.get('/history', { params: { file } })
        store.history = res.data.versions || []
        store.historyDiff = ''
        return true
    } catch (e) {
        store.history = []
        store.historyDiff = ''
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.historyLoadError')
        alert(msg)
        return false
    }
}

export async function loadHistoryDiff(file, from, to) {
    try {
        const res = await api.get('/history/diff', { params: { file, from, to } })
        store.historyDiff = res.data.diff || ''
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.historyDiffError')
        alert(msg)
        return false
    }
}

export async function restoreHistory(file, version) {
    try {
        await api.post('/history/restore', { file, version })
        alert(t('alert.restoreSuccess'))
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.restoreError')
        alert(msg)
        return false
    }
}

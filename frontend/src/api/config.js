// dnsmasq config editor + versioned history + file deletion actions.
// Backed by handlers_config.go and handlers_safety.go.

import { store, api, actions } from '../store.js'
import i18n, { translateApiError } from '../i18n.js'

const { t } = i18n.global

export async function loadConfig() {
    try {
        const res = await api.get('/config')
        store.configSnapshot = res.data
        const rRes = await api.get('/templates/ranges')
        store.dhcpRanges = rRes.data.ranges || []
    } catch (e) {
        if (e.response?.status === 401) actions.logout()
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

// ===== Raw text editor (GET/PUT /api/files/:name) =====
//
// The visual editor (saveConfig above) round-trips through a directive
// model — fine for known keys, but it cannot represent arbitrary raw
// lines a power user might want (comments, ordering, unsupported
// directives). The raw path hands the file contents to the backend as
// plain text; writeFileRaw still runs `dnsmasq --test` + .bak rollback,
// so the safety guarantee is identical to the visual path. PUT is
// admin-only on the backend; a non-admin gets 403, surfaced via alert.
//
// Unlike the snapshot-based helpers above, raw content is transient
// per-edit, so we return it to the caller instead of parking it in
// store — the textarea lives entirely inside DnsmasqConfig.vue.

export async function loadRawFile(name) {
    try {
        const res = await api.get(`/files/${encodeURIComponent(name)}`)
        return res.data.content
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.rawLoadError')
        alert(msg)
        return null
    }
}

export async function saveRawFile(name, content) {
    try {
        await api.put(`/files/${encodeURIComponent(name)}`, { content })
        return true
    } catch (e) {
        // dnsmasq_test_failed comes back with a `detail` field carrying the
        // raw dnsmasq output — show it so the user can see WHICH line
        // dnsmasq rejected. Mirrors saveConfig's error shape exactly.
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.rawSaveError')
        if (e.response?.data?.detail) {
            alert(msg + '\n\n' + e.response.data.detail)
        } else {
            alert(msg)
        }
        return false
    }
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

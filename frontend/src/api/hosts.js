// Host / template / lease actions. All async, all return success boolean
// (or response data) so callers can chain UI feedback. Imported into the
// actions object in store.js via object spread.
//
// Errors are surfaced via alert() with a translated message — same UX as
// the original monolithic store.js, just split out for navigability.

import { store, api } from '../store.js'
import i18n, { translateApiError } from '../i18n.js'

const { t } = i18n.global

export async function loadTemplates() {
    try {
        const res = await api.get('/templates')
        store.templates = res.data
    } catch (e) {}
}

export async function createTemplate(template) {
    try {
        await api.post('/templates', template)
        await loadTemplates()
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.templateCreateError', 'Failed to create template')
        alert(msg)
        return false
    }
}

export async function deleteTemplate(id) {
    try {
        await api.delete(`/templates/${id}`)
        await loadTemplates()
        return true
    } catch (e) {
        alert(t('alert.templateDeleteError', 'Failed to delete template'))
        return false
    }
}

export async function applyTemplate(mac, templateId) {
    try {
        const res = await api.post('/hosts/apply-template', { mac, template_id: templateId })
        return res.data
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.templateApplyError', 'Failed to apply template')
        alert(msg)
        return null
    }
}

export async function bulkMove(hosts, target) {
    try {
        const res = await api.post('/hosts/bulk-move', { hosts, target })
        store.hosts = (await api.get('/hosts')).data
        return res.data
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkMoveError', 'Failed to move hosts')
        alert(msg)
        return null
    }
}

export async function bulkEdit(hosts, ipTransform, hostnameTransform) {
    try {
        const res = await api.post('/hosts/bulk-edit', { hosts, ip_transform: ipTransform, hostname_transform: hostnameTransform })
        store.hosts = (await api.get('/hosts')).data
        return res.data
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkEditError', 'Failed to edit hosts')
        alert(msg)
        return null
    }
}

export async function downloadCSV() {
    try {
        const res = await api.get('/hosts/csv', { responseType: 'blob' })
        const url = window.URL.createObjectURL(new Blob([res.data]))
        const link = document.createElement('a')
        link.href = url
        link.setAttribute('download', 'intermasq_hosts.csv')
        document.body.appendChild(link); link.click(); document.body.removeChild(link)
    } catch (e) { alert(t('alert.csvExportError')) }
}

export async function importCSV(file, targetFile) {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('target_file', targetFile)
    try {
        const res = await api.post('/hosts/csv', formData)
        alert(t('alert.csvImportSuccess', { count: res.data.count }))
        store.hosts = (await api.get('/hosts')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.csvImportError')
        alert(msg)
        return false
    }
}

export async function loadNewDevices() {
    try {
        const res = await api.get('/new-devices')
        store.newDevices = res.data
    } catch (e) {}
}

// bulkLeaseToStatic — writes one dhcp-host= line per selected lease.
// The backend does NOT run `dnsmasq --test` here (see backend docs), so
// after a successful conversion the UI shows an explicit reminder that
// the user must click "Apply" for dnsmasq to actually pick up the new
// entries. Without this reminder users reported "I added 5 devices and
// they didn't get static IPs" — they didn't realize apply was required.
export async function bulkLeaseToStatic(leases, file) {
    try {
        const res = await api.post('/leases/to-static', { leases, file })
        const count = res.data.count
        alert(t('alert.bulkLeaseToStaticSuccess', { count }) + '\n\n⚠️ ' + t('alert.applyReminder', 'dnsmasq --test was NOT run. Click "Apply" to activate changes.'))
        store.hosts = (await api.get('/hosts')).data
        store.leases = (await api.get('/leases')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkLeaseToStaticError')
        alert(msg)
        return false
    }
}

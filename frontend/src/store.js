import { reactive } from 'vue'
import axios from 'axios'
import i18n, { translateApiError } from './i18n.js'

const { t } = i18n.global

export const store = reactive({
    token: localStorage.getItem('token') || '',
    view: 'loading',
    tab: 'static',
    isDnsmasqActive: false,
    searchQuery: '',
    
    hosts: [],
    leases: [],
    arpTable: {},
    plugins: [],
    auditLog: [],
    templates: [],
    configSnapshot: null,
    dhcpRanges: [],
    aliases: [],
    newDevices: [],
    users: [],
    
    transferData: null,
    history: [],
    historyDiff: '',
    selectedLeases: []
})

export const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use(config => {
    if (store.token) config.headers.Authorization = `Bearer ${store.token}`
    return config
})

export const actions = {
    setToken(newToken) {
        store.token = newToken
        localStorage.setItem('token', newToken)
    },
    logout() {
        store.token = ''
        localStorage.removeItem('token')
        store.view = 'login'
    },
    
    async checkStatus() {
        try {
            const res = await api.get('/status')
            store.isDnsmasqActive = res.data.dnsmasq_active
            if (res.data.setup_required) store.view = 'setup'
            else if (store.token) { store.view = 'dashboard'; this.loadData() }
            else store.view = 'login'
        } catch (e) { store.view = 'error' }
    },

    async loadData() {
        try {
            const [hRes, lRes, aRes, pRes, auditRes, tRes, cfgRes, rangesRes, alRes] = await Promise.all([
                api.get('/hosts'), 
                api.get('/leases'), 
                api.get('/arp'),
                api.get('/plugins').catch(() => ({ data: [] })),
                api.get('/audit').catch(() => ({ data: [] })),
                api.get('/templates').catch(() => ({ data: [] })),
                api.get('/config').catch(() => ({ data: null })),
                api.get('/templates/ranges').catch(() => ({ data: { ranges: [] } })),
                api.get('/aliases').catch(() => ({ data: [] }))
            ])
            store.hosts = hRes.data
            store.leases = lRes.data
            store.arpTable = aRes.data
            store.plugins = pRes.data
            store.auditLog = auditRes.data.reverse()
            store.templates = tRes.data
            store.configSnapshot = cfgRes.data
            store.dhcpRanges = rangesRes.data.ranges || []
            store.aliases = alRes.data
        } catch (e) {
            if (e.response?.status === 401) this.logout()
            else store.view = 'error'
        }
    },

    async loadArp() {
        if (store.view === 'dashboard') {
            try { const res = await api.get('/arp'); store.arpTable = res.data } catch(e) {}
        }
    },

    async applyConfig() {
        try {
            await api.post('/reload')
            alert(t('alert.applySuccess'))
            this.checkStatus()
        } catch (e) {
            if (e.response && e.response.status === 400) alert(translateApiError(e.response.data.error))
            else alert(t('alert.reloadError'))
        }
    },

    async downloadBackup() {
        try {
            const res = await api.get('/backup', { responseType: 'blob' })
            const url = window.URL.createObjectURL(new Blob([res.data]))
            const link = document.createElement('a')
            link.href = url
            link.setAttribute('download', `dnsmasq_backup.zip`)
            document.body.appendChild(link); link.click(); document.body.removeChild(link)
        } catch (e) { alert(t('alert.backupError')) }
    },

    async restartSystem() {
        if(!confirm(t('confirm.restartSystem'))) return
        try {
            await api.post('/restart-self')
            alert(t('alert.restartInProgress'))
            setTimeout(() => location.reload(), 5000)
        } catch (e) { alert(t('alert.restartError')) }
    },

    async loadAudit() {
        try {
            const res = await api.get('/audit')
            store.auditLog = res.data.reverse()
        } catch (e) {}
    },

    async downloadCSV() {
        try {
            const res = await api.get('/hosts/csv', { responseType: 'blob' })
            const url = window.URL.createObjectURL(new Blob([res.data]))
            const link = document.createElement('a')
            link.href = url
            link.setAttribute('download', 'intermasq_hosts.csv')
            document.body.appendChild(link); link.click(); document.body.removeChild(link)
        } catch (e) { alert(t('alert.csvExportError')) }
    },

    async importCSV(file, targetFile) {
        const formData = new FormData()
        formData.append('file', file)
        formData.append('target_file', targetFile)
        try {
            const res = await api.post('/hosts/csv', formData)
            alert(t('alert.csvImportSuccess', { count: res.data.count }))
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.csvImportError')
            alert(msg)
            return false
        }
    },

    async loadTemplates() {
        try {
            const res = await api.get('/templates')
            store.templates = res.data
        } catch (e) {}
    },

    async createTemplate(template) {
        try {
            await api.post('/templates', template)
            this.loadTemplates()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.templateCreateError', 'Failed to create template')
            alert(msg)
            return false
        }
    },

    async deleteTemplate(id) {
        try {
            await api.delete(`/templates/${id}`)
            this.loadTemplates()
            return true
        } catch (e) {
            alert(t('alert.templateDeleteError', 'Failed to delete template'))
            return false
        }
    },

    async applyTemplate(mac, templateId) {
        try {
            const res = await api.post('/hosts/apply-template', { mac, template_id: templateId })
            return res.data
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.templateApplyError', 'Failed to apply template')
            alert(msg)
            return null
        }
    },

    async bulkMove(hosts, target) {
        try {
            const res = await api.post('/hosts/bulk-move', { hosts, target })
            this.loadData()
            return res.data
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkMoveError', 'Failed to move hosts')
            alert(msg)
            return null
        }
    },

    async bulkEdit(hosts, ipTransform, hostnameTransform) {
        try {
            const res = await api.post('/hosts/bulk-edit', { hosts, ip_transform: ipTransform, hostname_transform: hostnameTransform })
            this.loadData()
            return res.data
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkEditError', 'Failed to edit hosts')
            alert(msg)
            return null
        }
    },

    async loadConfig() {
        try {
            const res = await api.get('/config')
            store.configSnapshot = res.data
            const rRes = await api.get('/templates/ranges')
            store.dhcpRanges = rRes.data.ranges || []
        } catch (e) {
            if (e.response?.status === 401) this.logout()
        }
    },

    async saveConfig(file, directives) {
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
    },

    async createConfigFile(name) {
        try {
            const res = await api.post('/config/file', { name })
            store.configSnapshot = res.data
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.configCreateError')
            alert(msg)
            return false
        }
    },

    async loadDhcpRanges() {
        try {
            const res = await api.get('/templates/ranges')
            store.dhcpRanges = res.data.ranges || []
        } catch (e) {}
    },

    async loadAliases() {
        try {
            const res = await api.get('/aliases')
            store.aliases = res.data
        } catch (e) {}
    },

    async addAlias(alias) {
        try {
            await api.post('/aliases', alias)
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasAddError')
            alert(msg)
            return false
        }
    },

    async bulkAddAliases(aliases, file) {
        try {
            const res = await api.post('/aliases/bulk', { aliases, file })
            this.loadData()
            return res.data
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasAddError')
            alert(msg)
            return null
        }
    },

    async deleteAlias(type, domain, file) {
        try {
            await api.post('/aliases/delete', { type, domain, file })
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasDeleteError')
            alert(msg)
            return false
        }
    },

    async downloadAliasesCSV() {
        try {
            const res = await api.get('/aliases/csv', { responseType: 'blob' })
            const url = window.URL.createObjectURL(new Blob([res.data]))
            const link = document.createElement('a')
            link.href = url
            link.setAttribute('download', 'intermasq_aliases.csv')
            document.body.appendChild(link); link.click(); document.body.removeChild(link)
        } catch (e) { alert(t('alert.csvExportError')) }
    },

    async importAliasesCSV(file, targetFile) {
        const formData = new FormData()
        formData.append('file', file)
        if (targetFile) formData.append('target_file', targetFile)
        try {
            const res = await api.post('/aliases/csv', formData)
            alert(t('alert.csvImportSuccess', { count: res.data.count }))
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.csvImportError')
            alert(msg)
            return false
        }
    },

    async loadHistory(file) {
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
    },

    async loadHistoryDiff(file, from, to) {
        try {
            const res = await api.get('/history/diff', { params: { file, from, to } })
            store.historyDiff = res.data.diff || ''
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.historyDiffError')
            alert(msg)
            return false
        }
    },

    async restoreHistory(file, version) {
        try {
            await api.post('/history/restore', { file, version })
            alert(t('alert.restoreSuccess'))
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.restoreError')
            alert(msg)
            return false
        }
    },

    async loadNewDevices() {
        try {
            const res = await api.get('/new-devices')
            store.newDevices = res.data
        } catch (e) {}
    },

    async loadUsers() {
        try {
            const res = await api.get('/users')
            store.users = res.data.users || []
        } catch (e) {}
    },

    async createUser(username, password) {
        try {
            await api.post('/users', { username, password })
            this.loadUsers()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to create user'
            alert(msg)
            return false
        }
    },

    async deleteUser(username) {
        try {
            await api.delete(`/users/${username}`)
            this.loadUsers()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to delete user'
            alert(msg)
            return false
        }
    },

    async changePassword(oldPassword, newPassword) {
        try {
            await api.post('/users/password', { old_password: oldPassword, new_password: newPassword })
            alert(t('alert.passwordChanged'))
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to change password'
            alert(msg)
            return false
        }
    },

    async logoutRequest() {
        try {
            await api.post('/logout')
        } catch (e) {}
        this.logout()
    },

    async bulkLeaseToStatic(leases, file) {
        try {
            const res = await api.post('/leases/to-static', { leases, file })
            alert(t('alert.bulkLeaseToStaticSuccess', { count: res.data.count }))
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.bulkLeaseToStaticError')
            alert(msg)
            return false
        }
    },

    async restoreBackup(file) {
        const formData = new FormData()
        formData.append('file', file)
        try {
            await api.post('/backup/restore', formData)
            alert(t('alert.restoreBackupSuccess'))
            this.loadData()
            return true
        } catch (e) {
            const msg = e.response?.data?.detail || e.response?.data?.error || t('alert.restoreBackupError')
            alert(msg)
            return false
        }
    },

    connectSSE() {
        if (!store.token) return
        const eventSource = new EventSource('/api/events?token=' + encodeURIComponent(store.token))
        eventSource.addEventListener('arp', (e) => {
            try { store.arpTable = JSON.parse(e.data) } catch (_) {}
        })
        eventSource.addEventListener('dnsmasq_status', (e) => {
            try { store.isDnsmasqActive = JSON.parse(e.data).active } catch (_) {}
        })
        eventSource.onerror = () => {}
        return eventSource
    }
}

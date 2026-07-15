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
    
    transferData: null 
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
            const [hRes, lRes, aRes, pRes, auditRes, tRes] = await Promise.all([
                api.get('/hosts'), 
                api.get('/leases'), 
                api.get('/arp'),
                api.get('/plugins').catch(() => ({ data: [] })),
                api.get('/audit').catch(() => ({ data: [] })),
                api.get('/templates').catch(() => ({ data: [] }))
            ])
            store.hosts = hRes.data
            store.leases = lRes.data
            store.arpTable = aRes.data
            store.plugins = pRes.data
            store.auditLog = auditRes.data.reverse()
            store.templates = tRes.data
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
    }
}

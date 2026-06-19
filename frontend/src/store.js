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
            const [hRes, lRes, aRes, pRes] = await Promise.all([
                api.get('/hosts'), 
                api.get('/leases'), 
                api.get('/arp'),
                api.get('/plugins').catch(() => ({ data: [] }))
            ])
            store.hosts = hRes.data
            store.leases = lRes.data
            store.arpTable = aRes.data
            store.plugins = pRes.data
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
    }
}

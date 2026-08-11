// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Intermasq frontend store — reactive state + axios client + core actions
// (auth, status, loadData, SSE, restart, apply config).
//
// Per-domain actions live in ./api/*.js and are merged into the actions
// object below. Each api/*.js module imports store + api from THIS file;
// ES module live bindings make that safe even though the import graph is
// technically circular (store → api/* → store).
//
// Files in ./api/:
//   hosts.js    templates, bulk-move/edit, CSV, lease→static, new devices
//   dns.js      DNS aliases (A/CNAME/PTR/TXT)
//   config.js   dnsmasq config editor, file delete, versioned history
//   system.js   backup/restore, audit, users, logout

import { reactive } from 'vue'
import axios from 'axios'
import { EventSourcePolyfill } from 'event-source-polyfill'
import i18n, { translateApiError } from './i18n.js'

import * as hostsApi from './api/hosts.js'
import * as dnsApi from './api/dns.js'
import * as configApi from './api/config.js'
import * as systemApi from './api/system.js'

const { t } = i18n.global

// decodeRole reads the `role` claim out of the JWT without verifying its
// signature. The backend re-checks role server-side on every admin request
// (AdminMiddleware in internal/auth), so a tampered client-side role only
// buys a misleading UI, not actual privilege — we trust it purely to decide
// which controls to render. Defaults to 'user' for missing/old/malformed
// tokens so a stale session degrades gracefully (admin controls hidden)
// rather than silently escalating.
function decodeRole(token) {
    if (!token) return 'user'
    try {
        let p = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
        while (p.length % 4) p += '='
        return JSON.parse(atob(p)).role || 'user'
    } catch (_) {
        return 'user'
    }
}

export const store = reactive({
    token: localStorage.getItem('token') || '',
    view: 'loading',
    version: '',
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
    configTemplates: [],

    transferData: null,
    history: [],
    historyDiff: '',
    selectedLeases: [],

    // role/isAdmin derive from the JWT `role` claim (see decodeRole). As
    // getters on a reactive object they re-evaluate whenever `token`
    // changes, so templates gating on store.isAdmin refresh on login,
    // logout and role switches without any manual wiring.
    get role() { return decodeRole(this.token) },
    get isAdmin() { return this.role === 'admin' },
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
        // Best-effort server-side JWT revocation (POST /api/logout → jti
        // blacklist). Fire-and-forget: local clear must happen regardless
        // — this is also the path taken on a 401 during loadData, where
        // the token is already invalid and the POST will 4xx harmlessly.
        // Consolidates the old store.logout() (local-only) and the old
        // system.logoutRequest() (POST + clear) into one canonical action.
        api.post('/logout').catch(() => {})
        store.token = ''
        localStorage.removeItem('token')
        store.view = 'login'
    },

    async checkStatus() {
        try {
            const res = await api.get('/status')
            store.isDnsmasqActive = res.data.dnsmasq_active
            store.version = res.data.version || ''
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

    async restartSystem() {
        if(!confirm(t('confirm.restartSystem'))) return
        try {
            await api.post('/restart-self')
            alert(t('alert.restartInProgress'))
            setTimeout(() => location.reload(), 5000)
        } catch (e) { alert(t('alert.restartError')) }
    },

    connectSSE() {
        if (!store.token) return
        // Token is sent via Authorization header instead of ?token= in URL
        // to keep it out of access logs / referrer / browser history.
        const eventSource = new EventSourcePolyfill('/api/events', {
            headers: { Authorization: `Bearer ${store.token}` }
        })
        eventSource.addEventListener('arp', (e) => {
            try { store.arpTable = JSON.parse(e.data) } catch (_) {}
        })
        eventSource.addEventListener('dnsmasq_status', (e) => {
            try { store.isDnsmasqActive = JSON.parse(e.data).active } catch (_) {}
        })
        eventSource.onerror = () => {}
        return eventSource
    },

    // ===== Per-domain actions (merged from ./api/*.js) =====
    ...hostsApi,
    ...dnsApi,
    ...configApi,
    ...systemApi,
}

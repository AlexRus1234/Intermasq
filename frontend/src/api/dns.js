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

// DNS alias actions (A / CNAME / PTR / TXT). Single add, bulk add, delete,
// CSV import/export. Mirrors the host actions shape — success boolean or
// response data, errors surfaced via alert() with translated messages.

import { store, api } from '../store.js'
import i18n, { translateApiError } from '../i18n.js'

const { t } = i18n.global

export async function loadAliases() {
    try {
        const res = await api.get('/aliases')
        store.aliases = res.data
    } catch (e) {}
}

export async function addAlias(alias) {
    try {
        await api.post('/aliases', alias)
        store.aliases = (await api.get('/aliases')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasAddError')
        alert(msg)
        return false
    }
}

export async function bulkAddAliases(aliases, file) {
    try {
        const res = await api.post('/aliases/bulk', { aliases, file })
        store.aliases = (await api.get('/aliases')).data
        return res.data
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasAddError')
        alert(msg)
        return null
    }
}

export async function deleteAlias(type, domain, file) {
    try {
        await api.post('/aliases/delete', { type, domain, file })
        store.aliases = (await api.get('/aliases')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.aliasDeleteError')
        alert(msg)
        return false
    }
}

export async function downloadAliasesCSV() {
    try {
        const res = await api.get('/aliases/csv', { responseType: 'blob' })
        const url = window.URL.createObjectURL(new Blob([res.data]))
        const link = document.createElement('a')
        link.href = url
        link.setAttribute('download', 'intermasq_aliases.csv')
        document.body.appendChild(link); link.click(); document.body.removeChild(link)
    } catch (e) { alert(t('alert.csvExportError')) }
}

export async function importAliasesCSV(file, targetFile) {
    const formData = new FormData()
    formData.append('file', file)
    if (targetFile) formData.append('target_file', targetFile)
    try {
        const res = await api.post('/aliases/csv', formData)
        alert(t('alert.csvImportSuccess', { count: res.data.count }))
        store.aliases = (await api.get('/aliases')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : t('alert.csvImportError')
        alert(msg)
        return false
    }
}

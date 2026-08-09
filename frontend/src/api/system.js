// System-level actions: backup/restore (ZIP), audit log, user CRUD,
// password change, logout, restart-self. Backed by handlers_safety.go,
// handlers_users.go, handlers.go.

import { store, api } from '../store.js'
import i18n, { translateApiError } from '../i18n.js'

const { t } = i18n.global

// ===== ZIP backup / restore =====

export async function downloadBackup() {
    try {
        const res = await api.get('/backup', { responseType: 'blob' })
        const url = window.URL.createObjectURL(new Blob([res.data]))
        const link = document.createElement('a')
        link.href = url
        link.setAttribute('download', `dnsmasq_backup.zip`)
        document.body.appendChild(link); link.click(); document.body.removeChild(link)
    } catch (e) { alert(t('alert.backupError')) }
}

export async function restoreBackup(file) {
    const formData = new FormData()
    formData.append('file', file)
    try {
        await api.post('/backup/restore', formData)
        alert(t('alert.restoreBackupSuccess'))
        // Reload all cached data — dnsmasq configs may have changed entirely.
        store.hosts = (await api.get('/hosts')).data
        store.aliases = (await api.get('/aliases')).data
        store.configSnapshot = (await api.get('/config')).data
        return true
    } catch (e) {
        const msg = e.response?.data?.detail || e.response?.data?.error || t('alert.restoreBackupError')
        alert(msg)
        return false
    }
}

// ===== Audit log =====

export async function loadAudit() {
    try {
        const res = await api.get('/audit')
        store.auditLog = res.data.reverse()
    } catch (e) {}
}

// ===== Users =====

export async function loadUsers() {
    try {
        const res = await api.get('/users')
        store.users = res.data.users || []
    } catch (e) {}
}

export async function createUser(username, password) {
    try {
        await api.post('/users', { username, password })
        await loadUsers()
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to create user'
        alert(msg)
        return false
    }
}

export async function deleteUser(username) {
    try {
        await api.delete(`/users/${username}`)
        await loadUsers()
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to delete user'
        alert(msg)
        return false
    }
}

export async function changePassword(oldPassword, newPassword) {
    try {
        await api.post('/users/password', { old_password: oldPassword, new_password: newPassword })
        alert(t('alert.passwordChanged'))
        return true
    } catch (e) {
        const msg = e.response?.data?.error ? translateApiError(e.response.data.error) : 'Failed to change password'
        alert(msg)
        return false
    }
}

// ===== Session =====
//
// Logout lives in store.js (actions.logout) — it POSTs /api/logout
// (best-effort revocation) and clears local state. It used to be split
// between store.logout() (local clear, used on 401) and a duplicate
// logoutRequest() here (POST + clear, wired to the menu). The single
// canonical action is imported everywhere via `actions`.

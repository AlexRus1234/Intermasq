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

import { reactive } from 'vue'

const state = reactive({
    toasts: []
})

let nextId = 0

function show(message, type = 'info', timeout = 5000) {
    const id = nextId++
    state.toasts.push({ id, message, type })
    if (timeout > 0) {
        setTimeout(() => remove(id), timeout)
    }
    return id
}

function remove(id) {
    const idx = state.toasts.findIndex(t => t.id === id)
    if (idx > -1) state.toasts.splice(idx, 1)
}

export const toast = {
    state,
    show,
    remove,
    success: (msg) => show(msg, 'success'),
    error: (msg) => show(msg, 'danger', 8000),
    warning: (msg) => show(msg, 'warning', 6000),
    info: (msg) => show(msg, 'info'),
}

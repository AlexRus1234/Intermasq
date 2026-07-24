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

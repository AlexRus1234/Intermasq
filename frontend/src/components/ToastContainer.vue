<script setup>
import { toast } from '../toast.js'

const icons = {
    success: '✅',
    danger: '⚠️',
    warning: '⚡',
    info: 'ℹ️',
}
</script>

<template>
    <div class="toast-container position-fixed top-0 end-0 p-3" style="z-index: 9999">
        <div v-for="t in toast.state.toasts" :key="t.id"
             class="toast show shadow-sm" role="alert">
            <div class="toast-header" :class="'text-bg-' + t.type">
                <strong class="me-auto">{{ icons[t.type] || 'ℹ️' }} Intermasq</strong>
                <button type="button" class="btn-close btn-close-white" @click="toast.remove(t.id)"></button>
            </div>
            <div class="toast-body" style="white-space: pre-wrap;">
                {{ t.message }}
            </div>
        </div>
    </div>
</template>

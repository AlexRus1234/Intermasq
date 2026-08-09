<!--
Intermasq - Web panel for dnsmasq
Copyright (C) 2026 AlexRus1234

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
-->

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

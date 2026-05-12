<template>
    <div class="row g-2 fade-in">
        <div class="col-12">
            <textarea v-model="text" class="form-control text-monospace" rows="4" placeholder="MAC IP Hostname (каждое с новой строки)"></textarea>
        </div>
        <div class="col-12 d-flex justify-content-between align-items-center">
            <span class="text-muted small">Распознано: {{ parsedCount }}</span>
            <div class="input-group" style="width: auto;">
                <input v-model="form.file" :readonly="selectedFile !== 'all'" :class="['form-control', selectedFile !== 'all' ? 'bg-light' : '']" placeholder="Файл назначения">
                <button @click="$emit('import', text)" class="btn btn-success" :disabled="parsedCount === 0">Импорт</button>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, computed } from 'vue'
const props = defineProps(['form', 'selectedFile'])
defineEmits(['import'])

const text = ref('')
const parsedCount = computed(() => {
    return text.value.split('\n').filter(line => /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})/i.test(line.trim())).length
})
</script>

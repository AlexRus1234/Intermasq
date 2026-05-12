<template>
  <div class="row justify-content-center mt-5">
    <div class="col-12 col-md-6 col-lg-4" style="min-width: 320px;">
      <div class="card shadow">
        <div class="card-body p-4">
          <h4 class="mb-4 text-center fw-bold">{{ store.view === 'setup' ? 'Настройка Администратора' : 'Вход в систему' }}</h4>
          
          <input v-model="username" class="form-control mb-3" placeholder="Логин">
          <input v-model="password" type="password" class="form-control mb-4" placeholder="Пароль" @keyup.enter="submit">
          
          <button @click="submit" class="btn btn-primary w-100 fw-bold">
            {{ store.view === 'setup' ? 'Создать аккаунт' : 'Войти' }}
          </button>
          
          <p v-if="error" class="text-danger mt-3 mb-0 text-center">{{ error }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { store, api, actions } from '../store.js'

const username = ref('')
const password = ref('')
const error = ref('')

async function submit() {
  if(!username.value || !password.value) { error.value = "Введите логин и пароль"; return }
  try {
    const endpoint = store.view === 'setup' ? '/setup' : '/login'
    const res = await api.post(endpoint, { username: username.value, password: password.value })
    
    // Сохраняем токен и загружаем панель
    store.token = res.data.token
    localStorage.setItem('token', res.data.token)
    store.view = 'dashboard'
    actions.loadData()
  } catch (e) { 
    error.value = store.view === 'setup' ? (e.response?.data?.error || "Ошибка настройки") : "Неверный логин или пароль" 
  }
}
</script>

<template>
  <div class="fade-in">
      <div class="card shadow-sm mb-4">
          <div class="card-header bg-secondary text-white fw-bold">{{ $t('users.changePassword') }}</div>
          <div class="card-body">
              <div class="row g-2">
                  <div class="col-md-4">
                      <input v-model="pwForm.old" type="password" class="form-control" :placeholder="$t('users.oldPassword')">
                  </div>
                  <div class="col-md-4">
                      <input v-model="pwForm.new" type="password" class="form-control" :placeholder="$t('users.newPassword')">
                  </div>
                  <div class="col-md-4">
                      <button @click="changePassword" class="btn btn-primary w-100">{{ $t('users.change') }}</button>
                  </div>
              </div>
          </div>
      </div>

      <div class="card shadow-sm mb-4">
          <div class="card-header bg-primary text-white fw-bold">{{ $t('users.createUser') }}</div>
          <div class="card-body">
              <div class="row g-2">
                  <div class="col-md-4">
                      <input v-model="newUser.username" type="text" class="form-control" :placeholder="$t('auth.username')">
                  </div>
                  <div class="col-md-4">
                      <input v-model="newUser.password" type="password" class="form-control" :placeholder="$t('auth.password')">
                  </div>
                  <div class="col-md-4">
                      <button @click="createUser" class="btn btn-success w-100">+ {{ $t('users.create') }}</button>
                  </div>
              </div>
          </div>
      </div>

      <div class="card shadow-sm">
          <div class="card-header bg-dark text-white d-flex justify-content-between align-items-center">
              <span class="fw-bold">{{ $t('users.title') }}</span>
              <span class="badge bg-light text-dark">{{ store.users.length }} {{ $t('users.count') }}</span>
          </div>
          <table class="table table-hover mb-0" v-if="store.users.length > 0">
              <thead class="table-light">
                  <tr>
                      <th>{{ $t('auth.username') }}</th>
                      <th class="text-end">{{ $t('hosts.actions') }}</th>
                  </tr>
              </thead>
              <tbody>
                  <tr v-for="u in store.users" :key="u">
                      <td class="font-monospace fw-bold">{{ u }}</td>
                      <td class="text-end">
                          <button @click="deleteUser(u)" class="btn btn-sm btn-outline-danger fw-bold">
                              🗑 {{ $t('hosts.deleteTooltip') }}
                          </button>
                      </td>
                  </tr>
              </tbody>
          </table>
          <div v-else class="text-center p-5 text-muted">
              {{ $t('users.empty') }}
          </div>
      </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { store, actions } from '../store.js'

const pwForm = reactive({ old: '', new: '' })
const newUser = reactive({ username: '', password: '' })

async function changePassword() {
    if (!pwForm.old || !pwForm.new) return
    await actions.changePassword(pwForm.old, pwForm.new)
    pwForm.old = ''
    pwForm.new = ''
}

async function createUser() {
    if (!newUser.username || !newUser.password) return
    const ok = await actions.createUser(newUser.username, newUser.password)
    if (ok) { newUser.username = ''; newUser.password = '' }
}

async function deleteUser(username) {
    if (!confirm(username + ' will be deleted. Continue?')) return
    await actions.deleteUser(username)
}
</script>

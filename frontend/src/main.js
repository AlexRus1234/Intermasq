import { createApp } from 'vue'
import './style.css'
import 'bootstrap/dist/css/bootstrap.min.css'
import 'bootstrap/dist/js/bootstrap.bundle.min.js'
import i18n from './i18n'
import App from './App.vue'

createApp(App).use(i18n).mount('#app')

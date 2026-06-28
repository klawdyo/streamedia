import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './styles/global.css'
import { useNavigationGuard } from '@/composables/useNavigationGuard'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)

// Registra o guard de navegação antes de montar
useNavigationGuard(router)

app.mount('#app')

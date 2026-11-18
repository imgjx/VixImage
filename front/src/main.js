import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import './style.css'

// 会话失效：跳回服务端登录页（原生表单登录，保持无 JS 可用）。
window.addEventListener('vix-unauthorized', () => {
  try { localStorage.removeItem('vix_sid') } catch (e) {}
  if (location.pathname !== '/login') location.href = '/login'
})

createApp(App).use(router).use(i18n).mount('#app')

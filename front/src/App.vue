<template>
  <div>
    <div class="top">
      <div class="logo">Vix<span>Image</span></div>
      <nav class="nav">
        <router-link v-for="n in navs" :key="n.path" :to="n.path"
          :class="{ on: $route.path === n.path || (n.path === '/' && $route.path === '/overview') }">
          {{ t(n.label) }}
        </router-link>
      </nav>
      <div class="right">
        <button class="lang-btn ghost" @click="toggleLang">{{ locale === 'zh' ? 'EN' : '中文' }}</button>
        <span>{{ user }}</span>
        <button class="sm" @click="logout">{{ t('common.logout') }}</button>
      </div>
    </div>
    <div class="wrap">
      <router-view @flash="flash = $event" />
    </div>
    <div v-if="flash" class="mask" @click.self="flash=''">
      <div class="modal" style="max-width:420px">
        <p style="white-space:pre-wrap">{{ flash }}</p>
        <div class="form-actions"><button class="primary" @click="flash=''">{{ t('common.confirm') }}</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, provide } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from './api'
import { setLang } from './i18n'

const { t, locale } = useI18n()
const user = ref('')
const flash = ref('')
provide('flash', (msg) => { flash.value = msg })

const navs = [
  { path: '/', label: 'nav.overview' },
  { path: '/mymodels', label: 'nav.mymodels' },
  { path: '/upstream', label: 'nav.upstream' },
  { path: '/playground', label: 'nav.playground' },
  { path: '/apikeys', label: 'nav.apikeys' },
  { path: '/config', label: 'nav.config' }
]

function toggleLang() {
  const next = locale.value === 'zh' ? 'en' : 'zh'
  setLang(next)
}
async function logout() {
  try { await api('/api/logout', { method: 'POST' }) } catch (e) {}
  try { localStorage.removeItem('vix_sid') } catch (e) {}
  location.href = '/login'
}
onMounted(async () => {
  try { const st = await api('/api/state'); user.value = st.username } catch (e) {}
})
</script>

<template>
  <div>
    <div class="page-head"><h1>{{ t('overview.title') }}</h1></div>
    <div class="cards">
      <div class="card" v-for="c in cards" :key="c.label"><div class="n">{{ c.value }}</div><div class="l">{{ c.label }}</div></div>
    </div>
    <div class="grid2" style="margin-top:18px">
      <div class="panel">
        <h3>{{ t('overview.quickStart') }}</h3>
        <ul style="margin:0;padding-left:18px;line-height:1.8;color:#cdd9f0">
          <li>{{ t('overview.intro1') }}</li>
          <li>{{ t('overview.intro2') }}</li>
          <li>{{ t('overview.intro3') }}</li>
          <li>{{ t('overview.intro4') }}</li>
          <li>{{ t('overview.intro5') }}</li>
          <li>{{ t('overview.intro6') }}</li>
        </ul>
      </div>
      <div class="panel">
        <h3>{{ t('overview.runMode') }}</h3>
        <ul style="margin:0;padding-left:18px;line-height:1.9;color:#cdd9f0">
          <li>{{ t('overview.storage') }}: <code class="mono">{{ state.storage }}</code></li>
          <li>{{ t('overview.cache') }}: <code class="mono">{{ state.cache }}</code></li>
          <li>{{ t('overview.guiport') }}: <code class="mono">{{ state.gui_port }}</code></li>
          <li>{{ t('overview.apiport') }}: <code class="mono">{{ state.api_port }}</code></li>
        </ul>
        <p class="hint" style="margin-top:14px">API · <code class="mono">POST /v1/agents/run</code> · <code class="mono">X-API-Key</code></p>
      </div>
    </div>
    <div class="panel" style="margin-top:18px">
      <div class="bar">
        <h3 style="margin:0">{{ t('overview.title') }}</h3>
        <div class="spacer"></div>
        <label style="margin:0;display:flex;align-items:center;gap:6px;color:var(--muted)">{{ t('overview.pageSize') }}
          <select v-model.number="size" @change="page=1;load()" style="width:auto;padding:5px 8px">
            <option :value="10">10</option><option :value="20">20</option><option :value="50">50</option>
          </select>
        </label>
        <button class="sm" @click="load">{{ t('common.refresh') }}</button>
      </div>
      <div style="overflow:auto">
      <table>
        <thead><tr>
          <th>ID</th><th>{{ t('overview.agent') }}</th><th>{{ t('overview.prompt') }}</th>
          <th>{{ t('overview.status') }}</th><th>{{ t('overview.images') }}</th>
          <th>{{ t('overview.stepsUsed') }}</th><th>{{ t('overview.duration') }}</th>
          <th>{{ t('overview.created') }}</th><th>{{ t('common.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="l in logs" :key="l.id">
            <td class="mono">{{ l.id }}</td>
            <td>{{ l.agent_call_name }}</td>
            <td><div class="wrap-text">{{ l.prompt }}</div></td>
            <td><span class="tag" :class="statusClass(l.status)">{{ statusLabel(l.status) }}</span></td>
            <td>{{ l.output_images }}</td>
            <td>{{ l.steps_used||0 }}</td>
            <td>{{ l.duration_ms }}ms</td>
            <td class="mono">{{ fmtTime(l.created_at) }}</td>
            <td><button class="sm ghost" @click="showDetail(l)">{{ t('overview.detail') }}</button></td>
          </tr>
          <tr v-if="!logs.length"><td colspan="9" class="empty">{{ t('overview.noLogs') }}</td></tr>
        </tbody>
      </table>
      </div>
      <div class="bar" style="margin-top:14px;margin-bottom:0">
        <button class="sm" :disabled="page<=1" @click="page--;load()">{{ t('overview.prev') }}</button>
        <span class="muted">{{ page }} {{ t('overview.of') }} {{ pages }}</span>
        <button class="sm" :disabled="page>=pages" @click="page++;load()">{{ t('overview.next') }}</button>
        <div class="spacer"></div><span class="muted">Total {{ total }}</span>
      </div>
    </div>

    <div v-if="detail" class="mask" @click.self="detail=null">
      <div class="modal">
        <h2>{{ t('overview.detail') }} #{{ detail.id }}</h2>
        <div class="sub">{{ detail.agent_call_name }} · {{ fmtTime(detail.created_at) }}</div>
        <p v-if="detail.theme"><b>{{ t('overview.theme') }}:</b> {{ detail.theme }}</p>
        <p v-if="detail.error" class="err"><b>{{ t('overview.error') }}:</b> {{ detail.error }}</p>
        <h3>{{ t('overview.thinking') }}</h3>
        <div v-if="traceList.length">
          <div class="trace-item" v-for="s in traceList" :key="s.step"><b>#{{ s.step }}</b> {{ s.think }} <span class="muted mono" v-if="s.ops">— {{ s.ops }}</span></div>
        </div>
        <p class="muted" v-else>{{ t('overview.noDetail') }}</p>
        <div class="form-actions"><button class="primary" @click="detail=null">{{ t('common.close') }}</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'

const { t } = useI18n()
const state = ref({}), stats = ref({}), logs = ref([])
const page = ref(1), size = ref(20), pages = ref(1), total = ref(0)
const detail = ref(null)
const cards = computed(() => [
  { label: t('overview.total'), value: stats.value.total||0 },
  { label: t('overview.success'), value: stats.value.success||0 },
  { label: t('overview.failed'), value: stats.value.failed||0 },
  { label: t('overview.running'), value: stats.value.running||0 },
  { label: t('overview.outputs'), value: stats.value.outputs||0 },
  { label: t('overview.steps'), value: stats.value.steps_used||0 }
])
const traceList = computed(() => {
  if (!detail.value || !detail.value.trace) return []
  try { return typeof detail.value.trace === 'string' ? JSON.parse(detail.value.trace) : detail.value.trace } catch (e) { return [] }
})
async function load() {
  try {
    const ov = await api(`/api/overview?page=${page.value}&size=${size.value}`)
    stats.value = ov.stats||{}; logs.value = ov.logs||[]
    page.value = ov.page||1; pages.value = ov.pages||1; total.value = ov.total||0
  } catch (e) {}
}
async function loadState() {
  try { state.value = await api('/api/state') } catch (e) {}
}
function showDetail(l) { detail.value = l }
function statusClass(s){ return s==='success'?'ok':(s==='failed'?'bad':(s==='success_partial'?'warn':'')) }
function statusLabel(s){ return s==='success_partial'?t('overview.successPartial'):(s||'—') }
function fmtTime(v){ if(!v) return '—'; return String(v).replace('T',' ').slice(0,19) }
let timer
onMounted(async () => { await Promise.all([load(), loadState()]); timer = setInterval(load, 10000) })
onUnmounted(() => clearInterval(timer))
</script>

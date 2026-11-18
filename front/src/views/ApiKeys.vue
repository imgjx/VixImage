<template>
  <div>
    <div class="page-head">
      <h1>{{ t('apikeys.title') }}</h1>
      <p>{{ t('apikeys.desc') }}</p>
    </div>
    <div class="panel">
      <div class="bar">
        <div class="spacer"></div>
        <button class="primary" @click="openAdd">{{ t('common.create') }} {{ t('apikeys.title') }}</button>
      </div>
      <div style="overflow:auto">
      <table>
        <thead><tr>
          <th>{{ t('apikeys.key') }}</th><th>{{ t('apikeys.name') }}</th>
          <th>{{ t('apikeys.calls') }}</th><th>{{ t('apikeys.lastUsed') }}</th>
          <th>{{ t('apikeys.status') }}</th><th>{{ t('common.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="k in keys" :key="k.id">
            <td class="mono">{{ k.key_mask }}</td>
            <td>{{ k.name||'—' }}</td>
            <td>{{ k.calls }}</td>
            <td class="mono">{{ fmt(k.last_used_at) }}</td>
            <td><span class="tag" :class="k.enabled===false?'':'ok'">{{ k.enabled===false?t('common.disabled'):t('common.enabled') }}</span></td>
            <td><button class="sm danger" @click="del(k)">{{ t('common.delete') }}</button></td>
          </tr>
          <tr v-if="!keys.length"><td colspan="6" class="empty">{{ t('apikeys.noData') }}</td></tr>
        </tbody>
      </table>
      </div>
    </div>

    <div v-if="modal" class="mask" @click.self="modal=false">
      <div class="modal">
        <h2>{{ t('apikeys.addTitle') }}</h2>
        <div class="sub">{{ t('apikeys.createdTip') }}</div>
        <label>{{ t('apikeys.name') }}</label>
        <input v-model="form.name" />
        <label>{{ t('apikeys.customKey') }}</label>
        <input v-model="form.key" />
        <div class="form-actions">
          <button class="primary" @click="save">{{ t('common.create') }}</button>
          <button @click="modal=false">{{ t('common.cancel') }}</button>
        </div>
        <div v-if="newKey" class="panel" style="margin-top:16px">
          <b>{{ t('apikeys.key') }}:</b> <code class="mono">{{ newKey }}</code>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
const { t } = useI18n()
const flash = inject('flash')
const keys = ref([])
const modal = ref(false), newKey = ref('')
const form = ref({ name:'', key:'' })
async function load(){ try { keys.value = (await api('/api/apikeys')).keys||[] } catch(e){} }
function openAdd(){ form.value={name:'',key:''}; newKey.value=''; modal.value=true }
async function save(){
  try { const r = await api('/api/apikeys',{method:'POST',body:{name:form.value.name,key:form.value.key}})
    newKey.value = r.key||''; await load(); flash(t('common.saved'))
  } catch(e){ flash(e.message) }
}
async function del(k){
  if(!confirm(t('apikeys.confirmDelete'))) return
  try { await api('/api/apikeys/'+k.id,{method:'DELETE'}); await load() } catch(e){ flash(e.message) }
}
function fmt(v){ return v?String(v).replace('T',' ').slice(0,19):'—' }
onMounted(load)
</script>

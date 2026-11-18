<template>
  <div>
    <div class="page-head"><h1>{{ t('mymodels.title') }}</h1><p>{{ t('mymodels.desc') }}</p></div>
    <div class="panel">
      <div class="bar"><div class="spacer"></div><button class="primary" @click="openAdd">{{ t('common.create') }} {{ t('mymodels.title') }}</button></div>
      <div style="overflow:auto">
      <table>
        <thead><tr>
          <th>ID</th><th>{{ t('mymodels.name') }}</th><th>{{ t('mymodels.callName') }}</th>
          <th>{{ t('mymodels.baseModel') }}</th><th>{{ t('mymodels.maxSteps') }}</th>
          <th>{{ t('mymodels.imageModels') }}</th><th>{{ t('mymodels.style') }}</th>
          <th>{{ t('common.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="m in list" :key="m.id">
            <td class="mono">{{ m.id }}</td><td>{{ m.name }}</td><td class="mono">{{ m.call_name }}</td>
            <td class="mono">{{ baseName(m.base_model_id) }}</td><td>{{ m.max_steps }}</td>
            <td><span class="tag" v-for="im in m.image_models" :key="im.upstream_id" style="margin-right:4px">{{ engineName(im.upstream_id) }} ×{{ im.steps }}</span></td>
            <td>{{ styleLabel(m.style) }}</td>
            <td>
              <button class="sm ghost" @click="openEdit(m)">{{ t('common.edit') }}</button>
              <button class="sm danger" @click="del(m)">{{ t('common.delete') }}</button>
            </td>
          </tr>
          <tr v-if="!list.length"><td colspan="8" class="empty">{{ t('mymodels.noData') }}</td></tr>
        </tbody>
      </table>
      </div>
    </div>

    <div v-if="modal" class="mask" @click.self="modal=false">
      <div class="modal">
        <h2>{{ editing?t('mymodels.editTitle'):t('mymodels.addTitle') }}</h2>
        <div class="row">
          <div><label>{{ t('mymodels.name') }}</label><input v-model="f.name" /></div>
          <div><label>{{ t('mymodels.callName') }}</label><input v-model="f.call_name" /></div>
        </div>
        <div class="row">
          <div><label>{{ t('mymodels.baseModel') }}</label>
            <select v-model.number="f.base_model_id"><option :value="0">—</option><option v-for="b in llms" :key="b.id" :value="b.id">{{ b.name }}</option></select>
          </div>
          <div><label>{{ t('mymodels.maxSteps') }}</label><input v-model.number="f.max_steps" type="number" /></div>
        </div>
        <label>{{ t('mymodels.prompt') }}</label><textarea v-model="f.prompt"></textarea>

        <h3 style="margin-top:16px">{{ t('mymodels.imageModels') }}</h3>
        <div v-for="(im,i) in f.image_models" :key="i" class="row" style="align-items:end;margin-bottom:8px">
          <div><label>{{ t('mymodels.imageModels') }}</label>
            <select v-model.number="im.upstream_id"><option v-for="im2 in images" :key="im2.id" :value="im2.id">{{ im2.name }}</option></select>
          </div>
          <div><label>{{ t('mymodels.stepsPerGen') }}</label><input v-model.number="im.steps" type="number" /></div>
          <div style="flex:0 0 auto"><button class="danger sm" @click="f.image_models.splice(i,1)">{{ t('common.delete') }}</button></div>
        </div>
        <button class="sm" @click="f.image_models.push({upstream_id:(images[0]||{}).id||0,steps:30})">+ {{ t('mymodels.addEngine') }}</button>

        <div class="row" style="margin-top:12px">
          <div><label>{{ t('mymodels.style') }}</label>
            <select v-model="f.style">
              <option value="on_demand">{{ t('mymodels.styleOnDemand') }}</option>
              <option value="creative">{{ t('mymodels.styleCreative') }}</option>
              <option value="cinematic">{{ t('mymodels.styleCinematic') }}</option>
            </select>
          </div>
          <div><label>{{ t('mymodels.exhaust') }}</label>
            <select v-model="f.exhaust">
              <option value="fail">{{ t('mymodels.exhaustFail') }}</option>
              <option value="partial">{{ t('mymodels.exhaustPartial') }}</option>
            </select>
          </div>
        </div>
        <div class="check"><input id="learn" type="checkbox" :checked="f.learning==='on'" @change="f.learning=$event.target.checked?'on':'off'" /><label for="learn">{{ t('mymodels.learning') }}</label></div>
        <div class="check"><input id="deep" type="checkbox" v-model="f.deep_think" /><label for="deep">{{ t('mymodels.deepThink') }}</label></div>

        <div class="form-actions">
          <button class="primary" @click="save">{{ t('common.save') }}</button>
          <button @click="modal=false">{{ t('common.cancel') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
const { t, locale } = useI18n()
const flash = inject('flash')
const list = ref([]), upstream = ref([])
const llms = computed(()=>upstream.value.filter(m=>m.type==='llm'))
const images = computed(()=>upstream.value.filter(m=>m.type==='image'))
const modal = ref(false), editing = ref(false)
const blank = () => ({ name:'', call_name:'', base_model_id:0, max_steps:100, prompt:'',
  style:'on_demand', exhaust:'fail', learning:'on', deep_think:false, image_models:[] })
const f = ref(blank())
async function load(){
  try {
    list.value = (await api('/api/agents')).agents||[]
    upstream.value = (await api('/api/upstream')).upstream||[]
  } catch(e){}
}
function openAdd(){ editing.value=false; f.value=blank(); modal.value=true }
function openEdit(m){
  editing.value=true
  f.value = Object.assign(blank(), JSON.parse(JSON.stringify(m)))
  if(!f.value.image_models) f.value.image_models=[]
  modal.value=true
}
async function save(){
  try {
    const body = Object.assign({}, f.value)
    if(editing.value) await api('/api/agents/'+f.value.id,{method:'PUT',body})
    else await api('/api/agents',{method:'POST',body})
    await load(); modal.value=false; flash(t('common.saved'))
  } catch(e){ flash(e.message) }
}
async function del(m){
  if(!confirm(t('mymodels.confirmDelete'))) return
  try { await api('/api/agents/'+m.id,{method:'DELETE'}); await load() } catch(e){ flash(e.message) }
}
function baseName(id){ const b=upstream.value.find(x=>x.id===id); return b?b.name:id }
function engineName(id){ const e=upstream.value.find(x=>x.id===id); return e?e.name:id }
function styleLabel(s){ return t('mymodels.style'+({on_demand:'OnDemand',creative:'Creative',cinematic:'Cinematic'}[s]||'OnDemand')) }
onMounted(load)
</script>

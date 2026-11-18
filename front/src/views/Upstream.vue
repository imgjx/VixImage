<template>
  <div>
    <div class="page-head">
      <h1>{{ t('upstream.title') }}</h1>
      <p>{{ t('upstream.desc') }}</p>
    </div>
    <div class="panel">
      <div class="bar">
        <div class="spacer"></div>
        <button class="primary" @click="openAdd">{{ t('common.add') }} {{ t('upstream.title') }}</button>
      </div>
      <div style="overflow:auto">
      <table>
        <thead><tr>
          <th>ID</th><th>{{ t('upstream.name') }}</th><th>{{ t('upstream.type') }}</th>
          <th>{{ t('upstream.format') }}</th><th>{{ t('upstream.modelName') }}</th>
          <th>{{ t('common.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="m in list" :key="m.id">
            <td class="mono">{{ m.id }}</td>
            <td>{{ m.name }}</td>
            <td><span class="tag">{{ m.type==='image'?t('upstream.typeImage'):t('upstream.typeLLM') }}</span></td>
            <td class="mono">{{ m.format }}</td>
            <td class="mono">{{ m.model_name }}</td>
            <td>
              <button class="sm ghost" @click="openEdit(m)">{{ t('common.edit') }}</button>
              <button class="sm danger" @click="del(m)">{{ t('common.delete') }}</button>
            </td>
          </tr>
          <tr v-if="!list.length"><td colspan="6" class="empty">{{ t('upstream.noData') }}</td></tr>
        </tbody>
      </table>
      </div>
    </div>

    <div v-if="modal" class="mask" @click.self="modal=false">
      <div class="modal">
        <h2>{{ editing?t('upstream.editTitle'):t('upstream.addTitle') }}</h2>
        <div class="row">
          <div><label>{{ t('upstream.name') }}</label><input v-model="f.name" /></div>
          <div><label>{{ t('upstream.type') }}</label>
            <select v-model="f.type" @change="onTypeChange"><option value="image">{{ t('upstream.typeImage') }}</option><option value="llm">{{ t('upstream.typeLLM') }}</option></select>
          </div>
        </div>
        <div class="row">
          <div><label>{{ t('upstream.format') }}</label>
            <select v-model="f.format">
              <template v-if="f.type==='image'">
                <option value="openai">OpenAI Images</option><option value="gemini">Gemini</option><option value="stability">Stability AI</option>
              </template>
              <template v-else>
                <option value="openai_chat">OpenAI Chat (stream)</option><option value="gemini">Gemini</option><option value="anthropic">Anthropic Claude</option>
              </template>
            </select>
          </div>
          <div><label>{{ t('upstream.modelName') }}</label><input v-model="f.model_name" /></div>
        </div>
        <label>{{ t('upstream.baseUrl') }}</label><input v-model="f.base_url" placeholder="https://api.example.com" />
        <label>{{ t('upstream.apiKey') }}</label><input v-model="f.api_key" type="password" :placeholder="editing?t('upstream.maskedKeyTip'):''" />

        <template v-if="f.type==='image'">
          <div class="row">
            <div><label>{{ t('upstream.size') }}</label><input v-model="f.size" placeholder="1024x1024" /></div>
            <div><label>{{ t('upstream.sizes') }}</label><input v-model="sizesText" placeholder="1024x1024, 720x1280" /></div>
          </div>
          <div class="row">
            <div><label>{{ t('upstream.n') }}</label><input v-model.number="f.n" type="number" /></div>
            <div><label>{{ t('upstream.steps') }}</label><input v-model.number="f.steps" type="number" /></div>
          </div>
          <label>{{ t('upstream.negPrompt') }}</label><input v-model="f.negative_prompt" />
          <label>{{ t('upstream.agentDesc') }}</label><textarea v-model="f.agent_desc"></textarea>
        </template>
        <template v-else>
          <div class="row">
            <div><label>{{ t('upstream.maxTokens') }}</label><input v-model.number="f.max_tokens" type="number" /></div>
            <div><label>{{ t('upstream.contextLimit') }}</label><input v-model.number="f.context_limit" type="number" /></div>
          </div>
          <div class="row">
            <div><label>{{ t('upstream.temperature') }}</label><input v-model.number="f.temperature" type="number" step="0.1" /></div>
            <div><label>{{ t('upstream.topP') }}</label><input v-model.number="f.top_p" type="number" step="0.05" /></div>
          </div>
          <label>{{ t('upstream.systemPrompt') }}</label><textarea v-model="f.system_prompt"></textarea>
          <div class="check"><input id="mm" type="checkbox" v-model="f.multimodal" /><label for="mm">{{ t('upstream.multimodal') }}</label></div>
        </template>

        <div class="check"><input id="en" type="checkbox" v-model="f.enabled" /><label for="en">{{ t('upstream.enabled') }}</label></div>
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
const { t } = useI18n()
const flash = inject('flash')
const list = ref([])
const modal = ref(false), editing = ref(false)
const sizesText = ref('')
const blank = () => ({ name:'', type:'image', format:'openai', base_url:'', api_key:'', model_name:'',
  multimodal:true, enabled:true, size:'', n:1, steps:30, negative_prompt:'', agent_desc:'',
  max_tokens:0, context_limit:0, temperature:0, top_p:0, system_prompt:'' })
const f = ref(blank())
async function load(){ try { list.value = (await api('/api/upstream')).upstream||[] } catch(e){} }
function onTypeChange(){ f.value.format = f.value.type==='image' ? 'openai' : 'openai_chat' }
function openAdd(){ editing.value=false; f.value=blank(); sizesText.value=''; modal.value=true }
function openEdit(m){
  editing.value=true
  f.value = Object.assign(blank(), m)
  if(!f.value.api_key) f.value.api_key='' // masked value kept; blank = no change
  if(f.value.type==='image' && f.value.api_key) f.value.api_key='' // 掩码不可回写
  sizesText.value = (m.sizes||[]).join(', ')
  modal.value=true
}
async function save(){
  const body = Object.assign({}, f.value)
  if(f.value.type==='image'){
    body.multimodal = true
    body.sizes = sizesText.value.split(',').map(s=>s.trim()).filter(Boolean)
  }
  try { await api('/api/upstream',{method:'POST',body}); await load(); modal.value=false; flash(t('common.saved')) }
  catch(e){ flash(e.message) }
}
async function del(m){
  if(!confirm(t('upstream.confirmDelete'))) return
  try { await api('/api/upstream/'+m.id,{method:'DELETE'}); await load() } catch(e){ flash(e.message) }
}
onMounted(load)
</script>

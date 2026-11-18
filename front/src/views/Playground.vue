<template>
  <div>
    <div class="page-head"><h1>{{ t('playground.title') }}</h1><p>{{ t('playground.desc') }}</p></div>
    <div class="panel">
      <div class="row">
        <div><label>{{ t('playground.agent') }}</label>
          <select v-model.number="agentId"><option :value="0">— {{ t('playground.selectAgent') }} —</option><option v-for="a in agents" :key="a.id" :value="a.id">{{ a.name }} ({{ a.call_name }})</option></select>
        </div>
        <div><label>{{ t('playground.size') }} ({{ t('common.optional') }})</label><input v-model="size" placeholder="1080x1920" /></div>
      </div>
      <label>{{ t('playground.prompt') }}</label>
      <textarea v-model="prompt" :placeholder="t('playground.promptPh')" style="min-height:90px"></textarea>
      <div class="check"><input id="deep" type="checkbox" v-model="deep" /><label for="deep">{{ t('playground.deepThink') }}</label></div>
      <label>{{ t('playground.refs') }} ({{ t('common.optional') }})</label>
      <input type="file" multiple accept="image/*" @change="onFiles" />
      <p class="hint">{{ t('playground.uploadTip') }}</p>
      <div class="form-actions">
        <button class="primary" @click="run" :disabled="running">{{ running?t('playground.running'):t('playground.run') }}</button>
      </div>
    </div>

    <div v-if="result" class="panel">
      <h3>{{ t('playground.result') }}</h3>
      <p class="muted">{{ t('playground.status') }}: <b>{{ result.status }}</b> · {{ t('playground.steps') }}: {{ result.steps_used }} · {{ t('playground.llm') }}: {{ result.llm_calls }} <template v-if="result.theme"> · {{ t('playground.theme') }}: {{ result.theme }}</template></p>
      <p v-if="result.error" class="err">{{ result.error }}</p>
      <h3 style="margin-top:8px">{{ t('playground.images') }}</h3>
      <div v-if="result.images && result.images.length" class="thumbs">
        <a v-for="(img,i) in result.images" :key="i" :href="img" target="_blank"><img :src="img" /></a>
      </div>
      <p v-else class="muted">{{ t('playground.noImages') }}</p>

      <div class="grid2" style="margin-top:16px">
        <div>
          <h3>{{ t('playground.runLog') }}</h3>
          <pre class="log">{{ (result.log||[]).join('\n') }}</pre>
        </div>
        <div>
          <h3>{{ t('playground.thinking') }}</h3>
          <div v-if="(result.trace||[]).length">
            <div class="trace-item" v-for="s in result.trace" :key="s.step"><b>#{{ s.step }}</b> {{ s.think }} <span class="muted mono" v-if="s.ops">— {{ s.ops }}</span></div>
          </div>
          <p class="muted" v-else>{{ t('playground.noTrace') }}</p>
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
const agents = ref([]), agentId = ref(0), prompt = ref(''), size = ref(''), deep = ref(false)
const files = ref([]), running = ref(false), result = ref(null)
async function load(){ try { agents.value = ((await api('/api/agents')).agents||[]).filter(a=>a.enabled!==false) } catch(e){} }
function onFiles(e){ files.value = Array.from(e.target.files||[]) }
async function run(){
  if(!agentId.value){ flash(t('playground.selectAgent')); return }
  if(!prompt.value.trim()){ flash(t('playground.inputPrompt')); return }
  running.value=true; result.value=null
  try {
    const fd = new FormData()
    fd.append('agent_id', String(agentId.value))
    fd.append('prompt', prompt.value)
    fd.append('size', size.value)
    fd.append('deep_think', deep.value?'1':'0')
    for(const f of files.value) fd.append('refs', f)
    const r = await api('/api/playground/agent',{method:'POST',body:fd})
    result.value = r
  } catch(e){ result.value = Object.assign({}, e.data||{}, { error:e.message }) }
  finally { running.value=false }
}
onMounted(load)
</script>

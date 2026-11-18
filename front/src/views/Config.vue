<template>
  <div>
    <div class="page-head"><h1>{{ t('config.title') }}</h1></div>

    <div class="panel">
      <h3>{{ t('config.base') }}</h3>
      <div class="row">
        <div><label>{{ t('config.guiPort') }}</label><input v-model.number="f.gui_port" type="number" /></div>
        <div><label>{{ t('config.apiPort') }}</label><input v-model.number="f.api_port" type="number" /></div>
      </div>
      <div class="row">
        <div><label>{{ t('config.storage') }}</label>
          <select v-model="f.storage_mode"><option value="sqlite">SQLite</option><option value="mysql">MySQL</option></select>
        </div>
        <div><label>{{ t('config.cacheMode') }}</label>
          <select v-model="f.cache_mode"><option value="file">File</option><option value="redis">Redis</option></select>
        </div>
      </div>
      <div v-if="f.storage_mode==='mysql'"><label>{{ t('config.mysqlDsn') }}</label><input v-model="f.mysql_dsn" /></div>
      <template v-if="f.cache_mode==='redis'">
        <div class="row">
          <div><label>{{ t('config.redisAddr') }}</label><input v-model="f.redis_addr" /></div>
          <div><label>{{ t('config.redisDb') }}</label><input v-model.number="f.redis_db" type="number" /></div>
        </div>
        <label>{{ t('config.redisPass') }}</label><input v-model="f.redis_pass" type="password" />
      </template>
      <div class="row">
        <div><label>{{ t('config.pythonBin') }}</label><input v-model="f.python_bin" /></div>
        <div><label>{{ t('config.imageEngine') }}</label>
          <select v-model="f.image_engine"><option value="go">Go (native)</option><option value="python">Python (PIL)</option></select>
        </div>
      </div>
      <div class="check"><input id="delout" type="checkbox" v-model="f.delete_outputs" /><label for="delout">{{ t('config.deleteOutputs') }}</label></div>
      <p class="hint">{{ t('config.restartNote') }}</p>
    </div>

    <div class="panel">
      <h3>{{ t('config.stock') }}</h3>
      <p class="hint">{{ t('config.stockTip') }}</p>
      <label>{{ t('config.stockOpenverse') }}</label>
      <input v-model="f.stock_openverse_base" placeholder="https://api.openverse.org" />
      <div class="row">
        <div><label>{{ t('config.stockPexels') }}</label><input v-model="f.stock_pexels_key" type="password" /></div>
        <div><label>{{ t('config.stockPixabay') }}</label><input v-model="f.stock_pixabay_key" type="password" /></div>
      </div>
    </div>

    <div class="panel">
      <h3>{{ t('config.account') }}</h3>
      <div class="row">
        <div><label>{{ t('config.username') }}</label><input v-model="acc.username" /></div>
        <div></div>
      </div>
      <div class="row">
        <div><label>{{ t('config.oldPass') }}</label><input v-model="acc.old_password" type="password" /></div>
        <div><label>{{ t('config.newPass') }}</label><input v-model="acc.new_password" type="password" /></div>
      </div>
    </div>

    <div class="form-actions">
      <button class="primary" @click="save" :disabled="saving">{{ t('common.save') }}</button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
const { t } = useI18n()
const flash = inject('flash')
const saving = ref(false)
const f = ref({})
const acc = ref({ username:'', old_password:'', new_password:'' })
async function load(){
  try {
    const c = await api('/api/config')
    f.value = Object.assign({ gui_port:8080, api_port:8090, storage_mode:'sqlite', cache_mode:'file',
      image_engine:'go', redis_db:0, delete_outputs:false, stock_openverse_base:'', stock_pexels_key:'', stock_pixabay_key:'' }, c)
    acc.value.username = c.gui_user||''
  } catch(e){}
}
async function save(){
  saving.value = true
  try {
    await api('/api/config',{method:'POST',body:{
      gui_port:f.value.gui_port, api_port:f.value.api_port, python_bin:f.value.python_bin,
      image_engine:f.value.image_engine, storage_mode:f.value.storage_mode, mysql_dsn:f.value.mysql_dsn,
      cache_mode:f.value.cache_mode, redis_addr:f.value.redis_addr, redis_db:f.value.redis_db,
      redis_pass:f.value.redis_pass, delete_outputs:f.value.delete_outputs,
      stock_openverse_base:f.value.stock_openverse_base, stock_pexels_key:f.value.stock_pexels_key,
      stock_pixabay_key:f.value.stock_pixabay_key
    }})
    await api('/api/user',{method:'POST',body:{
      username:acc.value.username, old_password:acc.value.old_password, new_password:acc.value.new_password
    }})
    flash(t('common.saved'))
  } catch(e){ flash(e.message) } finally { saving.value=false }
}
onMounted(load)
</script>

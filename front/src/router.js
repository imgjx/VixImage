import { createRouter, createWebHistory } from 'vue-router'
import Overview from './views/Overview.vue'
import MyModels from './views/MyModels.vue'
import Upstream from './views/Upstream.vue'
import Playground from './views/Playground.vue'
import ApiKeys from './views/ApiKeys.vue'
import Config from './views/Config.vue'

const routes = [
  { path: '/', name: 'overview', component: Overview },
  { path: '/overview', redirect: '/' },
  { path: '/mymodels', name: 'mymodels', component: MyModels },
  { path: '/upstream', name: 'upstream', component: Upstream },
  { path: '/playground', name: 'playground', component: Playground },
  { path: '/apikeys', name: 'apikeys', component: ApiKeys },
  { path: '/config', name: 'config', component: Config }
]

const router = createRouter({ history: createWebHistory(), routes })
export default router

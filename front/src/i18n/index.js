import { createI18n } from 'vue-i18n'

export const zh = {
  app: { name: 'VixImage', sub: '图像 Agent 控制台' },
  nav: {
    overview: '概览', mymodels: '我的模型', upstream: '上游模型',
    playground: 'Playground', apikeys: 'API 密钥', config: '系统配置'
  },
  common: {
    logout: '退出', save: '保存', cancel: '取消', add: '添加', create: '创建',
    edit: '编辑', delete: '删除', confirm: '确认', close: '关闭', refresh: '刷新',
    enabled: '启用', disabled: '禁用', yes: '是', no: '否', actions: '操作',
    loading: '加载中…', saved: '已保存', failed: '操作失败', retry: '重试',
    optional: '可选', none: '无', search: '搜索', all: '全部', tips: '提示',
    language: '语言'
  },
  login: {
    title: '登录', account: '账号', password: '密码', button: '登录',
    defaultTip: '默认账号 admin / admin123（可在控制台或 CLI 修改）'
  },
  overview: {
    title: '调用概述', total: '总调用', success: '成功', failed: '失败',
    running: '进行中', outputs: '产出图片', steps: '累计步数',
    quickStart: '快速上手', runMode: '运行模式',
    storage: '存储', cache: '缓存', guiport: 'GUI 端口', apiport: 'API 端口',
    id: 'ID', agent: '模型', prompt: '提示词', status: '状态', images: '产出',
    llmCalls: 'LLM 次数', stepsUsed: '步数', duration: '耗时', created: '时间',
    trace: '轨迹', thinking: '思考轨迹', noLogs: '暂无调用记录', prev: '上一页', next: '下一页',
    pageSize: '每页', of: '/', successPartial: '半成品',
    detail: '详情', theme: '主题', error: '错误', noDetail: '无详情',
    intro1: '在「上游模型」添加图片模型与大模型，LLM 需设 max_tokens（小上下文模型务必配置）。',
    intro2: '在「我的模型」创建 Agent：基座 + 图片模型 + 步数预算 + 耗尽策略 + 风格。',
    intro3: '用 Playground 试玩，或用 API POST /v1/agents/run 调起（可传参考图）。',
    intro4: '自循环 Agent：多模态大模型自主规划、生图、排版、审片，带系统级强制自检，多轮迭代出片。',
    intro5: '免版权素材库：Agent 可用 stock 技能从 Openverse / Pexels / Pixabay 搜索真实图片嵌入海报。',
    intro6: '全链路有容错无臆造：解析容错、429 退避、索引越界跳过；非法工具/生图失败则硬报错。'
  },
  upstream: {
    title: '上游模型', desc: '按类型配置图片模型或大模型，兼容多种上游协议。',
    name: '名称', type: '类型', format: '格式', baseUrl: '接口地址', apiKey: 'API 密钥',
    modelName: '模型名', multimodal: '多模态', enabled: '启用',
    size: '默认尺寸', sizes: '可选尺寸(逗号分隔)', n: '张数', steps: '单次生图步数',
    negPrompt: '反向提示词', agentDesc: '对 Agent 的介绍',
    maxTokens: '最大输出(max_tokens)', contextLimit: '上下文限制(context_limit)',
    temperature: '温度', topP: 'Top P', systemPrompt: '系统提示词前缀',
    typeImage: '图片模型', typeLLM: '大模型 LLM', noData: '暂无上游模型',
    maskedKeyTip: '掩码显示，留空表示不修改', addTitle: '添加上游模型', editTitle: '编辑上游模型',
    confirmDelete: '确认删除该上游模型？'
  },
  mymodels: {
    title: '我的模型', desc: '创建 Agent：选择基座与图片引擎，设定步数与风格。',
    name: '名称', callName: '调用名', baseModel: '基座模型', maxSteps: '总步数预算',
    prompt: '人设/提示词前缀', style: '风格', exhaust: '耗尽策略',
    learning: '学习数据(Few-shot)', deepThink: '深度思考',
    imageModels: '图片引擎', stepsPerGen: '单次消耗步数', addEngine: '添加引擎',
    noData: '暂无 Agent 模型', addTitle: '创建 Agent', editTitle: '编辑 Agent',
    confirmDelete: '确认删除该 Agent？', styleOnDemand: '按需执行', styleCreative: '创意脑洞', styleCinematic: '电影大片',
    exhaustFail: '生成失败', exhaustPartial: '返回半成品'
  },
  apikeys: {
    title: 'API 密钥', desc: '管理 API 调用密钥，按密钥统计调用次数。',
    key: '密钥', name: '备注', calls: '调用次数', lastUsed: '最近调用',
    created: '创建时间', status: '状态', customKey: '自定义密钥(留空自动生成)',
    noData: '暂无密钥，请创建一个', addTitle: '新建密钥', confirmDelete: '确认删除该密钥？',
    createdTip: '密钥仅创建时完整显示一次，请妥善保存'
  },
  config: {
    title: '系统配置', base: '基础', ports: '端口', portsTip: '改动端口需重启生效',
    guiPort: 'GUI 端口', apiPort: 'API 端口', dataDir: '数据目录', sqlitePath: 'SQLite 路径',
    storage: '存储模式', cacheMode: '缓存模式', mysqlDsn: 'MySQL DSN',
    redisAddr: 'Redis 地址', redisDb: 'Redis DB', redisPass: 'Redis 密码',
    pythonBin: 'Python 路径', imageEngine: '图像引擎', deleteOutputs: '提取后删除成品',
    account: '账号安全', username: '用户名', oldPass: '原密码', newPass: '新密码',
    accountTip: '修改账号密码', stock: '素材库密钥',
    stockOpenverse: 'Openverse 端点(可选)', stockPexels: 'Pexels API Key', stockPixabay: 'Pixabay API Key',
    stockTip: 'Openverse 免密钥；Pexels / Pixabay 需自行申请 API Key',
    accountUpdated: '账号已更新', restartNote: '部分配置(存储/缓存模式)重启后生效'
  },
  playground: {
    title: 'Playground', desc: '选择「我的模型」跑完整 Agent 自循环，可传参考图。',
    agent: '我的模型', prompt: '创作指令', promptPh: '例如：做一张竖版科幻海报…',
    size: '尺寸', deepThink: '深度思考', refs: '参考图', run: '开始运行', running: '运行中…',
    result: '运行结果', images: '产出图片', noImages: '本次没有产出图片',
    runLog: '运行日志', thinking: '每步思考', noTrace: '暂无轨迹',
    status: '状态', steps: '步数', llm: '大模型请求', theme: '主题',
    selectAgent: '请选择 Agent', inputPrompt: '请输入创作指令',
    uploadTip: '可多选图片作为参考图'
  }
}

export const en = {
  app: { name: 'VixImage', sub: 'Image Agent Console' },
  nav: {
    overview: 'Overview', mymodels: 'My Models', upstream: 'Upstream',
    playground: 'Playground', apikeys: 'API Keys', config: 'Settings'
  },
  common: {
    logout: 'Logout', save: 'Save', cancel: 'Cancel', add: 'Add', create: 'Create',
    edit: 'Edit', delete: 'Delete', confirm: 'Confirm', close: 'Close', refresh: 'Refresh',
    enabled: 'Enabled', disabled: 'Disabled', yes: 'Yes', no: 'No', actions: 'Actions',
    loading: 'Loading…', saved: 'Saved', failed: 'Operation failed', retry: 'Retry',
    optional: 'Optional', none: 'None', search: 'Search', all: 'All', tips: 'Tips',
    language: 'Language'
  },
  login: {
    title: 'Login', account: 'Username', password: 'Password', button: 'Sign in',
    defaultTip: 'Default: admin / admin123 (change it in console or CLI)'
  },
  overview: {
    title: 'Overview', total: 'Total Calls', success: 'Success', failed: 'Failed',
    running: 'Running', outputs: 'Images', steps: 'Total Steps',
    quickStart: 'Quick Start', runMode: 'Runtime Mode',
    storage: 'Storage', cache: 'Cache', guiport: 'GUI Port', apiport: 'API Port',
    id: 'ID', agent: 'Agent', prompt: 'Prompt', status: 'Status', images: 'Images',
    llmCalls: 'LLM Calls', stepsUsed: 'Steps', duration: 'Duration', created: 'Time',
    trace: 'Trace', thinking: 'Thinking Trace', noLogs: 'No call records yet', prev: 'Prev', next: 'Next',
    pageSize: 'Per page', of: '/', successPartial: 'Partial',
    detail: 'Detail', theme: 'Theme', error: 'Error', noDetail: 'No detail',
    intro1: 'Add image & LLM models under Upstream; set max_tokens for the LLM.',
    intro2: 'Create an Agent under My Models: base + image engines + step budget + exhaust policy + style.',
    intro3: 'Try it in Playground, or call API POST /v1/agents/run (reference images supported).',
    intro4: 'Self-looping Agent: the LLM plans, generates, composes and reviews, with forced visual self-check.',
    intro5: 'Royalty-free stock: the stock skill searches Openverse / Pexels / Pixabay for real images.',
    intro6: 'Fault-tolerant but never fabricating: parse tolerance, 429 backoff, index-skip; hard errors otherwise.'
  },
  upstream: {
    title: 'Upstream Models', desc: 'Configure image or LLM models with multiple upstream protocols.',
    name: 'Name', type: 'Type', format: 'Format', baseUrl: 'Base URL', apiKey: 'API Key',
    modelName: 'Model Name', multimodal: 'Multimodal', enabled: 'Enabled',
    size: 'Default Size', sizes: 'Sizes (comma)', n: 'Count', steps: 'Steps / image',
    negPrompt: 'Negative Prompt', agentDesc: 'Description for Agent',
    maxTokens: 'Max Output (max_tokens)', contextLimit: 'Context Limit',
    temperature: 'Temperature', topP: 'Top P', systemPrompt: 'System Prompt Prefix',
    typeImage: 'Image Model', typeLLM: 'LLM', noData: 'No upstream models',
    maskedKeyTip: 'Masked; leave blank to keep unchanged', addTitle: 'Add Upstream', editTitle: 'Edit Upstream',
    confirmDelete: 'Delete this upstream model?'
  },
  mymodels: {
    title: 'My Models', desc: 'Create an Agent: pick base & image engines, set steps and style.',
    name: 'Name', callName: 'Call Name', baseModel: 'Base Model', maxSteps: 'Step Budget',
    prompt: 'Persona / Prompt Prefix', style: 'Style', exhaust: 'Exhaust Policy',
    learning: 'Few-shot Learning', deepThink: 'Deep Thinking',
    imageModels: 'Image Engines', stepsPerGen: 'Steps / gen', addEngine: 'Add Engine',
    noData: 'No Agent models', addTitle: 'Create Agent', editTitle: 'Edit Agent',
    confirmDelete: 'Delete this Agent?', styleOnDemand: 'On Demand', styleCreative: 'Creative', styleCinematic: 'Cinematic',
    exhaustFail: 'Fail', exhaustPartial: 'Return Partial'
  },
  apikeys: {
    title: 'API Keys', desc: 'Manage API keys and per-key call statistics.',
    key: 'Key', name: 'Label', calls: 'Calls', lastUsed: 'Last Used',
    created: 'Created', status: 'Status', customKey: 'Custom key (blank to auto-generate)',
    noData: 'No keys yet, create one', addTitle: 'New Key', confirmDelete: 'Delete this key?',
    createdTip: 'The full key is shown only once at creation; save it safely'
  },
  config: {
    title: 'Settings', base: 'General', ports: 'Ports', portsTip: 'Port changes require restart',
    guiPort: 'GUI Port', apiPort: 'API Port', dataDir: 'Data Dir', sqlitePath: 'SQLite Path',
    storage: 'Storage', cacheMode: 'Cache', mysqlDsn: 'MySQL DSN',
    redisAddr: 'Redis Addr', redisDb: 'Redis DB', redisPass: 'Redis Password',
    pythonBin: 'Python Path', imageEngine: 'Image Engine', deleteOutputs: 'Delete output after export',
    account: 'Account', username: 'Username', oldPass: 'Old Password', newPass: 'New Password',
    accountTip: 'Change account / password', stock: 'Stock Library Keys',
    stockOpenverse: 'Openverse Endpoint (optional)', stockPexels: 'Pexels API Key', stockPixabay: 'Pixabay API Key',
    stockTip: 'Openverse needs no key; Pexels / Pixabay need your own API Key',
    accountUpdated: 'Account updated', restartNote: 'Some settings (storage/cache) apply after restart'
  },
  playground: {
    title: 'Playground', desc: 'Run a full Agent loop with My Models; reference images supported.',
    agent: 'My Model', prompt: 'Prompt', promptPh: 'e.g. Create a vertical sci-fi poster…',
    size: 'Size', deepThink: 'Deep Thinking', refs: 'Reference Images', run: 'Run', running: 'Running…',
    result: 'Result', images: 'Output Images', noImages: 'No images produced this run',
    runLog: 'Run Log', thinking: 'Step Thinking', noTrace: 'No trace yet',
    status: 'Status', steps: 'Steps', llm: 'LLM calls', theme: 'Theme',
    selectAgent: 'Select an Agent', inputPrompt: 'Enter a prompt',
    uploadTip: 'Select multiple images as references'
  }
}

const i18n = createI18n({
  legacy: false,
  locale: localStorage.getItem('vix_lang') || 'zh',
  fallbackLocale: 'zh',
  messages: { zh, en }
})

export function setLang(lang) {
  i18n.global.locale.value = lang
  localStorage.setItem('vix_lang', lang)
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en'
}

export default i18n

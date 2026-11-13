# VixImage

一个用 Go 写的**简易自循环图片编辑 Agent 系统**：给一个提示词，多模态大模型进入
`观察 → 决策 → 执行` 自循环，每轮输出一个 `{think, tools[]}`，执行器严格解析并逐条执行一批
**全部与图像工作有关**的技能，由 **Go 原生渲染引擎**合成成片。它有明确的**完成信号**（`finish`），
也能反复**查看自己的作品**（`view`）后再改。

核心是**排版完全由 LLM 自由规划、不套模板**：画布尺寸（竖屏 / 横版 / 方形）、每个图层的位置与
样式、参考图的编辑与嵌入都由模型自己决定；同一个 Agent 做「竖屏科幻电影海报」和「横版撞色拼贴」
会得到截然不同的版式，而不会张张一个样。

## 特性

- **自循环 Agent**
  - **每次大模型请求 = 1 步**：一轮输出一个短思考标记 `think` + 一批 `tools`（一次可含多个
    skill use），状态跨轮累积，Agent 可「打底 → 查看 → 调整 → 完成」反复迭代，不是一次性吐完整份脚本
  - **技能全是图像工作**：`canvas`（建/重设画布）、`gen`（生成图片）、`add / set / del / clear`（图层编辑）、
    `layers`（查看图层清单）、`copy`（**复制中间产物存档/分叉/复用**）、`undo`（**编辑回滚**）、
    `view`（查看作品）、`finish`（完成并标注产物）
  - **链底工具**：`view` 与 `finish` 必须出现在 `tools` 最后一个、每轮最多一个，放中间直接报错
  - **完成信号**：只有收到 `finish` 才交付成片
  - **会看自己的作品**：`view` 把当前画布或指定产物渲染出来回灌给下一轮大模型；
    连续编辑多轮时 Agent 看不到产物，只会收到「编辑成功」状态文本
  - **质量铁律（主提示词）**：明确要求**成片质量必须完美、只允许极细微瑕疵**（文字零重叠/零出界/零裁切、
    主次分明、留白得当、对齐工整、配色协调）；**完成一次大改或准备 `finish` 前必须 `view` 自检偏移**，
    有偏差用 `set/del` 微调或 `undo` 回滚、改到位再交付；**一轮可放很多个动作**，建议一次把整版铺完整
  - **系统级强制 view（提交产物前，硬保证）**：不依赖模型自觉——模型一旦发出 `finish`，系统先
    **拦截并强制渲染「本次要交付的成片」回灌给下一轮**，附自检提示逼其核对；模型看过成片后满意可再次
    `finish` 确认，有瑕疵先修正（改动会置脏 → 再次强制回看）。有次数上限（`MaxForceReviews`）防死循环。
    即「提交产物前必看一次自己的作品」由系统兜底，不再靠提示词软要求
  - **主动创意·场景化构图 + 画中画透视贴合**：主提示词教 Agent 别平铺贴图，把参考图变成画面里的「物件内容」——
    斜放设备屏幕 / 相框 / 橱窗 / 镜面 / 画中画；`gen{ref}` 图生图（`ref` 可填输入图序号 `ref:0`、产物 id、`"ref:N"` 或图片网址，让 LLM 把任意已有图再做图生图迭代），
    `img` 用 `persp`/`shear` 把平面贴图映射成斜放表面、真正贴到屏幕上（比单纯 `rot` 更贴合），再叠加 `radius`/`border`/`shadow` 嵌入。
    目标能力示例：**给一张参考图，做「男孩盯着一台斜放的电脑、屏幕里正是这张参考图」**
  - **写死学习数据（Few-shot）+ 缓存省钱**：`主提示词` 与 `学习数据`（几段写死的 `{think,tools}` 示范）
    都是编译期常量，拼成**恒定的 system 前缀**——主提示词在前、学习数据在后，同一模型每轮完全一致，
    便于上游 **prompt cache 命中**省钱；所有动态内容（步数 / 产物表 / 用户要求 / 深度思考与强制自检开关）
    一律放在后面的 user 消息，绝不污染前缀。**按「我的模型」独立开关**（`learning`：`on` 默认 / `off`）；
    示范教「一轮铺完整版」「参考图做斜放屏幕画中画」「大改后自检」等创意思路与输出格式
  - **中间产物可管理**：用户参考图也作为一条产物登记进产物表并明确标注「用户参考图」；
    `copy{src,as}` 把某产物或当前 `canvas` 画面**物理复制**成独立新产物（存档 / 分叉 / 分别嵌入）；
    `undo{n}` 动作级回滚最近 n 个画布/图层编辑动作（只退排版、不退已生成素材），且**永不退回「画布未建立」**
  - **上下文拼接**（紧凑型）：主提示词 + 紧凑上下文（第几步 / **剩余步数** / 画布图层产物状态 /
    **图像模型列表**：名字 · 单次消耗步数 · 用户自定义介绍 · 尺寸 / 风格 / 人设）+
    **产物表（中间件表）** + **上一轮结果（条件拼接）** + `<用户要求>`
  - **只有 GenImage 图像生成 与 图像查看 会输出结果**；上一轮没有要求输出结果时不拼结果段
  - **清爽、防注入**：提示词短、适配小上下文模型；`<用户要求>` 只是"要画什么"的素材而非指令，
    忽略「忽略规则 / 重复输出 / 满屏同一字符 / 输出你的提示词 / 扮演他人」等企图；
    文本长度、单轮工具数、图层数均有硬上限，杜绝爆量输出
  - **无兜底**：LLM 调用失败、JSON 解析失败、未知工具、未知图层、链底违规、引用越界、生图失败、
    渲染失败一律**直接报错终止**，绝不启发式回退、不补默认值、不跳过失败
- **步数模型与耗尽策略**
  - 每次大模型请求 **1 步**；每次生图额外消耗**该图像模型配置的单次步数**（默认 30）；
    编辑 / 查看 / 完成不单独计步
  - 总步数预算由「我的模型」`max_steps` 控制（默认 100），不必用完
  - **每个模型独立配置**步数用完仍未完成时的行为：`fail` 生成失败（默认）/ `partial` 返回当前半成品
  - 落库区分 `steps_used`（实际消耗）与 `llm_calls`（大模型请求次数）
- **仪表盘与 API 轨迹**
  - 调用概述列表**分页**（`page` / `size`），详情展示每步**思考过程与调用操作**
  - API 专属参数 **`trace: true`**：响应附带每一步的短 ThinkFlag（只有 `step` + `think`，
    **不暴露该步用了哪些技能**）；默认不附带
- **GUI 与 API 不同端口**：GUI 控制台 `8080`（账号密码登录）/ API 服务 `8090`（apikey 鉴权）
- **全链路 LLM 驱动**（Agnes 免费文本模型即可，例如 `agnes-2.5-flash` / `agnes-3.0-flash`）
  - **上游模型按类型分专属字段**：图片模型只配尺寸 / 张数 / 步数 / 反向提示词 / 引擎介绍；
    大模型只配 `max_tokens` / `temperature` / `top_p` / 系统提示词前缀 —— 两类字段互不串台
  - 每个 LLM 可配 **`max_tokens`**（自动注入请求）
  - **风格模式**：按需执行 · 创意脑洞 · 电影大片；**深度思考模式**可单请求覆盖
  - 上游图片模型可填「对 Agent 的介绍」+ 多尺寸 `sizes`，Agent 据此对症选引擎
- **图像生产技能 `gen`**：兼容 OpenAI / Agnes 图片 API，**并行创建请求**（上限 4）、
  图生图 `ref`、`response_format` 与 `data[].url / b64_json / images / output` 等多种响应格式
- **Agent 临时工作区**：每次运行独立工作区，可放多张图片、做坏了 `undo` 回滚或重做同名产物即覆盖，
  也能用 `copy` 存档一版再分叉试改；成品提取后**中间产物 100% 删除**；
  可配置「提取后是否连成品一起删除」（`delete_outputs`）
- **Playground**：控制台试玩页 —— 选「我的模型」+ 提示词 + 参考图，跑**完整 Agent 自循环**
  （规划 → 生图 → 渲染 → 审片 → 交付），并展示运行日志与每步思考
- **双缓存 / 双存储**：`file` / `redis`，`sqlite`（纯 Go）/ `mysql`，配置切换重启生效

## 目录结构

```
viximage/
├── main.go                  # 入口 + CLI（user / apikey / config）
├── go.mod / go.sum
├── internal/
│   ├── config/              # 配置（端口 / 账号 / apikey / 双存双缓模式）
│   ├── storage/             # Storage 接口 + SQLite / MySQL 双实现（调用日志分页）
│   ├── cache/               # Cache 接口 + File / Redis 双实现
│   ├── client/              # 上游图片/LLM 兼容 HTTP 客户端
│   ├── upstream/            # 上游模型管理（多模态限制、max_tokens、sizes、介绍）
│   ├── agentmodel/          # 我的模型（步数预算 / 耗尽策略 / 风格）
│   ├── agent/               # 自循环图片编辑 Agent 运行时
│   │   ├── script.go        #   Turn / Tool / renderState / 步数常量 / 严格校验 / 转渲染规格
│   │   ├── script_parse.go  #   解析层字段值容错（颜色数组、字段别名、尾随逗号、碎片对象合并）
│   │   ├── planner.go       #   短主提示词 + 上下文拼接 + 单轮 LLM 决策
│   │   ├── agent.go         #   自循环主循环：执行工具 / 并行生图 / 查看回灌 / 完成 / 落库
│   │   └── skills/          # 生产技能 + Go 原生通用渲染引擎（render.go / effects.go / text.go ...）
│   ├── api/                 # API 服务（apikey 鉴权、trace 参数）
│   └── gui/                 # GUI 控制台（登录 + 五个页面，内嵌 web/）
│       └── web/             # 原生 HTML 页面
└── README.md
```

## 构建与运行

```bash
GOSUMDB=off go build -o viximage .
./viximage                                       # 启动 GUI + API，配置落在 ./config.json
./viximage -dir /data/vix                        # 指定配置/数据目录（数据收进该目录下 data/）
./viximage -config /etc/vix/config.json          # 显式指定配置文件（覆盖 -dir）
./viximage -gui-port 9000 -api-port 9001         # 命令行临时覆盖端口（不写回配置）
./viximage -version                              # 打印版本号
```

启动参数：
- `-dir <目录>`：配置/数据根目录（默认 `.`，首次运行生成 `config.json`）
- `-config <文件>`：直接指定配置文件路径，优先级高于 `-dir`
- `-gui-port <端口>` / `-api-port <端口>`：临时覆盖端口（0 表示用配置值）
- 运行期 CLI 会实时打印启动信息与每个请求的访问日志（方法 / 路径 / 状态码 / 耗时）

> 登录：未登录时 302 跳到登录页，登录页是**原生 HTML 表单**提交（服务端渲染错误提示，**不依赖 JavaScript**），
> 老旧浏览器同样可用；会话同时支持 Cookie / `X-Vix-Sid` 请求头 / URL 令牌三种携带方式，
> 浏览器不保存 Cookie 时也能靠本地令牌自动恢复登录，不会反复被弹回登录页。
> 会话保存在**进程内存**中，不依赖磁盘缓存目录可写（Windows 装在 Program Files 等场景也能正常登录）。

### Docker 部署

仓库根目录已提供多阶段 `Dockerfile`（编译用 golang:1.21-alpine，运行用 alpine，含 Python/PIL 可选引擎与中文字体）。

```bash
docker build -t viximage:0.1.0 --build-arg VERSION=0.1.0 .
docker run -d --name viximage \
  -p 8080:8080 -p 8090:8090 \
  -v viximage-data:/data \
  viximage:0.1.0
```

也可以直接用 compose：

```bash
docker compose up -d
```

- 控制台：http://localhost:8080　API：http://localhost:8090
- 数据（配置 / 数据库 / 缓存 / 作品）持久化在 `viximage-data` 卷的 `/data` 下
- 默认账号 `admin` / `admin123`，登录后在「系统配置」里修改
- 自定义端口：改 `docker-compose.yml` 里的 `ports` 映射，或用环境变量覆盖
- Dockerfile 已配置 Go 模块代理回退（goproxy.cn → goproxy.io → 官方），
  国内网络构建不会卡在 `proxy.golang.org`；也可自行覆盖：
  `docker build --build-arg GOPROXY=https://your-proxy .`

默认账号：`admin` / `admin123`。默认 apikey：`vix-sk-default-0001`。

## CLI（无需启动服务即可管理）

```bash
./viximage user set --username admin --password NewPass   # 改 GUI 账号密码
./viximage user set --username NewName                  # 仅改用户名（不动密码）
./viximage apikey add                                     # 新建密钥（自动生成，打印完整密钥，仅此一次）
./viximage apikey add vix-sk-xxxx                         # 新建指定密钥（需全局唯一）
./viximage apikey list                                    # 列出全部密钥（含调用次数）
./viximage apikey rm <id|key>                             # 删除密钥（按 id 或完整密钥）
./viximage config                                         # 打印当前配置
```

## 控制台页面

- **调用概述**：总调用 / 成功 / 失败 / 产出图 / 累计步数，**分页流水**，详情含每步 ThinkFlag 与操作轨迹
- **上游模型**：按类型分专属字段 —— 图片模型（默认尺寸 / 多尺寸 / 张数 / 单次步数 / 反向提示词 / 引擎介绍）
  与多模态大模型（`max_tokens` / `temperature` / `top_p` / 系统提示词前缀）
- **我的模型**：创建 / 编辑 Agent（基架模型 + **总步数** + 图片模型**单次消耗步数** +
  风格 + 深度思考 + **步数用完策略 fail / partial**）
- **API 密钥**：独立的密钥管理页，可创建多个密钥、分别统计调用次数与最近调用时间（**默认不预置任何密钥**）
- **系统配置**：端口、双存储 / 双缓存、GUI 账号与密码（随「保存配置」一起提交）、图片编辑引擎、提取后删除成品
- **Playground**：选「我的模型」跑完整 Agent 自循环，可传参考图，并查看运行日志与每步思考

## API 文档

API 服务默认监听 `:8090`（可在「系统配置」或启动参数 `-api-port` 修改）。
**所有接口除 `/health` 外均需鉴权**；鉴权失败返回 `401 {"error":"invalid or missing apikey"}`。

### 鉴权方式

每个请求二选一携带密钥：

| 方式 | 写法 |
| --- | --- |
| 请求头 | `X-API-Key: vix-sk-xxxx` |
| 请求头 | `Authorization: Bearer vix-sk-xxxx` |

密钥在「API 密钥」页或 CLI `viximage apikey add` 创建，**默认不预置任何密钥**。
每次成功调用都会累加该密钥的 `calls` 计数与最近调用时间（可在「API 密钥」页查看）。

---

### 健康检查

`GET /health`

```json
{ "status": "ok", "service": "viximage-api", "port": 8090 }
```

---

### 列出我的模型

`GET /v1/agents`

```bash
curl -H "X-API-Key: vix-sk-xxxx" http://localhost:8090/v1/agents
```

响应：

```json
{ "agents": [
  { "id": 1, "name": "海报师", "call_name": "poster", "max_steps": 100,
    "base_model_id": 1, "image_models": [...], "enabled": true,
    "style": "creative", "prompt": "..." }
] }
```

---

### 调起一个模型（核心接口）

`POST /v1/agents/run` · `Content-Type: application/json`

请求体：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `call_name` | string | **必填**，模型调用名（见 `/v1/agents`） |
| `prompt` | string | **必填**，创作指令 |
| `images` | string[] | 可选，参考图。元素为 `/v1/upload` 返回的 `up_xxx.png` id，或 http(s) 直链 |
| `webhook` | string | 可选，完成时回调的 URL |
| `deep_think` | bool | 可选，本次强制开启深度思考（覆盖模型设置） |
| `size` | string | 可选，强制尺寸如 `"1280x720"`（覆盖生图画幅映射） |
| `trace` | bool | 可选，响应附带每一步短 ThinkFlag（仅 step + think，不暴露内部技能） |

```bash
# 基础调用：Agent 进入自循环作图
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"做一张竖版科幻电影海报，中心发光星球，顶部大标题，底部演职员小字；先排版，再查看自己的作品，最后完成"}'

# 带 trace：查看每步思考
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"做一张横版撞色音乐节海报","trace":true}'

# 传多张参考图：图层 src 用 ref:0 / ref:1，可抠图/虚化/旋转/圆形裁切后嵌入任意区域
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"把参考图圆形裁切后斜放在右侧，左侧放品牌大字","images":["up_xxx.png"]}'

# 单次请求强制开启深度思考 / 覆盖尺寸
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"科幻海报","deep_think":true,"size":"1280x720"}'
```

成功响应：

```json
{
  "status": "success",
  "output_images": ["/v1/files/xxx.png"],
  "steps_used": 22,
  "llm_calls": 2,
  "exhausted": false,
  "partial": false,
  "deep_think": false,
  "style": "creative",
  "theme": "...",
  "workspace": "...",
  "log": ["...", "..."],
  "reasoning": "...",
  "trace": [ { "step": 1, "think": "..." } ]
}
```

`status` 取值：`success` / `success_partial`（步数耗尽但已出图） / `failed`。
`trace:true` 时才会返回 `trace[]` 与 `reasoning`。

---

### 上传输入图

`POST /v1/upload` · `multipart/form-data`，字段名 `file`（可多文件）

```bash
curl -X POST http://localhost:8090/v1/upload -F "file=@cat.png"
```

响应：`{ "uploaded": ["up_1700000000000_cat.png"] }` —— 把其中的 `up_xxx.png` 填进
`/v1/agents/run` 的 `images[]` 即作为参考图。

### 取回产出图

`GET /v1/files/{name}` —— 直接返回图片二进制（无需鉴权头之外的额外参数，但需 API Key）。

```bash
curl -H "X-API-Key: vix-sk-xxxx" http://localhost:8090/v1/files/xxx.png -o out.png
```

---

### 上游模型列表

`GET /v1/upstream` —— 返回已配置的上游模型（密钥字段已掩码）。

```bash
curl -H "X-API-Key: vix-sk-xxxx" http://localhost:8090/v1/upstream
```

---

### API 密钥管理（GUI 会话鉴权，需登录）

以下接口由控制台「API 密钥」页调用，需携带登录 Cookie（非 API Key）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/apikeys` | 列出全部密钥：`{keys:[{id,key,key_mask,name,calls,last_used_at,created_at,enabled}]}` |
| `POST` | `/api/apikeys` | 新建密钥，body `{key?,name?}`；`key` 留空自动生成，返回 `{ok,id,key}` |
| `DELETE` | `/api/apikeys/{id}` | 删除指定密钥，返回 `{ok:true}` |

CLI 等价命令：`viximage apikey list | add [key] | rm <id|key>`。

---

响应关键字段：`status`（`success` / `success_partial` / `failed`）、`output_images`、
`steps_used`、`llm_calls`、`exhausted`、`partial`；`trace:true` 时额外返回 `trace[]` 与 `reasoning`。

## 工作流说明（自循环 Agent）

1. 在「上游模型」添加一个图片模型和一个多模态大模型，给 LLM 设好 `max_tokens`（建议 ≥ 2400）。
2. 在「我的模型」创建 Agent：选基座、设总步数预算（默认 100）、添加图片模型并设**每次生图消耗步数**
   （默认 30）、选风格、设置**步数用完仍未完成时**：生成失败 / 返回半成品；
   按需开关**学习数据（Few-shot 示范）**（默认开，写死前缀可命中缓存省钱）。
3. 用 API `POST /v1/agents/run` 传 `call_name` + `prompt`（可选 `images` / `deep_think` / `size` / `trace`）。
4. Agent 自循环，直到 `finish` 或步数耗尽：
   1. **拼上下文**：写死的 system 前缀（主提示词 + 可选学习数据，恒定可命中缓存）+ user 消息：
      紧凑上下文（含剩余步数、图像模型列表、状态）+ 产物表 + 上一轮结果（只有上一轮有 `gen` / `view`
      输出图才拼，编辑轮只回「编辑成功」）+ 用户要求，以及深度思考 / 系统强制自检的开关提示。
      首轮附输入参考图，后续轮附上一轮输出图（缩略，最多 6 张）。
   2. **决策**：一次大模型请求 = **1 步**，输出 `{think, tools[]}`；严格校验工具、图层、链底规则。
   3. **执行**：一批工具顺序执行；`gen` 并发生图并按引擎扣分；`view` 在链底渲染当前作品回灌下一轮；
      编辑类只改内存状态、不落盘、不回图。每个画布/图层编辑动作前**动作级压一份内存快照**，
      `undo` 据此精确回滚；`copy` 把产物或当前画面**物理复制**成独立新产物；`layers` 回灌图层清单。
      状态（画布 / 图层 / 产物表）跨轮保留，`undo` 只退排版、产物表常驻。
   4. **系统级强制 view + 完成**：模型发出 `finish` 时系统**先拦截**，强制渲染待交付成片回灌下一轮并附
      自检提示 → 模型核对后满意再次 `finish` 确认（或先修正，修正会触发再回看，有次数上限）→
      才渲染最终画布 / 引用指定产物，提取成品、清理工作区、落库轨迹。
   5. **耗尽**：`fail` 直接报错；`partial` 渲染当前画布返回 `success_partial`。

> **解析容错 ≠ 操作兜底**：小模型常把颜色写成 RGB 数组、工具名当 key、参数包在 `params` 里、
> `layer` 写成数组、带尾随逗号、把 `think`/`tools` 拆成两个对象、给产物 id 加 `gen:` 前缀等，
> 这些**字段写法差异由解析层归一化**（`script_parse.go`），不改变工具语义。
> 但**未知工具、未知图层、链底违规、引用越界、生图/渲染失败仍是硬错误**，绝不猜测操作或补全默认值。

## 渲染引擎的图层能力

`tools` 里的 `add / set` 图层映射到 Go 原生渲染器（`internal/agent/skills`）的规格项：

| 图层 | 能力 |
|---|---|
| `img` | `fit` cover/contain、圆角 `radius`、圆形裁切 `circle`、投影 `shadow`、描边 `border`、压暗 `ov`、**抠图 `cut`**、**虚化 `blur`**、**旋转斜放 `rot`**、**透视贴合 `persp`(0~0.9) / `shear`(-0.5~0.5)**（把平面贴图映射成斜放表面，画中画更贴屏）、透明度 `opacity` |
| `text` | 多行、自动折行、锚点 `anchor`、字间距 `spacing`、**斜体 `italic`**、辉光 `glow`、描边 `stroke` |
| `rect` / `circle` | 半透明色块 / 圆形色块、圆角、填充 |
| `line` / `scrim` | 装饰线 / 渐变蒙版（`dir` 指定最暗一侧，平滑衰减） |
| `canvas` | 任意宽高（竖屏/横版/方形）、渐变或纯色背景 |

`src` 可引用：产物 id（也兼容 `gen:xxx` 等前缀）、`ref:N`（第 N 张输入图）、图片 URL / data URI。
对 `ref:N` 叠加 `cut / blur / rot / persp / shear / circle` 即可「先编辑参考图、再透视贴合嵌入任意区域」。

字体自动探测：优先 `Noto Sans CJK`（TTC 集合，取简体中文子字体），兼容 `.otf` 单字体与
`WenQuanYi` / `PingFang`；等宽字体仅用于纯 ASCII，含中文时自动回退 CJK 字体（避免方块字）。

## 降级与容错

- **缓存**：`redis` 模式自动包裹一层 File 备份，Redis 不可达时透明降级，会话与计数照常工作。
- **存储**：`mysql` 不可连接时启动即报明确错误，不会静默丢数据。
- **数据隔离**：`-dir <目录>` 把 `config.json`、SQLite 库、缓存、产出全部收进该目录下的 `data/`，多实例互不干扰。

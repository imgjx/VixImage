# VixImage

[English](README.md) · 中文

一个用 Go 写的**自循环图片编辑 Agent 系统**：给一个提示词，多模态大模型进入 `观察 → 决策 → 执行` 自循环，每轮输出一个 `{think, tools[]}`，执行器严格解析并逐条执行一批**全部与图像工作有关**的技能，由 **Go 原生渲染引擎**合成成片。它有明确的**完成信号**（`finish`），也能反复**查看自己的作品**（`view`）后再改。

核心是**排版完全由 LLM 自由规划、不套模板**：画布尺寸（竖屏 / 横版 / 方形）、每个图层的位置与样式、参考图的编辑与嵌入都由模型自己决定。

## 特性

- **自循环 Agent** — 每次大模型请求 = 1 步；状态跨轮累积；一轮一批工具；链底工具 `view`/`finish`；交付前系统强制回看成片自检。
- **纯图像技能** — `canvas`、`gen`、`stock`、`qrcode`、`qrdecode`、`ocr`、`htmlrender`、`add/set/del/clear`、`layers`、`copy`、`undo`、`view`、`finish`。详见[技能与图层](docs/zh/skills.md)。
- **免版权素材库** — `stock` 技能从 Openverse（免密钥）、Pexels、Pixabay 搜索真实可授权图片并嵌入海报。
- **OCR 与 HTML 渲染** — `ocr` 用 PaddleOCR 识别图片文字（文本回灌模型）；`htmlrender` 用无头 Chrome 把 HTML 精确渲染成图片产物。
- **主动创意构图** — 把参考图变成画面里的物件（斜放屏幕 / 相框 / 画中画），用 `persp`/`shear` 做透视贴合。
- **广泛的上游兼容** — OpenAI Images、Gemini、Stability、OpenAI Chat（流式）、Anthropic。详见[上游兼容格式](docs/zh/upstream-formats.md)。
- **有容错、无臆造** — 解析容错、429 退避、单轮容错重输出、索引越界跳过；硬错误仍直接报错。详见[自循环工作流](docs/zh/agent-workflow.md)。
- **Web 控制台** — Vue 3 + vue-i18n 单页应用（中/英），源码在 `front/`，构建后嵌入单二进制。
- **双缓存 / 双存储** — File/Redis 与 SQLite/MySQL，配置可切换。
- **REST API** — `POST /v1/agents/run`，apikey 鉴权，可选 `trace`。详见 [API 文档](docs/zh/api.md)。

## 文档

| 主题 | 中文 | English |
| --- | --- | --- |
| 快速开始与 Docker | [快速开始](docs/zh/getting-started.md) | [Getting Started](docs/en/getting-started.md) |
| 自循环工作流 | [自循环工作流](docs/zh/agent-workflow.md) | [Agent Workflow](docs/en/agent-workflow.md) |
| 技能与图层 | [技能与图层](docs/zh/skills.md) | [Skills](docs/en/skills.md) |
| 上游兼容格式 | [上游兼容格式](docs/zh/upstream-formats.md) | [Upstream Formats](docs/en/upstream-formats.md) |
| REST API | [API 文档](docs/zh/api.md) | [API Reference](docs/en/api.md) |
| CLI 与配置 | [CLI 与配置](docs/zh/cli-and-config.md) | [CLI & Config](docs/en/cli-and-config.md) |

## 目录结构

```
viximage/
├── main.go                  # 入口 + CLI（user / apikey / config）
├── front/                   # Vue 3 + vue-i18n 控制台源码（构建进 internal/gui/web/）
├── internal/
│   ├── config/              # 配置（端口 / 账号 / apikey / 双存双缓）
│   ├── storage/             # Storage 接口 + SQLite / MySQL
│   ├── cache/               # Cache 接口 + File / Redis
│   ├── client/              # 上游图片/LLM HTTP 客户端
│   ├── upstream/            # 上游模型管理
│   ├── agentmodel/          # 「我的模型」（步数预算 / 耗尽策略 / 风格）
│   ├── agent/               # 自循环 Agent 运行时
│   │   └── skills/          # 生产技能 + Go 原生渲染引擎
│   ├── api/                 # API 服务（apikey 鉴权、trace）
│   └── gui/                 # Web 控制台（内嵌构建好的 Vue 资源）
└── docs/                    # 文档（en/ 与 zh/）
```

## 快速开始

```bash
GOSUMDB=off go build -o viximage .
./viximage                     # GUI :8080，API :8090
```

默认账号 `admin` / `admin123`，默认不预置 apikey。详见[快速开始](docs/zh/getting-started.md)。

## 构建前端

```bash
cd front
pnpm install
pnpm run build       # 输出到 ../internal/gui/web/，由 Go 嵌入
```

## 许可

见 [LICENSE](LICENSE)。

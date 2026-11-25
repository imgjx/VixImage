# VixImage

English · [中文](README_zh.md)

A **self-looping image-editing Agent written in Go**: give it a prompt, and a multimodal LLM enters an `observe → decide → act` loop, emitting one `{think, tools[]}` per turn. The executor strictly parses and runs a batch of **image-only** skills, and a native Go render engine composes the final image. It has an explicit completion signal (`finish`) and can repeatedly **view its own work** (`view`) before revising.

Layout is fully planned by the LLM — no templates. Canvas size (portrait / landscape / square), every layer's position and style, and reference-image editing/embedding are all decided by the model, so the same Agent produces genuinely different layouts for different tasks.

## Features

- **Self-looping Agent** — each turn = one LLM request; state accumulates across turns; a batch of tools per turn; bottom tools `view`/`finish`; system-enforced visual self-check before delivery.
- **Image-only skills** — `canvas`, `gen`, `stock`, `qrcode`, `qrdecode`, `ocr`, `htmlrender`, `svgrender`, `add/set/del/clear`, `layers`, `copy`, `undo`, `view`, `finish`. See [Skills](docs/en/skills.md).
- **Royalty-free stock library** — the `stock` skill searches Openverse (no key), Pexels and Pixabay for real, licensable images and embeds them into posters.
- **OCR & HTML/SVG rendering** — `ocr` recognizes text in images via PaddleOCR (fed back to the model); `htmlrender`/`svgrender` precisely render HTML/SVG into image assets with headless Chrome.
- **Precision positioning** — layers support nine-grid anchors `pos` (auto center/edge with safe insets), percentage coordinates `px/py/pw/ph`, and pixel offsets `ox/oy`, so the model never has to mentally compute large pixel values — dramatically more accurate placement with no offsets or overflow.
- **Proactive creative composition** — reference images become objects inside a scene (slanted screens, frames, picture-in-picture) with `persp`/`shear` perspective fitting.
- **Broad upstream compatibility** — OpenAI Images, Gemini, Stability, OpenAI Chat (streaming) and Anthropic. See [Upstream Formats](docs/en/upstream-formats.md).
- **Tolerant, never fabricating** — parse tolerance, 429 backoff, same-turn retry on bad output, index-skip; hard errors remain hard. See [Agent Workflow](docs/en/agent-workflow.md).
- **Web console** — Vue 3 + vue-i18n SPA (zh/en) in `front/`, embedded into the single binary.
- **Dual cache / dual storage** — File/Redis and SQLite/MySQL, switchable via config.
- **REST API** — `POST /v1/agents/run` with apikey auth and optional `trace`. See [API Reference](docs/en/api.md).

## Documentation

| Topic | English | 中文 |
| --- | --- | --- |
| Quick start & Docker | [Getting Started](docs/en/getting-started.md) | [快速开始](docs/zh/getting-started.md) |
| Self-loop workflow | [Agent Workflow](docs/en/agent-workflow.md) | [自循环工作流](docs/zh/agent-workflow.md) |
| Skills & layers | [Skills](docs/en/skills.md) | [技能与图层](docs/zh/skills.md) |
| Upstream compatibility | [Upstream Formats](docs/en/upstream-formats.md) | [上游兼容格式](docs/zh/upstream-formats.md) |
| REST API | [API Reference](docs/en/api.md) | [API 文档](docs/zh/api.md) |
| CLI & configuration | [CLI & Config](docs/en/cli-and-config.md) | [CLI 与配置](docs/zh/cli-and-config.md) |

## Project Layout

```
viximage/
├── main.go                  # entry + CLI (user / apikey / config)
├── front/                   # Vue 3 + vue-i18n console source (built into internal/gui/web/)
├── internal/
│   ├── config/              # config (ports / account / apikey / storage & cache modes)
│   ├── storage/             # Storage interface + SQLite / MySQL
│   ├── cache/               # Cache interface + File / Redis
│   ├── client/              # upstream image/LLM HTTP client (OpenAI/Gemini/Stability/Anthropic)
│   ├── upstream/            # upstream model management
│   ├── agentmodel/          # "My Models" (step budget / exhaustion / style)
│   ├── agent/               # self-loop Agent runtime
│   │   └── skills/          # production skills + native Go render engine
│   ├── api/                 # API service (apikey auth, trace)
│   └── gui/                 # web console (embeds built Vue assets)
└── docs/                    # documentation (en/ + zh/)
```

## Quick Start

```bash
GOSUMDB=off go build -o viximage .
./viximage                     # GUI on :8080, API on :8090
```

Default account `admin` / `admin123`. No API key is pre-provisioned. See [Getting Started](docs/en/getting-started.md) for details.

## Build the front-end

```bash
cd front
pnpm install
pnpm run build       # outputs to ../internal/gui/web/ (embedded by Go)
```

## License

See [LICENSE](LICENSE).

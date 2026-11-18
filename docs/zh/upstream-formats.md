# 上游兼容格式

VixImage 本身是**图片生成系统**，上游以**图片生成接口**为主，同时兼容常见**文本接口**（含流式），不绑定任何私有格式。在「上游模型」页按类型选择 `format` 即可，旧版本私有 `agnes` 格式会自动迁移为 `openai`。

| `format` | 适用 | 对应上游协议 | 端点 |
| --- | --- | --- | --- |
| `openai` | 图片 | OpenAI Images（兼容绝大多数第三方网关 / 中转） | `POST {base}/v1/images/generations` |
| `gemini` | 图片 / 文本 | Google Gemini `generateContent` | `POST {base}/v1beta/models/{model}:generateContent` |
| `stability` | 图片 | Stability AI 稳定扩散 | `POST {base}/v2beta/stable-image/generate/core` |
| `openai_chat` | 文本 | OpenAI Chat（规划器，**支持 SSE 流式**，每一步都能看到） | `POST {base}/v1/chat/completions` |
| `anthropic` | 文本 | Anthropic Claude Messages | `POST {base}/v1/messages` |

要点：

- **图片接口**：自动请求 `b64_json`，兼容 `data[].url / b64_json / images / output` 等多种回包；图生图 `ref` 参考图会自动转 data URI；`base` 末尾带 `/v1` 时自动去重，不会出现 `/v1/v1/`。
- **文本接口**：`openai_chat` 支持 `stream` 增量回调；Gemini / Anthropic 用各自原生协议适配，并解析出文本与内联图片。
- 所有上游请求统一带浏览器 UA，规避部分网关对非浏览器客户端的拦截。

## 按类型分专属字段

- **图片模型**：默认尺寸 / 多尺寸 `sizes` / 张数 `n` / 单次步数 `steps` / 反向提示词 `negative_prompt` / 引擎介绍 `agent_desc`。
- **大模型 LLM**：`max_tokens`（输出上限）/ `context_limit`（输入字符上限）/ `temperature` / `top_p` / `system_prompt`（系统提示词前缀）。两类字段互不串台。

小上下文模型务必配置 `max_tokens`（建议 ≥ 2400）。`context_limit=0` 表示不限制；设为正数时按字符预算裁剪较早消息、保留图片与最新用户要求。

# Upstream Compatibility

VixImage is an **image generation system**: upstreams are primarily image-generation APIs, with common text APIs (including streaming) also supported. It is not tied to any private format. Pick `format` by type on the Upstream Models page; the legacy private `agnes` format is auto-migrated to `openai`.

| `format` | Use | Protocol | Endpoint |
| --- | --- | --- | --- |
| `openai` | image | OpenAI Images (and most third-party gateways) | `POST {base}/v1/images/generations` |
| `gemini` | image / text | Google Gemini `generateContent` | `POST {base}/v1beta/models/{model}:generateContent` |
| `stability` | image | Stability AI | `POST {base}/v2beta/stable-image/generate/core` |
| `openai_chat` | text | OpenAI Chat (planner, **SSE streaming**, every step visible) | `POST {base}/v1/chat/completions` |
| `anthropic` | text | Anthropic Claude Messages | `POST {base}/v1/messages` |

Notes:

- **Image APIs**: requests `b64_json` and tolerates `data[].url / b64_json / images / output` response shapes; image-to-image `ref` is converted to a data URI; a `base` ending in `/v1` is de-duplicated (no `/v1/v1/`).
- **Text APIs**: `openai_chat` supports incremental `stream` callbacks; Gemini / Anthropic use their native protocols, extracting text and inline images.
- All upstream requests carry a browser-like User-Agent to avoid gateway blocks on non-browser clients.

## Type-Specific Fields

- **Image model**: default size / `sizes` / count `n` / per-image steps `steps` / `negative_prompt` / `agent_desc`.
- **LLM**: `max_tokens` (output cap) / `context_limit` (input character budget) / `temperature` / `top_p` / `system_prompt` (prefix). The two sets never mix.

For small-context models always set `max_tokens` (recommend ≥ 2400). `context_limit=0` means unlimited; a positive value trims older messages by character budget while keeping images and the newest request.

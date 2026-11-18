# API Reference

The API service listens on `:8090` by default (configurable in Settings or via `-api-port`). **All endpoints except `/health` require authentication**; failures return `401 {"error":"invalid or missing apikey"}`.

## Authentication

Send the key one of two ways:

| Method | Form |
| --- | --- |
| Header | `X-API-Key: vix-sk-xxxx` |
| Header | `Authorization: Bearer vix-sk-xxxx` |

Keys are created on the API Keys page or via CLI `viximage apikey add`; **none is pre-provisioned**. Every successful call increments that key's `calls` counter and last-used time.

## Health Check

`GET /health` → `{ "status": "ok", "service": "viximage-api", "port": 8090 }`

## List My Models

`GET /v1/agents` → `{ "agents": [...] }`

```bash
curl -H "X-API-Key: vix-sk-xxxx" http://localhost:8090/v1/agents
```

## Run an Agent (core endpoint)

`POST /v1/agents/run` · `Content-Type: application/json`

| Field | Type | Description |
| --- | --- | --- |
| `call_name` | string | **Required**, Agent call name (see `/v1/agents`) |
| `prompt` | string | **Required**, creative instruction |
| `images` | string[] | optional reference images (`/v1/upload` ids or http(s) URLs) |
| `webhook` | string | optional completion callback URL |
| `deep_think` | bool | optional, force deep thinking for this run |
| `size` | string | optional, force size such as `"1280x720"` |
| `trace` | bool | optional, append short per-step ThinkFlags |

```bash
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"A vertical sci-fi movie poster, glowing planet, big title","trace":true}'
```

Success response:

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
  "log": ["..."],
  "reasoning": "...",
  "trace": [ { "step": 1, "think": "..." } ]
}
```

`status` values: `success` / `success_partial` (steps exhausted but an image was produced) / `failed`. `trace[]` and `reasoning` are returned only with `trace:true`.

## Upload Input Images

`POST /v1/upload` · `multipart/form-data`, field `file` (multiple allowed) → `{ "uploaded": ["up_xxx.png"] }`. Feed an id into `/v1/agents/run` `images[]` as a reference image.

## Fetch Output Image

`GET /v1/files/{name}` — returns image bytes (requires API Key).

## List Upstream Models

`GET /v1/upstream` — returns configured upstream models (keys masked).

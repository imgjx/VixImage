# API 文档

API 服务默认监听 `:8090`（可在「系统配置」或启动参数 `-api-port` 修改）。**所有接口除 `/health` 外均需鉴权**；鉴权失败返回 `401 {"error":"invalid or missing apikey"}`。

## 鉴权方式

每个请求二选一携带密钥：

| 方式 | 写法 |
| --- | --- |
| 请求头 | `X-API-Key: vix-sk-xxxx` |
| 请求头 | `Authorization: Bearer vix-sk-xxxx` |

密钥在「API 密钥」页或 CLI `viximage apikey add` 创建，**默认不预置任何密钥**。每次成功调用都会累加该密钥的 `calls` 计数与最近调用时间。

## 健康检查

`GET /health` → `{ "status": "ok", "service": "viximage-api", "port": 8090 }`

## 列出我的模型

`GET /v1/agents` → `{ "agents": [...] }`

```bash
curl -H "X-API-Key: vix-sk-xxxx" http://localhost:8090/v1/agents
```

## 调起一个模型（核心接口）

`POST /v1/agents/run` · `Content-Type: application/json`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `call_name` | string | **必填**，模型调用名（见 `/v1/agents`） |
| `prompt` | string | **必填**，创作指令 |
| `images` | string[] | 可选，参考图（`/v1/upload` 返回的 id 或 http(s) 直链） |
| `webhook` | string | 可选，完成时回调 URL |
| `deep_think` | bool | 可选，本次强制开启深度思考 |
| `size` | string | 可选，强制尺寸如 `"1280x720"` |
| `trace` | bool | 可选，响应附带每步短 ThinkFlag |

```bash
curl -X POST http://localhost:8090/v1/agents/run \
  -H "X-API-Key: vix-sk-xxxx" -H 'Content-Type: application/json' \
  -d '{"call_name":"poster","prompt":"做一张竖版科幻电影海报，中心发光星球，顶部大标题","trace":true}'
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
  "log": ["..."],
  "reasoning": "...",
  "trace": [ { "step": 1, "think": "..." } ]
}
```

`status` 取值：`success` / `success_partial`（步数耗尽但已出图） / `failed`。`trace:true` 时才会返回 `trace[]` 与 `reasoning`。

## 上传输入图

`POST /v1/upload` · `multipart/form-data`，字段名 `file`（可多文件）→ `{ "uploaded": ["up_xxx.png"] }`。把 id 填进 `/v1/agents/run` 的 `images[]` 即作为参考图。

## 取回产出图

`GET /v1/files/{name}` —— 返回图片二进制（需 API Key）。

## 上游模型列表

`GET /v1/upstream` —— 返回已配置的上游模型（密钥字段已掩码）。

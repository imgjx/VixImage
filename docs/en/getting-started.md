# Getting Started

## Build & Run

```bash
GOSUMDB=off go build -o viximage .
./viximage                                       # start GUI + API, config in ./config.json
./viximage -dir /data/vix                        # custom config/data dir (data goes into data/)
./viximage -config /etc/vix/config.json          # explicit config file (overrides -dir)
./viximage -gui-port 9000 -api-port 9001         # temporary port override (not persisted)
./viximage -version                              # print version
```

Flags:

- `-dir <dir>`: config/data root (default `.`; generates `config.json` on first run)
- `-config <file>`: explicit config path, higher priority than `-dir`
- `-gui-port <port>` / `-api-port <port>`: temporary override (0 = use config)
- The CLI prints startup info and per-request access logs (method / path / status / duration) at runtime.

GUI console defaults to `http://localhost:8080` (login required); API service defaults to `http://localhost:8090` (apikey auth).

> Login: unauthenticated requests redirect (302) to `/login`; the login page is a plain HTML form (server-rendered errors, no JavaScript needed) so old browsers work. Sessions support Cookie / `X-Vix-Sid` header / URL token. The main console is a Vue 3 + vue-i18n SPA with a language switcher in the top-right.

## Docker

A multi-stage `Dockerfile` is provided (build: golang:1.21-alpine; run: alpine with optional Python/PIL engine and CJK fonts).

```bash
docker build -t viximage:0.2.0 --build-arg VERSION=0.2.0 .
docker run -d --name viximage \
  -p 8080:8080 -p 8090:8090 \
  -v viximage-data:/data \
  viximage:0.2.0
```

Or with compose:

```bash
docker compose up -d
```

- Console: http://localhost:8080　API: http://localhost:8090
- Data (config / database / cache / outputs) persists under `/data` in the `viximage-data` volume
- Default account `admin` / `admin123`; change it in Settings after login
- Go module proxy fallback is configured so builds work behind restrictive networks

Default account: `admin` / `admin123`. **No API key is pre-provisioned** — create one on the API Keys page or via `viximage apikey add`.

## First-use flow

1. Add an image model and a multimodal LLM under Upstream Models.
2. Create an Agent under My Models: pick a base model, set the step budget, add image engines and per-image step cost.
3. Call `POST /v1/agents/run` with `call_name` + `prompt`; see the [API docs](api.md).
4. Use Playground to try the full self-loop in the console.

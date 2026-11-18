# CLI & Configuration

## CLI (manage without starting servers)

```bash
./viximage user set --username admin --password NewPass   # change GUI account/password
./viximage user set --username NewName                  # change username only
./viximage apikey add                                     # new key (auto-generated, shown once)
./viximage apikey add vix-sk-xxxx                         # new key with a custom value (must be unique)
./viximage apikey list                                    # list all keys (with call counts)
./viximage apikey rm <id|key>                             # delete a key (by id or full value)
./viximage config                                         # print current config
```

## Console Pages

The console is a Vue 3 + vue-i18n SPA with a language switcher in the top-right; source lives in `front/`.

- **Overview**: total / success / failed / images / total steps, paginated logs, per-step ThinkFlags and operation trace
- **Upstream Models**: type-specific fields for image models and multimodal LLMs
- **My Models**: create/edit Agents (base model + step budget + per-image step cost + style + deep thinking + exhaustion policy)
- **API Keys**: standalone key management with per-key call statistics
- **Settings**: ports, dual storage/cache, GUI account/password, image engine, stock library keys, delete-output toggle
- **Playground**: run a full Agent self-loop with reference images, view logs and per-step thinking

## Configuration Fields

| Group | Fields |
| --- | --- |
| Ports | `gui_port`(8080), `api_port`(8090) |
| Storage | `storage_mode`(sqlite/mysql), `sqlite_path`, `mysql_dsn` |
| Cache | `cache_mode`(file/redis), `redis_addr`, `redis_pass`, `redis_db`, `cache_file_dir` |
| Image engine | `image_engine`(go/python), `python_bin` |
| Stock library | `stock_openverse_base`, `stock_pexels_key`, `stock_pixabay_key` |
| Outputs | `delete_outputs` (delete the finished image after export) |

> Storage / cache mode changes apply after restart. Sessions live in process memory and do not depend on a writable cache directory, so login works even when installed under Program Files on Windows.

## Dual Cache / Dual Storage

- **Cache**: `redis` mode wraps a File fallback; if Redis is unreachable it degrades transparently.
- **Storage**: if `mysql` cannot connect, startup fails with a clear error (no silent data loss).
- **Data isolation**: `-dir <dir>` keeps `config.json`, SQLite DB, cache and outputs inside that directory's `data/`, so multiple instances stay isolated.

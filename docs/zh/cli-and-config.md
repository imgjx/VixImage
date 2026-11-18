# CLI 与系统配置

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

控制台为 Vue 3 + vue-i18n 单页应用，右上角可切换中/英界面；源码在 `front/` 目录。

- **调用概述**：总调用 / 成功 / 失败 / 产出图 / 累计步数，分页流水，详情含每步 ThinkFlag 与操作轨迹
- **上游模型**：按类型分专属字段 —— 图片模型与多模态大模型
- **我的模型**：创建 / 编辑 Agent（基架模型 + 总步数 + 图片模型单次消耗步数 + 风格 + 深度思考 + 耗尽策略）
- **API 密钥**：独立密钥管理页，可创建多个密钥、分别统计调用次数
- **系统配置**：端口、双存储 / 双缓存、GUI 账号与密码、图像引擎、素材库密钥、提取后删除成品
- **Playground**：选「我的模型」跑完整 Agent 自循环，可传参考图，查看日志与每步思考

## 配置字段说明

| 分组 | 字段 |
| --- | --- |
| 端口 | `gui_port`(8080)、`api_port`(8090) |
| 存储 | `storage_mode`(sqlite/mysql)、`sqlite_path`、`mysql_dsn` |
| 缓存 | `cache_mode`(file/redis)、`redis_addr`、`redis_pass`、`redis_db`、`cache_file_dir` |
| 图像引擎 | `image_engine`(go/python)、`python_bin` |
| 素材库 | `stock_openverse_base`、`stock_pexels_key`、`stock_pixabay_key` |
| 产物 | `delete_outputs`（提取后是否删除成品） |

> 存储 / 缓存模式改动重启后生效。会话保存在进程内存中，不依赖磁盘缓存目录可写，Windows 装在 Program Files 等场景也能正常登录。

## 双缓存 / 双存储

- **缓存**：`redis` 模式自动包裹一层 File 备份，Redis 不可达时透明降级。
- **存储**：`mysql` 不可连接时启动即报明确错误，不会静默丢数据。
- **数据隔离**：`-dir <目录>` 把 `config.json`、SQLite 库、缓存、产出全部收进该目录下的 `data/`，多实例互不干扰。

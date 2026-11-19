# 快速开始

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

GUI 控制台默认 `http://localhost:8080`（账号密码登录），API 服务默认 `http://localhost:8090`（apikey 鉴权）。

> 登录：未登录时 302 跳到登录页；登录页是原生 HTML 表单提交（服务端渲染错误提示，不依赖 JavaScript），老旧浏览器同样可用。会话同时支持 Cookie / `X-Vix-Sid` 请求头 / URL 令牌三种携带方式。控制台主体为 Vue 3 + vue-i18n 单页应用，右上角可切换中/英界面。

## Docker 部署

仓库根目录已提供多阶段 `Dockerfile`（编译用 golang:1.21-alpine，运行用 alpine，含 Python/PIL 可选引擎与中文字体）。

```bash
docker build -t viximage:0.2.1 --build-arg VERSION=0.2.1 .
docker run -d --name viximage \
  -p 8080:8080 -p 8090:8090 \
  -v viximage-data:/data \
  viximage:0.2.1
```

也可以直接用 compose：

```bash
docker compose up -d
```

- 控制台：http://localhost:8080　API：http://localhost:8090
- 数据（配置 / 数据库 / 缓存 / 作品）持久化在 `viximage-data` 卷的 `/data` 下
- 默认账号 `admin` / `admin123`，登录后在「系统配置」里修改
- Dockerfile 已配置 Go 模块代理回退（goproxy.cn → goproxy.io → 官方），国内网络构建不会卡住

默认账号：`admin` / `admin123`。**默认不预置任何 apikey**，首次启动请到「API 密钥」页或 `viximage apikey add` 创建。

## 首次使用流程

1. 在「上游模型」添加一个图片模型和一个多模态大模型。
2. 在「我的模型」创建 Agent：选基座、设总步数预算、添加图片模型并设每次生图消耗步数。
3. 用 API `POST /v1/agents/run` 传 `call_name` + `prompt` 调起，详见 [API 文档](api.md)。
4. 用 Playground 直接在控制台试玩完整自循环链路。

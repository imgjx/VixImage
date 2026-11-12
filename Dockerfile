# ============================================================
# VixImage —— 多阶段构建
#   阶段 1 builder：编译静态二进制（CGO_ENABLED=0）
#   阶段 2 runtime ：最小运行镜像，仅含二进制与 Python/PIL（可选编辑引擎回退）
# ============================================================

# ---------- 阶段 1：编译 ----------
FROM golang:1.21-alpine AS builder

# git 用于可能的私有依赖拉取；ca-certificates 用于 HTTPS 代理
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Go 模块代理：默认官方源在国内/受限网络常常超时，这里按 goproxy.cn →
# goproxy.io → 官方 的顺序回退，避免构建卡死。
ARG GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=sum.golang.google.cn,sum.golang.org

# 先只拷依赖清单，最大化利用镜像层缓存
COPY go.mod go.sum ./
RUN go mod download

# 再拷源码并编译
COPY . .

ARG VERSION=0.1.0
# -s -w 去符号表与调试信息，减小体积
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X main.Version=${VERSION}" \
      -o /out/viximage .

# ---------- 阶段 2：运行 ----------
FROM alpine:3.20

# ca-certificates：出网 HTTPS（调用模型 API）
# tzdata：时区
# python3 + py3-pillow：可选的「python」编辑引擎回退（默认用 Go 引擎）
# fonts-noto-cjk + fontconfig：中文文字渲染，避免中文变方块
# bash：调试便利
RUN apk add --no-cache \
      ca-certificates \
      tzdata \
      python3 \
      py3-pillow \
      fontconfig \
      font-noto-cjk \
      bash \
 && fc-cache -f

# 以非 root 运行
RUN addgroup -S vix && adduser -S -G vix vix

WORKDIR /app
COPY --from=builder /out/viximage /app/viximage

# 数据目录：配置、数据库、缓存、产物、上传。
# 用卷挂载到宿主机可持久化数据（容器重建不丢配置与作品）。
VOLUME ["/data"]
ENV HOME=/data

# 8080 = GUI 控制台，8090 = API 服务
EXPOSE 8080 8090

USER vix

# 用数据目录作为工作目录；配置文件落在 /data/config.json
WORKDIR /data

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:8080/login" >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/viximage"]
CMD ["-dir", "/data"]

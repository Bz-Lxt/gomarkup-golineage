# syntax=docker/dockerfile:1

# ---------------------------------------------------------------- #
# 构建阶段
# ---------------------------------------------------------------- #
FROM golang:1.25-alpine AS builder

# 国内网络下不走镜像源会导致依赖拉取极慢甚至超时。
# 这些配置写在镜像内而非依赖宿主环境变量，保证任何机器上构建行为一致。
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off \
    CGO_ENABLED=0

WORKDIR /build

# 先只拷贝依赖清单，让依赖层在源码变动时仍能命中缓存。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath 去除构建机的绝对路径，-s -w 去掉符号表与调试信息，显著减小体积。
# TARGETOS / TARGETARCH 由 buildx 注入，使同一份 Dockerfile 可产出 amd64 与 arm64 镜像。
ARG TARGETOS=linux
ARG TARGETARCH
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/golineage ./cmd/server

# 构建期自检：镜像内跑一遍单元测试，避免把逻辑已损坏的产物发布出去。
RUN go test ./internal/graph/ ./internal/eventstore/ ./internal/api/ 2>&1 | tail -20

# ---------------------------------------------------------------- #
# 运行阶段
# ---------------------------------------------------------------- #
FROM alpine:3.21

# 换用国内镜像源，否则 apk 在国内网络下常常卡住。
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata wget && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 以非 root 身份运行，缩小容器逃逸时的影响面。
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=builder /out/golineage /app/golineage

USER app
ENV TZ=Asia/Shanghai \
    HTTP_ADDR=:8080

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=12 \
    CMD wget -q -O- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/golineage"]

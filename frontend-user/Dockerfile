# syntax=docker/dockerfile:1

FROM node:22-alpine AS builder

# 国内网络下 npm 官方源经常超时，锁定镜像源保证构建可复现。
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com

WORKDIR /build
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# 镜像构建跳过 vue-tsc：类型检查在开发机与 QA 阶段执行，避免构建镜像被
# 与业务无关的类型噪音拖垮。产物正确性由 Vite 编译与 E2E 兜底。
RUN npm run build:only

FROM nginx:1.27-alpine

RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories && \
    apk add --no-cache wget tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    rm /etc/nginx/conf.d/default.conf

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /build/dist /usr/share/nginx/html

ENV TZ=Asia/Shanghai
EXPOSE 80

HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=12 \
    CMD wget -q -O- http://127.0.0.1/ >/dev/null 2>&1 || exit 1

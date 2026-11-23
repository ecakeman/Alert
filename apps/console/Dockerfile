# syntax=docker/dockerfile:1

# 基础镜像可用 --build-arg NODE_IMAGE=... / NGINX_IMAGE=... 覆盖（Compose 见 compose.yaml build.args）。
ARG NODE_IMAGE=node:22-alpine
ARG NGINX_IMAGE=nginx:1.27-alpine

# ---------- 构建阶段：Node 编译前端 ----------
FROM ${NODE_IMAGE} AS build
WORKDIR /app
# 国内网络使用镜像源，可通过 --build-arg NPM_REGISTRY=... 覆盖
ARG NPM_REGISTRY=https://registry.npmmirror.com
COPY package.json package-lock.json ./
RUN npm ci --registry="${NPM_REGISTRY}" --no-audit --no-fund
COPY . .
RUN npm run build

# ---------- 运行阶段：仅静态文件 + Nginx ----------
FROM ${NGINX_IMAGE}
COPY nginx.conf /etc/nginx/nginx.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
# 标准 nginx 镜像默认以 root 启动 master、以 nginx 用户运行 worker。

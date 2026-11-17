# Go Backend：构建阶段编译静态二进制，运行阶段仅保留二进制与健康检查工具。
# 基础镜像可用 --build-arg GO_IMAGE=... / DEBIAN_IMAGE=... 覆盖（Compose 见 compose.yaml build.args）。
ARG GO_IMAGE=golang:1.25.7-bookworm
ARG DEBIAN_IMAGE=debian:bookworm-slim
FROM ${GO_IMAGE} AS build
WORKDIR /src
# 国内网络默认走 goproxy.cn；如需官方源可通过 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO 关闭后产物为静态二进制，不依赖运行镜像的 libc。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/system-core ./cmd/system-core

FROM ${DEBIAN_IMAGE}
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 alertflow \
    && useradd --system --uid 10001 --gid alertflow --no-create-home --shell /usr/sbin/nologin alertflow
COPY --from=build /out/system-core /usr/local/bin/system-core
USER 10001:10001
EXPOSE 9090
HEALTHCHECK --interval=2s --timeout=3s --start-period=10s --retries=30 \
    CMD ["system-core", "health"]
ENTRYPOINT ["system-core"]

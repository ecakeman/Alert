# syntax=docker/dockerfile:1

# 构建上下文为仓库根（compose 中 context: ../..）。
ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY services/platform services/platform
COPY services/console-bff services/console-bff
WORKDIR /src/services/console-bff
ENV GOWORK=off CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/console-bff ./cmd/console-bff

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S alert && adduser -S alert -G alert
WORKDIR /app
COPY --from=build /out/console-bff /app/console-bff
USER alert
ENV TZ=UTC
EXPOSE 8080
ENTRYPOINT ["/app/console-bff"]
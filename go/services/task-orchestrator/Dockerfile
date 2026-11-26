# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY platform platform
COPY contracts contracts
COPY services/task-orchestrator services/task-orchestrator
WORKDIR /src/services/task-orchestrator
ENV GOWORK=off CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
RUN go build -trimpath -ldflags="-s -w" -o /out/task-orchestrator ./cmd/task-orchestrator

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S alert && adduser -S alert -G alert
WORKDIR /app
COPY --from=build /out/task-orchestrator /app/task-orchestrator
RUN mkdir -p /data/artifacts && chown -R alert:alert /data
USER alert
ENV TZ=UTC
EXPOSE 8084 9090
ENTRYPOINT ["/app/task-orchestrator"]
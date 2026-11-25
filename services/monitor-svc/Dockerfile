# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY services/platform services/platform
COPY services/monitor-svc services/monitor-svc
WORKDIR /src/services/monitor-svc
ENV GOWORK=off CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
RUN go build -trimpath -ldflags="-s -w" -o /out/monitor-svc ./cmd/monitor-svc

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S alert && adduser -S alert -G alert
WORKDIR /app
COPY --from=build /out/monitor-svc /app/monitor-svc
USER alert
ENV TZ=UTC
EXPOSE 8083
ENTRYPOINT ["/app/monitor-svc"]
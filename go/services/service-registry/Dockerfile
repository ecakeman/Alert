# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY platform platform
COPY services/service-registry services/service-registry
WORKDIR /src/services/service-registry
ENV GOWORK=off CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
RUN go build -trimpath -ldflags="-s -w" -o /out/service-registry ./cmd/service-registry

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S alert && adduser -S alert -G alert
WORKDIR /app
COPY --from=build /out/service-registry /app/service-registry
USER alert
ENV TZ=UTC
EXPOSE 8081
ENTRYPOINT ["/app/service-registry"]
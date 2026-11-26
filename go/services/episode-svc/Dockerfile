# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY platform platform
COPY contracts contracts
COPY services/episode-svc services/episode-svc
WORKDIR /src/services/episode-svc
ENV GOWORK=off CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct
RUN go build -trimpath -ldflags="-s -w" -o /out/episode-svc ./cmd/episode-svc

FROM ${RUNTIME_IMAGE}
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S alert && adduser -S alert -G alert
WORKDIR /app
COPY --from=build /out/episode-svc /app/episode-svc
USER alert
ENV TZ=UTC
EXPOSE 8082
ENTRYPOINT ["/app/episode-svc"]
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build

ARG GOPROXY=https://proxy.golang.org,direct
ARG VERSION=1.6.4
ENV CGO_ENABLED=0 GOPROXY=${GOPROXY} GOSUMDB=off

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pulse ./cmd/pulse

# Real proxy handshake engine. Pin the tested release and allow a mirror/override
# for networks where GitHub release downloads are blocked.
FROM alpine:3.22 AS xray-core
ARG XRAY_VERSION=v26.11.9
ARG TARGETARCH=amd64
ARG XRAY_DOWNLOAD_URL=
RUN apk add --no-cache curl unzip ca-certificates && \
    mkdir -p /out && \
    if [ -n "$XRAY_DOWNLOAD_URL" ]; then \
      URL="$XRAY_DOWNLOAD_URL"; \
    else \
      case "$TARGETARCH" in \
        arm64) ASSET="Xray-linux-arm64-v8a.zip" ;; \
        *) ASSET="Xray-linux-64.zip" ;; \
      esac; \
      URL="https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${ASSET}"; \
    fi; \
    echo "Downloading Xray: $URL" && \
    curl -fsSL --retry 5 --retry-delay 2 "$URL" -o /tmp/xray.zip && \
    unzip -q /tmp/xray.zip -d /out

FROM alpine:3.22

RUN apk add --no-cache ca-certificates iputils tzdata wget

COPY --from=build /out/pulse /usr/local/bin/pulse
COPY --from=xray-core /out/xray /usr/local/bin/xray
WORKDIR /data
ENV PULSE_DB_PATH=/data/pulse.db
EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

# The container runs as root so the ICMP ping monitor works with NET_RAW.
# Xray is executed as the same process user and only listens on 127.0.0.1.
ENTRYPOINT ["pulse"]
CMD ["--addr", ":8080"]

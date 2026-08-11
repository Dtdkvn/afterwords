# syntax=docker/dockerfile:1.7

FROM node:24-alpine@sha256:d32cdf619f63fe0471182d08996dd516c6275bb5fd31ae06e55a570bd9e1ad43 AS frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
WORKDIR /src/web
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=secret,id=afterwords_ca,required=false \
    if [ -f /run/secrets/afterwords_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/afterwords_ca; fi; \
    npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS backend
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN --mount=type=secret,id=afterwords_ca,required=false \
    set -eu; \
    if [ -f /run/secrets/afterwords_ca ]; then \
      cat /etc/ssl/certs/ca-certificates.crt /run/secrets/afterwords_ca > /tmp/afterwords-build-ca.pem; \
      export SSL_CERT_FILE=/tmp/afterwords-build-ca.pem; \
    fi; \
    apk add --no-cache ca-certificates git; \
    rm -f /tmp/afterwords-build-ca.pem
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=afterwords_ca,required=false \
    set -eu; \
    if [ -f /run/secrets/afterwords_ca ]; then \
      cat /etc/ssl/certs/ca-certificates.crt /run/secrets/afterwords_ca > /tmp/afterwords-build-ca.pem; \
      export SSL_CERT_FILE=/tmp/afterwords-build-ca.pem; \
    fi; \
    go mod download; \
    rm -f /tmp/afterwords-build-ca.pem
COPY . .
COPY --from=frontend /src/internal/server/webdist ./internal/server/webdist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
      -o /out/afterwords ./cmd/afterwords

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN --mount=type=secret,id=afterwords_ca,required=false \
    set -eu; \
    if [ -f /run/secrets/afterwords_ca ]; then \
      cat /etc/ssl/certs/ca-certificates.crt /run/secrets/afterwords_ca > /tmp/afterwords-build-ca.pem; \
      export SSL_CERT_FILE=/tmp/afterwords-build-ca.pem; \
    fi; \
    apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 afterwords \
    && adduser -S -D -H -u 10001 -G afterwords afterwords \
    && install -d -o afterwords -g afterwords /data; \
    rm -f /tmp/afterwords-build-ca.pem
COPY --from=backend /out/afterwords /usr/local/bin/afterwords
USER afterwords
WORKDIR /data
ENV PORT=8080 \
    DB_PATH=/data/afterwords.db \
    SECURE_COOKIES=false
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/afterwords"]

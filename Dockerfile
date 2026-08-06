# syntax=docker/dockerfile:1
# ai-agent 多阶段构建：编译二进制 + 运行时含 Poppler / Tesseract（中文 OCR）

ARG GO_VERSION=1.25.5

# ---------- build ----------
FROM golang:${GO_VERSION}-bookworm AS builder
WORKDIR /src

ARG GOPROXY=
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS="-trimpath" \
    GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /out/ai-agent ./cmd/server

# ---------- runtime ----------
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-chi-sim \
    tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/ai-agent /app/ai-agent
COPY configs/config.docker.yaml /app/configs/config.yaml

RUN mkdir -p /app/logs /app/attachments

# 使用 bind mount 挂载 logs/attachments 时以 root 运行更省事；生产可改为非 root + named volume
ENV TZ=Asia/Shanghai

EXPOSE 18090

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:18090/health || exit 1

ENTRYPOINT ["/app/ai-agent"]
CMD ["-config", "/app/configs/config.yaml"]

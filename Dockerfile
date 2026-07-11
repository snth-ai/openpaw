FROM golang:1.26-bookworm AS builder

WORKDIR /app

# Go deps
COPY go.mod go.sum ./
RUN go mod download

# Build (pure Go — no CGO)
COPY . .
ENV CGO_ENABLED=0
RUN go build -o openpaw .

# Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates yt-dlp && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/openpaw .
# Default identity — SOUL.md is gitignored (per-synth), so a fresh clone must
# still build; override with your own via bind mount at /app/SOUL.md
COPY SOUL.example.md ./SOUL.md
# Static assets served at runtime: /graph page + built-in skills
COPY web ./web
COPY skills ./skills

RUN mkdir -p /app/data

EXPOSE 8080

CMD ["./openpaw"]

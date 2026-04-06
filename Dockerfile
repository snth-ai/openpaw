FROM golang:1.26-bookworm AS builder

WORKDIR /app

# LanceDB native lib
RUN apt-get update && apt-get install -y curl && \
    curl -sSL https://raw.githubusercontent.com/lancedb/lancedb-go/main/scripts/download-artifacts.sh | bash

# Go deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
ENV CGO_CFLAGS="-I/app/include"
ENV CGO_LDFLAGS="/app/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread"
RUN go build -o openpaw .

# Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates yt-dlp && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/openpaw .
COPY SOUL.md .

RUN mkdir -p /app/data /app/skills

EXPOSE 8080

CMD ["./openpaw"]

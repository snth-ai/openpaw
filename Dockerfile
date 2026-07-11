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
COPY SOUL.md .

RUN mkdir -p /app/data /app/skills

EXPOSE 8080

CMD ["./openpaw"]

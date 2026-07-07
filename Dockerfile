# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ledger-core ./cmd/api

# Migration binary
FROM alpine:3.19 AS migrate-builder
RUN wget -O /tmp/migrate.tar.gz https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz \
    && tar -xzf /tmp/migrate.tar.gz -C /usr/local/bin \
    && chmod +x /usr/local/bin/migrate

# Final image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/ledger-core /ledger-core
COPY --from=migrate-builder /usr/local/bin/migrate /usr/local/bin/migrate
COPY db/migrations /migrations
COPY start.sh /start.sh

RUN chmod +x /start.sh

EXPOSE 8080
ENTRYPOINT ["/start.sh"]

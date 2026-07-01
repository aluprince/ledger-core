# Multi-stage build — final image is lean with no Go toolchain.
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ledger-core ./cmd/api

# Final image — distroless for minimal attack surface.
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/ledger-core /ledger-core

EXPOSE 8080
ENTRYPOINT ["/ledger-core"]

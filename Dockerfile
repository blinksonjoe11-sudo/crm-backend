# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o crm ./cmd/server

# ── Runtime stage ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/crm /crm
EXPOSE 8080
ENTRYPOINT ["/crm"]

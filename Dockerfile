# ── Stage 1: Build ─────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Copy module files and download dependencies (uses pre-generated go.sum)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o golive .

# ── Stage 2: Run ───────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates wget

# Copy binary and assets
COPY --from=builder /app/golive    /golive
COPY --from=builder /app/templates /templates
COPY --from=builder /app/static    /static

EXPOSE 8080

ENTRYPOINT ["/golive"]

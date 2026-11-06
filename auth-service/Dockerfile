FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o auth-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/auth-service /auth-service
EXPOSE 8081
ENTRYPOINT ["/auth-service"]

FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod ./
RUN go mod tidy
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o frontend .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/frontend  /frontend
COPY --from=builder /app/templates /templates
COPY --from=builder /app/static    /static
EXPOSE 8080
ENTRYPOINT ["/frontend"]

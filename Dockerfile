FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /capy-server ./cmd/server

FROM alpine:3.19
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=builder /capy-server .
COPY --from=builder /app/docs ./docs
COPY --from=builder /app/schema.sql ./schema.sql
COPY --from=builder /app/migrations ./migrations
RUN adduser -D -g '' appuser
USER appuser
EXPOSE 8080
ENTRYPOINT ["./capy-server"]

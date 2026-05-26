# Stage 1: Build
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o qqgo-server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/qqgo-server .
EXPOSE 8080
ENV DB_PATH=/data/qqgo.db
VOLUME ["/data"]
CMD ["./qqgo-server"]

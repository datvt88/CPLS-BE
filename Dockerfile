# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first (caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary (templates are embedded via go:embed)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o main .

# Final stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Copy binary
COPY --from=builder /app/main .

EXPOSE 8080

CMD ["/app/main"]

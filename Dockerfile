# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies including gcc for CGO (required for go-sqlite3)
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for sqlite3 support
RUN CGO_ENABLED=1 GOOS=linux go build -mod=mod -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections and libc for CGO binaries
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directory for DuckDB/SQLite
RUN mkdir -p /app/data

# Copy binary from builder
COPY --from=builder /app/main .

# Copy admin templates (required for admin UI)
COPY --from=builder /app/admin/templates ./admin/templates

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]

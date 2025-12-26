# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (CGO disabled - no sqlite3)
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directory for local file storage
RUN mkdir -p /app/data /app/data/stocks

# Copy binary from builder
COPY --from=builder /app/main .

# Copy admin templates (required for admin UI)
COPY --from=builder /app/admin/templates ./admin/templates

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]

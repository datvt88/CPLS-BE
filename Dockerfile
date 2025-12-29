# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first (for better caching)
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=mod -ldflags="-s -w" -o main .

# Verify binary was created
RUN ls -la main

# Final stage
FROM alpine:3.19

# Install ca-certificates and timezone data
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directories
RUN mkdir -p /app/data /app/data/stocks

# Copy binary from builder
COPY --from=builder /app/main .

# Copy admin templates (required for admin UI)
COPY --from=builder /app/admin/templates ./admin/templates

# Make binary executable
RUN chmod +x /app/main

# Expose port
EXPOSE 8080

# Run the application
CMD ["/app/main"]

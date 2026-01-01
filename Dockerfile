# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build binary with optimizations
# CGO_ENABLED=0 for static binary (no external C dependencies)
# -ldflags="-s -w" strips debug info for smaller binary
# -trimpath removes file system paths from binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=1.0.0" \
    -trimpath \
    -o main .

# Final stage - minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user for security
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

# Create data directory with proper permissions
RUN mkdir -p /app/data/stocks && chown -R appuser:appgroup /app

# Copy binary from builder
COPY --from=builder /app/main .

# Switch to non-root user
USER appuser

# Set timezone (Asia/Ho_Chi_Minh for Vietnam)
ENV TZ=Asia/Ho_Chi_Minh

# Expose port (Cloud Run uses PORT env variable)
EXPOSE 8080

# Health check for container orchestration
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

# Run the binary
CMD ["/app/main"]

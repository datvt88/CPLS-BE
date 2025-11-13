FROM golang:1.23-alpine

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Set GOTOOLCHAIN to avoid version conflicts
ENV GOTOOLCHAIN=local

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# Use -mod=mod to allow go to update go.mod if needed
RUN go build -mod=mod -o main .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]

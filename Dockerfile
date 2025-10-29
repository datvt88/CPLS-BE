# Use official Golang image
FROM golang:1.20

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod ./
COPY go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application
COPY . .

# Build the Go app
RUN go build -o main .

# Expose port
EXPOSE 8080

# Run the executable
CMD ["./main"]

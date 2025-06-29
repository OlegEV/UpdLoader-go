# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git (needed for some Go modules)
RUN apk add --no-cache git

# Copy go mod and sum files first for better caching
COPY go.mod ./
COPY go.su[m] ./

# Download dependencies (this layer will be cached if go.mod doesn't change)
RUN go mod download

# Copy source code
COPY . .

# Ensure all dependencies are properly resolved and build the application
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Set timezone to Moscow
ENV TZ=Europe/Moscow

# Create app directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create temp directory for file processing
RUN mkdir -p ./temp

# Expose port (not strictly necessary for this bot, but good practice)
# EXPOSE 8080

# Run the application
CMD ["./main"]
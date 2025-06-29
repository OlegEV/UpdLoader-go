# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git (needed for some Go modules)
RUN apk add --no-cache git

# Copy source code
COPY . .

# Download dependencies and build the application
RUN go mod download && go mod tidy && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/main.go

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
EXPOSE 8080

# Run the application
CMD ["./main"]
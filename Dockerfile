# Build stage
FROM golang:1.26-alpine AS builder

# Set working directory inside the container
WORKDIR /app

# Copy go mod and sum files first to leverage Docker cache layer
COPY go.mod go.sum* ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application as a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Final stage (Small footprint)
FROM alpine:latest

WORKDIR /app

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/main .

# Expose port (Backend server runs on 8080)
EXPOSE 8080

# Command to run the executable
CMD ["./main"]

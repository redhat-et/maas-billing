# Build stage
FROM golang:1.24.2-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o key-manager .

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install CA certificates for HTTPS requests
RUN microdnf update -y && microdnf install -y ca-certificates && microdnf clean all

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/key-manager .

# Make binary executable and fix permissions for OpenShift
RUN chmod +x key-manager && \
    chgrp -R 0 /app && \
    chmod -R g=u /app

# Use a non-root user (OpenShift will assign random UID)
USER 1001

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["./key-manager"]

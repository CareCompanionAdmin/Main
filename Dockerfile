# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o carecompanion \
    ./cmd/server

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
# - aws-cli: For managing security group rules in dev mode
# - putty: For converting PEM to PPK format
# - curl: For fetching EC2 instance metadata
RUN apk add --no-cache ca-certificates tzdata aws-cli putty curl

# Create non-root user
RUN addgroup -g 1001 -S carecompanion && \
    adduser -u 1001 -S carecompanion -G carecompanion

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/carecompanion .

# Copy templates and static files
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Copy migrations (if running migrations from container)
COPY --from=builder /app/migrations ./migrations

# Create uploads directory
RUN mkdir -p /app/uploads && chown -R carecompanion:carecompanion /app

# Switch to non-root user
USER carecompanion

# Expose port
EXPOSE 8090

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/health || exit 1

# Run the application
CMD ["./carecompanion"]

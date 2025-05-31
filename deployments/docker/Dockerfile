# Multi-stage build for NBAR QoS Classifier

# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -a -installsuffix cgo \
    -o nbar-classifier \
    ./cmd/nbar-classifier

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S nbar && \
    adduser -u 1001 -S nbar -G nbar

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/nbar-classifier .

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Create directories for data and logs
RUN mkdir -p /app/data /app/logs && \
    chown -R nbar:nbar /app

# Switch to non-root user
USER nbar

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default command
ENTRYPOINT ["./nbar-classifier"]
CMD ["--config=./configs/config.yaml"]

# Multi-stage build for nmcd
# Supports amd64 and arm64 architectures

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version and platform
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

# Build the binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -v -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /build/nmcd \
    ./cmd/nmcd

# Runtime stage
FROM --platform=$TARGETPLATFORM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S nmcd && adduser -S nmcd -G nmcd

# Create data directory
RUN mkdir -p /data && chown nmcd:nmcd /data

# Copy binary from builder
COPY --from=builder /build/nmcd /usr/local/bin/nmcd

# Set ownership
RUN chown nmcd:nmcd /usr/local/bin/nmcd

# Switch to non-root user
USER nmcd

# Set working directory
WORKDIR /data

# Expose ports
# 8334: P2P (mainnet)
# 8336: RPC (mainnet)
# 18334: P2P (testnet)
# 18336: RPC (testnet)
# 18444: P2P (regtest)
# 18445: RPC (regtest)
# 9090: Prometheus metrics
EXPOSE 8334 8336 18334 18336 18444 18445 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8336/health || exit 1

# Volume for data persistence
VOLUME ["/data"]

# Default command
ENTRYPOINT ["/usr/local/bin/nmcd"]
CMD ["-datadir=/data", "-rpcaddr=0.0.0.0:8336"]

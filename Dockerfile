# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git for version info
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o mediawiki-mcp-server .

# Runtime stage
FROM alpine:3.20

# MCP Registry label for OCI validation
LABEL io.modelcontextprotocol.server.name="io.github.olgasafonova/mediawiki-mcp-server"

# Install ca-certificates for HTTPS and poppler-utils for PDF support
RUN apk add --no-cache ca-certificates poppler-utils

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mediawiki-mcp-server .

# Create non-root user
RUN adduser -D -u 1000 mcp
USER mcp

# Default to HTTP mode on port 8080
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["./mediawiki-mcp-server"]
CMD ["-http", ":8080"]

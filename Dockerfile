# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git for go mod (if needed for private repos)
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o nordic-registry-mcp-server .

# Runtime stage
FROM alpine:3.19

# Add ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/nordic-registry-mcp-server .

# MCP servers communicate via stdio
# Run as non-root user for security
RUN adduser -D -g '' mcp
USER mcp

ENTRYPOINT ["./nordic-registry-mcp-server"]

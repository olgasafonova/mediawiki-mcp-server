# Deployment

How to run the MediaWiki MCP server in HTTP mode for remote access (ChatGPT, n8n, hosted environments).

For stdio mode (Claude Desktop, Cursor, VS Code), see [SETUP.md](SETUP.md).

## HTTP Transport Options

For ChatGPT, n8n, and remote access, the server supports HTTP transport.

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-http` | (empty) | HTTP address (e.g., `:8080`). Empty = stdio mode |
| `-token` | (empty) | Bearer token for authentication |
| `-origins` | (empty) | Allowed CORS origins (comma-separated) |
| `-rate-limit` | 60 | Max requests per minute per IP |

### Examples

```bash
# Basic HTTP server
./mediawiki-mcp-server -http :8080

# With authentication
./mediawiki-mcp-server -http :8080 -token "your-secret-token"

# Restrict to specific origins
./mediawiki-mcp-server -http :8080 -token "secret" \
  -origins "https://chat.openai.com,https://n8n.example.com"

# Bind to localhost only (for use behind reverse proxy)
./mediawiki-mcp-server -http 127.0.0.1:8080 -token "secret"
```

## Security Best Practices

When exposing the server over HTTP:

### 1. Always Use Authentication

```bash
./mediawiki-mcp-server -http :8080 -token "$(openssl rand -hex 32)"
```

### 2. Use HTTPS via Reverse Proxy

Example nginx configuration:

```nginx
server {
    listen 443 ssl;
    server_name mcp.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 3. Restrict Origins

```bash
./mediawiki-mcp-server -http :8080 -token "secret" \
  -origins "https://chat.openai.com"
```

### Built-in Security Features

| Feature | Description |
|---------|-------------|
| Bearer Auth | Validates `Authorization: Bearer <token>` header |
| Origin Validation | Blocks requests from unauthorized domains |
| Rate Limiting | 60 requests/minute per IP (configurable) |
| Security Headers | X-Content-Type-Options, X-Frame-Options |
| Circuit Breaker | Automatic failover after consecutive API failures |

## HTTP Endpoints

When running in HTTP mode, these endpoints are available:

| Endpoint | Description |
|----------|-------------|
| `/` | MCP protocol endpoint (tools, resources, prompts) |
| `/health` | Liveness check (always returns 200 if server is running) |
| `/ready` | Readiness check (verifies wiki API connectivity) |
| `/tools` | Tool discovery (lists all 40+ tools by category) |
| `/status` | Resilience status (circuit breaker state, dedup stats) |
| `/metrics` | Prometheus metrics (request counts, latencies) |
| `/.well-known/mcp-server-card` | [SEP-2127](https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2127) Server Card (pre-connect discovery) via [mcp-servercard-go](https://github.com/olgasafonova/mcp-servercard-go) |

### Health Checks

Use `/health` and `/ready` for container orchestration:

```yaml
# Kubernetes example
livenessProbe:
  httpGet:
    path: /health
    port: 8080
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
```

### Tool Discovery

Get a list of all available tools:

```bash
curl http://localhost:8080/tools | jq '.categories | keys'
```

### Resilience Monitoring

Check circuit breaker and request deduplication status:

```bash
curl http://localhost:8080/status
# Returns: {"circuit_breaker":{"state":"closed",...},"inflight_requests":0}
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `MEDIAWIKI_URL` | Yes | Wiki API endpoint (e.g., `https://wiki.com/api.php`) |
| `MEDIAWIKI_USERNAME` | No | Bot username (`User@BotName`) |
| `MEDIAWIKI_PASSWORD` | No | Bot password |
| `MEDIAWIKI_TIMEOUT` | No | Request timeout (default: `30s`) |
| `MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS` | No | Comma-separated host allowlist for `mediawiki_upload_file`'s `file_url` path (supports `*.` subdomain wildcards). Fail-closed: unset = no URL uploads allowed. Does not affect base64 `file_data` uploads. |
| `MEDIAWIKI_MAX_UPLOAD_DATA_BYTES` | No | Max decoded size of a base64 `file_data` upload, in bytes (default: `104857600`, i.e. 100 MiB, matching MediaWiki's default `$wgMaxUploadSize`). |
| `MCP_AUTH_TOKEN` | No | Bearer token for HTTP authentication |
| `WIKI_NO_SESSION_CACHE` | No | Set to any non-empty value to disable the `wiki` CLI session cache |
| `XDG_CONFIG_HOME` | No | Overrides session cache location (default: `~/.config/wiki/sessions.json`) |

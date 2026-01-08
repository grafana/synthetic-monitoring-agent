# gRPC Connection Management in Synthetic Monitoring Agent

This document provides a comprehensive overview of how gRPC connections are handled in the
synthetic-monitoring-agent, including architecture, configuration options, and best practices
implementation.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Connection Lifecycle](#connection-lifecycle)
3. [Configuration Options](#configuration-options)
4. [Error Handling](#error-handling)
5. [Observability](#observability)
6. [Security](#security)
7. [Best Practices Implementation](#best-practices-implementation)
8. [Troubleshooting](#troubleshooting)
9. [References](#references)

---

## Architecture Overview

### Components

The agent uses **two independent gRPC connections** to the synthetic-monitoring-api server:

1. **Checks Updater** (`internal/checks/checks.go`)
   - Maintains continuous check configuration
   - Receives ADD/UPDATE/DELETE operations for checks
   - Long-lived bidirectional streaming RPC: `GetChanges()`
   - Manages lifecycle of scrapers (one per check)

2. **Adhoc Handler** (`internal/adhoc/adhoc.go`)
   - Handles on-demand check execution
   - Processes instant test requests from UI
   - Long-lived server streaming RPC: `GetAdHocChecks()`
   - Creates ephemeral runners for single-shot execution

### Connection Setup

**Location:** `cmd/synthetic-monitoring-agent/grpc.go`

```go
func dialAPIServer(
    addr string,
    allowInsecure bool,
    apiToken string,
    logger zerolog.Logger,
    grpcClientMetrics *grpcprom.ClientMetrics,
) (*grpc.ClientConn, error)
```

- Uses `grpc.NewClient()` for lazy connection establishment
- Single HTTP/2 connection with multiplexed RPCs
- No connection pooling (gRPC handles this internally)
- Shared connection used by both Updater and Adhoc handler

### Interceptor Chain

The agent uses chained interceptors for cross-cutting concerns:

```go
grpc.WithChainUnaryInterceptor(
    logging.UnaryClientInterceptor(interceptorLogger(logger), logOpts...),
    grpcClientMetrics.UnaryClientInterceptor(),
),
grpc.WithChainStreamInterceptor(
    logging.StreamClientInterceptor(interceptorLogger(logger), logOpts...),
    grpcClientMetrics.StreamClientInterceptor(),
),
```

**Order matters:** Logging → Metrics ensures logs appear before metrics collection.

---

## Connection Lifecycle

### 1. Initial Connection

```
Agent Start
    ↓
dialAPIServer() called (lazy connection)
    ↓
Connection established on first RPC
    ↓
RegisterProbe() RPC with WaitForReady(true)
    ↓
Connection ready for streaming RPCs
```

**RegisterProbe timing:**
- Default: Waits indefinitely (respects context cancellation)
- With `-grpc-connect-timeout=30s`: Fails after 30 seconds
- Returns `codes.DeadlineExceeded` if timeout exceeded

### 2. Steady State

Both handlers run independent loops:

**Checks Updater:**
```go
GetChanges(ctx, &knownChecks, grpc.WaitForReady(true))
// Receives: CheckOperation{Type: ADD/UPDATE/DELETE, Check: {...}}
// Manages scraper lifecycle based on operations
```

**Adhoc Handler:**
```go
GetAdHocChecks(ctx, &sm.Void{}, grpc.WaitForReady(true))
// Receives: AdHocRequest{Check: {...}, Timeout: ...}
// Executes check and publishes results
```

### 3. Connection Monitoring

**Keepalive pings:** Sent every 90 seconds (configurable via `synthetic_monitoring.HealthCheckInterval`)

```go
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                90 * time.Second,  // Ping interval
    Timeout:             30 * time.Second,  // Ping ack timeout
    PermitWithoutStream: true,              // Allow pings on idle streams
})
```

**Purpose:**
- Detects dead connections faster than TCP timeout
- Critical for long-lived idle streams (GetChanges may be quiet between updates)
- Triggers reconnection if no ping acknowledgment within 30s

### 4. Error Handling and Reconnection

**Error classification:**
```go
switch st.Code() {
case codes.Canceled:
    // Context cancelled - graceful shutdown
    return context.Canceled

case codes.Unavailable, codes.DeadlineExceeded:
    // Transient errors - trigger retry with backoff
    return TransientError(...)

case codes.PermissionDenied, codes.Unauthenticated:
    // Auth failures - fatal, don't retry
    return errNotAuthorized

case codes.Unimplemented:
    // API version mismatch - fatal
    return errIncompatibleApi

default:
    // Unknown error - preserve details
    return st.Err()
}
```

**Backoff strategy:**
- Exponential backoff for transient errors
- Resets backoff after successful connection
- See `newConnectionBackoff()` in `cmd/synthetic-monitoring-agent/main.go`

### 5. Graceful Shutdown

**Signal handling:**
- `SIGTERM`: Immediate graceful shutdown
- `SIGUSR1`: Disconnect from API, keep scrapers running for 1 minute
  - Allows another agent to take over
  - Reconnects after 1 minute if no takeover occurs
  - Supports zero-downtime upgrades

---

## Configuration Options

### Command-Line Flags

```bash
# Required
-api-server-address string
    gRPC API server address (default "localhost:4031")
    Example: synthetic-monitoring-grpc.grafana.net:443

-api-token string
    Synthetic monitoring probe authentication token

# Optional
-api-insecure
    Don't use TLS with connections to gRPC API
    Use only for local development/testing

-grpc-connect-timeout duration
    Timeout for initial gRPC connection (0 = no timeout)
    Default: 0 (wait indefinitely)
    Example: -grpc-connect-timeout=30s
```

### Environment Variables

```bash
# Alternative to -api-token flag
export SM_AGENT_API_TOKEN="your-token-here"
```

### Configuration Examples

**Production deployment:**
```bash
./synthetic-monitoring-agent \
  -api-server-address=synthetic-monitoring-grpc.grafana.net:443 \
  -api-token="${SM_AGENT_API_TOKEN}" \
  -grpc-connect-timeout=30s
```

**Local development:**
```bash
./synthetic-monitoring-agent \
  -api-server-address=localhost:4031 \
  -api-token="${API_TOKEN}" \
  -api-insecure \
  -debug
```

**Kubernetes deployment:**
```yaml
env:
  - name: SM_AGENT_API_TOKEN
    valueFrom:
      secretKeyRef:
        name: sm-agent-secret
        key: api-token
args:
  - "-api-server-address=synthetic-monitoring-grpc.grafana.net:443"
  - "-grpc-connect-timeout=30s"
```

---

## Error Handling

### Error Types

**1. Transient Errors (Retriable)**
- `codes.Unavailable`: Network errors, connection resets, transport closing
- `codes.DeadlineExceeded`: Timeout exceeded (including connect timeout)
- **Action:** Exponential backoff, then retry

**2. Fatal Errors (Non-Retriable)**
- `codes.PermissionDenied`: Invalid/expired API token
- `codes.Unauthenticated`: Authentication failure
- `codes.Unimplemented`: API version mismatch
- **Action:** Log error, exit with failure

**3. Context Errors**
- `codes.Canceled`: Context cancelled by application
- **Action:** Clean shutdown, no retry

### Error Handler Implementation

**Location:** `internal/checks/checks.go` (in the `grpcErrorHandler` function within `loop`)

```go
grpcErrorHandler := func(action string, err error) error {
    st, ok := status.FromError(err)
    if !ok {
        c.logger.Error().Err(err).Str("action", action).Msg("non-grpc error")
        return fmt.Errorf("%s: %w", action, err)
    }

    c.logger.Error().
        Err(err).
        Str("action", action).
        Uint32("code", uint32(st.Code())).
        Msg(st.Message())

    switch st.Code() {
    case codes.Canceled:
        return context.Canceled

    case codes.Unavailable, codes.DeadlineExceeded:
        return TransientError(fmt.Sprintf("%s: %s", action, st.Message()))

    case codes.PermissionDenied, codes.Unauthenticated:
        return errNotAuthorized

    case codes.Unimplemented:
        return errIncompatibleApi

    default:
        return st.Err()
    }
}
```

**Best practice:** Uses status codes instead of string matching (error messages are not part of stable API).

---

## Observability

### Structured Logging

**Implementation:** `cmd/synthetic-monitoring-agent/grpc.go` (see `interceptorLogger` function)

The agent uses a zerolog adapter for gRPC logging:

```go
func interceptorLogger(l zerolog.Logger) logging.Logger {
    return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
        l := l.With().Fields(fields).Logger()

        switch lvl {
        case logging.LevelDebug:
            l.Debug().Msg(msg)
        case logging.LevelInfo:
            l.Info().Msg(msg)
        case logging.LevelWarn:
            l.Warn().Msg(msg)
        case logging.LevelError:
            l.Error().Msg(msg)
        default:
            l.Info().Msg(msg)
        }
    })
}
```

**Logs generated:**
```json
{
  "level": "info",
  "subsystem": "grpc",
  "grpc.method": "/synthetic_monitoring.Checks/RegisterProbe",
  "grpc.start_time": "2025-11-28T00:00:00Z",
  "message": "started call"
}
{
  "level": "info",
  "subsystem": "grpc",
  "grpc.method": "/synthetic_monitoring.Checks/RegisterProbe",
  "grpc.code": "OK",
  "grpc.duration": "0.123s",
  "message": "finished call"
}
```

### Prometheus Metrics

**Implementation:** `cmd/synthetic-monitoring-agent/grpc.go` + `metrics.go`

**Metrics exported:**

| Metric | Type | Description |
|--------|------|-------------|
| `grpc_client_started_total` | Counter | Total RPCs started |
| `grpc_client_handled_total` | Counter | Total RPCs completed (by status code) |
| `grpc_client_msg_received_total` | Counter | Total messages received |
| `grpc_client_msg_sent_total` | Counter | Total messages sent |
| `grpc_client_handling_seconds` | Histogram | RPC latency distribution |

**Labels:**
- `grpc_method`: RPC method name (e.g., `/synthetic_monitoring.Checks/RegisterProbe`)
- `grpc_service`: Service name (e.g., `synthetic_monitoring.Checks`)
- `grpc_type`: `unary` or `streaming`
- `grpc_code`: gRPC status code (e.g., `OK`, `Unavailable`)

**Example queries:**
```promql
# RPC success rate
rate(grpc_client_handled_total{grpc_code="OK"}[5m])
/ rate(grpc_client_handled_total[5m])

# P95 latency by method
histogram_quantile(0.95,
  rate(grpc_client_handling_seconds_bucket[5m])
)

# Error rate by code
rate(grpc_client_handled_total{grpc_code!="OK"}[5m])
```

### TLS Connection Logging

**Location:** `internal/checks/checks.go` + `internal/adhoc/adhoc.go`

After successful `RegisterProbe`, the agent logs TLS connection details:

```go
func logConnectionSecurity(ctx context.Context, logger zerolog.Logger) {
    p, ok := peer.FromContext(ctx)
    if !ok {
        logger.Debug().Msg("no peer information available")
        return
    }

    tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
    if !ok {
        logger.Info().Str("security", "insecure").Msg("connection established without TLS")
        return
    }

    // TLS version is converted inline using a switch statement
    logger.Info().
        Str("tls_version", tlsVersion).  // e.g., "TLS 1.3"
        Str("cipher_suite", tls.CipherSuiteName(tlsInfo.State.CipherSuite)).
        Str("server_name", tlsInfo.State.ServerName).
        Bool("handshake_complete", tlsInfo.State.HandshakeComplete).
        Msg("secure connection established")
}
```

**Example log output:**
```json
{
  "level": "info",
  "tls_version": "TLS 1.3",
  "cipher_suite": "TLS_AES_128_GCM_SHA256",
  "server_name": "synthetic-monitoring-grpc.grafana.net",
  "handshake_complete": true,
  "message": "secure connection established"
}
```

### Connection State Logging

**Important:** Connection state is logged for **observability only**, not for decision-making.

From gRPC anti-patterns documentation:
> "connections with a ClientConn are dynamic — they may come and go over time"

**Implementation:**
```go
// Connection state is logged for debugging/observability only.
// Do not make decisions based on connection state - it provides no guarantees.
// Trust RPC-level error handling instead, as connections are dynamic.
logger := c.logger.With().Str("connection_state", c.api.conn.GetState().String()).Logger()
```

**Why this matters:**
- Connection state can change immediately after checking
- Provides no guarantee about RPC success
- RPCs should be attempted regardless of state
- Use error codes from failed RPCs for decision-making

---

## Security

### TLS Configuration

**Location:** `cmd/synthetic-monitoring-agent/grpc.go` (see `dialAPIServer` function)

```go
transportCreds := insecure.NewCredentials()
if !allowInsecure {
    transportCreds = credentials.NewTLS(&tls.Config{
        ServerName: grpcApiHost(addr),
        MinVersion: tls.VersionTLS12, // Enforce TLS 1.2 minimum for security
        // TLS 1.2 is widely supported and secure. TLS 1.0 and 1.1 are deprecated.
        // TLS 1.3 is preferred but TLS 1.2 ensures broader compatibility.
    })
}
```

**Security properties:**
- **Minimum TLS 1.2:** Prevents negotiation of deprecated TLS 1.0/1.1
- **Server name verification:** Validates certificate matches expected hostname
- **TLS 1.3 support:** Automatically negotiated if server supports it
- **System trust store:** Uses OS certificate store for validation

### Authentication

**Method:** Bearer token via per-RPC credentials

**Implementation:** `cmd/synthetic-monitoring-agent/grpc.go` (see `creds` type)

```go
type creds struct {
    Token         string
    AllowInsecure bool
}

func (c creds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
    return map[string]string{
        "authorization": "Bearer " + c.Token,
    }, nil
}

func (c creds) RequireTransportSecurity() bool {
    // Only require transport security when insecure mode is NOT enabled.
    return !c.AllowInsecure
}
```

**Properties:**
- Token automatically attached to every RPC
- No manual header manipulation needed
- Respects insecure mode for local development

### Token Storage

**Best practices:**
- Store tokens in secrets management system (Kubernetes secrets, Vault, etc.)
- Pass via environment variable (`SM_AGENT_API_TOKEN`)
- Never commit tokens to version control
- Rotate tokens regularly

---

## Best Practices Implementation

### ✅ Modern Client API

**Status:** Implemented

Uses `grpc.NewClient()` instead of deprecated `grpc.Dial()`:
- Lazy connection establishment (connects on first RPC)
- No blocking during initialization
- Follows current gRPC-Go best practices

**Location:** `cmd/synthetic-monitoring-agent/grpc.go` (see `dialAPIServer` function)

### ✅ Error Handling with Status Codes

**Status:** Implemented

Uses `status.FromError()` and status codes instead of string matching:
- Robust against gRPC library updates
- Follows documented error handling patterns
- Properly distinguishes transient vs. fatal errors

**Location:** `internal/checks/checks.go:393-430`, `internal/adhoc/adhoc.go`

### ✅ WaitForReady on Critical RPCs

**Status:** Implemented

```go
// Critical startup RPCs use WaitForReady(true)
client.RegisterProbe(ctx, probeInfo, grpc.WaitForReady(true))
client.GetChanges(ctx, knownChecks, grpc.WaitForReady(true))

// Health check pings use WaitForReady(false) for fail-fast
client.Ping(ctx, void, grpc.WaitForReady(false))
```

**Rationale:**
- Startup: Queue RPC until connection ready (respects timeout)
- Health checks: Fail immediately to detect issues quickly

### ✅ Keepalive Configuration

**Status:** Implemented

```go
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                90 * time.Second,  // Send ping every 90s
    Timeout:             30 * time.Second,  // Wait 30s for ack
    PermitWithoutStream: true,              // Allow pings on idle streams
})
```

**Why these values:**
- 90s balances network overhead with timely failure detection
- 30s timeout catches unresponsive connections
- `PermitWithoutStream=true` critical for idle long-lived streams

### ✅ Configurable Connection Timeout

**Status:** Implemented

**Flag:** `-grpc-connect-timeout duration`

**Implementation:**
- Default: `0` (no timeout, wait indefinitely)
- When set: Applies to initial `RegisterProbe` RPC only
- Uses `context.WithTimeout()` for timeout enforcement
- Falls back to existing retry logic on timeout

**Example:**
```bash
./synthetic-monitoring-agent \
  -api-server-address=api.grafana.net:443 \
  -api-token=$TOKEN \
  -grpc-connect-timeout=30s
```

**Use cases:**
- Faster failure detection during deployment
- Avoid hanging during DNS resolution issues
- Configurable per environment (aggressive in prod, relaxed in dev)

### ✅ Observability

**Status:** Implemented

- **Structured logging:** All gRPC calls logged via zerolog
- **Prometheus metrics:** Success rates, latencies, error codes
- **TLS logging:** Connection security details logged on connect
- **Connection state logging:** For debugging (not decision-making)

### ⚠️ No Built-in Retry Policy

**Status:** Intentionally not implemented

**Decision:** Keep manual retry logic instead of gRPC's built-in retry

**Rationale:**
- Better observability into retry behavior
- More control over connection-level vs. RPC-level retry
- Existing implementation is well-tested and understood
- Built-in retry adds complexity without solving current problems

**Alternative:** The current backoff strategy provides equivalent functionality with better visibility.

---

## Troubleshooting

### Connection Issues

**Symptom:** Agent fails to connect to API server

**Check:**
1. Network connectivity: `curl -v https://synthetic-monitoring-grpc.grafana.net`
2. DNS resolution: `nslookup synthetic-monitoring-grpc.grafana.net`
3. Firewall rules: Ensure outbound port 443 is open
4. TLS version: Server must support TLS 1.2 or higher

**Logs to examine:**
```json
{"level":"error","action":"registering probe","code":14,"message":"Unavailable"}
// Code 14 = Unavailable (network issue)

{"level":"error","action":"registering probe","code":7,"message":"PermissionDenied"}
// Code 7 = PermissionDenied (auth issue)
```

### Authentication Failures

**Symptom:** `codes.PermissionDenied` or `codes.Unauthenticated` errors

**Check:**
1. API token validity: Token may be expired or revoked
2. Token format: Must be valid Bearer token
3. Permissions: Token must have probe registration permissions

**Resolution:**
- Regenerate token in Synthetic Monitoring UI
- Update token in secret store
- Restart agent with new token

### Timeout Issues

**Symptom:** `codes.DeadlineExceeded` errors

**Possible causes:**
1. Network latency too high for timeout value
2. DNS resolution taking too long
3. Server overloaded or unresponsive

**Resolution:**
- Increase `-grpc-connect-timeout` value
- Check network latency: `ping synthetic-monitoring-grpc.grafana.net`
- Verify server status in monitoring dashboards

### Keepalive Issues

**Symptom:** Frequent disconnections despite stable network

**Check:**
1. Firewall/NAT timeouts: May be dropping idle connections
2. Load balancer settings: May have shorter idle timeout than keepalive
3. Server-side keepalive enforcement

**Resolution:**
- Adjust keepalive parameters (requires code change)
- Configure firewall/NAT to allow longer idle connections
- Contact server operators about keepalive compatibility

### TLS Issues

**Symptom:** TLS handshake failures

**Check:**
1. TLS version support: Server must support TLS 1.2+
2. Certificate validity: Check expiration and chain
3. System trust store: Must include CA certificates

**Logs to examine:**
```json
{"level":"error","message":"transport: authentication handshake failed"}
```

**Resolution:**
- Update system CA certificates: `update-ca-certificates`
- Verify server certificate: `openssl s_client -connect host:443`
- Check TLS version: Should see "TLS 1.2" or "TLS 1.3" in logs

---

## References

### Official Documentation

- [gRPC-Go Anti-patterns](https://github.com/grpc/grpc-go/blob/master/Documentation/anti-patterns.md)
- [gRPC-Go Keepalive](https://github.com/grpc/grpc-go/blob/master/Documentation/keepalive.md)
- [gRPC-Go Error Handling](https://github.com/grpc/grpc-go/blob/master/Documentation/rpc-errors.md)
- [gRPC-Go Metadata](https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md)
- [gRPC Core Concepts](https://grpc.io/docs/what-is-grpc/core-concepts/)

### Libraries Used

- **gRPC:** `google.golang.org/grpc`
- **Metrics:** `github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus`
- **Logging:** `github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging`
- **Logger:** `github.com/rs/zerolog`

### Related Documentation

- `DEVELOPMENT.md`: Developer's guide to the codebase
- `README.md`: General agent documentation
- `internal/checks/checks.go`: Checks updater implementation
- `internal/adhoc/adhoc.go`: Adhoc handler implementation
- `cmd/synthetic-monitoring-agent/grpc.go`: Connection setup

---

*Document created: 2025-11-27*
*Last updated: 2026-01-08*

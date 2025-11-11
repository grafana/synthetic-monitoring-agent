# Developer's guide to the Synthetic Monitoring agent

This file provides some guidance as to how to work with the codebase.

## Project Overview

This is the Grafana Synthetic Monitoring agent: a distributed probe that
executes network checks (HTTP, DNS, TCP, ICMP, gRPC, Traceroute, k6-based
scripted/browser checks) from various locations and pushes metrics/logs to
Grafana Cloud (Prometheus/Loki).

## Common Commands

The build system is organized around make. Make is ubiquitous and the chances
are that if you have `go` installed, you also have `make` installed. If not,
it's straightforward to install (possibly using your distribution's preferred
package manager), with very little dependencies.

You can get up-to-date documentation by running `make help`, but some common
targets are mentioned below.

### Building
```bash
make build              # Build all Go binaries for all platforms
make build-native       # Build only for current OS/arch
make deps               # Download and verify Go dependencies
```

### Testing
```bash
make test               # Run all tests with coverage and race detection
make test-fast          # Run only fast tests (with -short flag)

# Run specific package tests
make test-go GO_TEST_ARGS=./internal/prober/...

# Run single test
make test-go GO_TEST_ARGS="-v -run TestSpecificTest ./internal/package/..."
```

Note that tests are run inside a docker container, to ensure some level of
reproducibility and isolation from the local environment. If you want, you can
use `go test` directly, but keep in mind that this might end up using a
different version of the Go compiler, and it might be affected by other things
in your local environment.

### Linting
```bash
make lint               # Run all linters (golangci-lint + go vet + shellcheck)
make lint-go            # Run only Go linters
```

Again, the linting tools are executed in a docker container, to ensure that
versions match. You can use `golangci-lint` directly, but keep in mind that the
configuration file targets v2.

### Code generation

Some of the code and support files are generated, specifically:

- internal/scraper/testdata/*.txt
- pkg/accounting/data.go
- pkg/pb/synthetic_monitoring/checks.pb.go

In order to update `internal/scraper/testdata/*.txt` you can use the
`testdata` target in the Makefile.

`pkg/accounting/data.go` and `pkg/pb/synthetic_monitoring/checks.pb.go`
are updated by the `generate` target in the Makefile.

Code generation is *not* run automatically, but there are some tests
that try to detect discrepancies.

### Development
```bash
# Run the agent locally (requires API token)
./dist/synthetic-monitoring-agent \
  -api-server-address=synthetic-monitoring-grpc.grafana.net:443 \
  -api-token=<your-token> \
  -verbose

# Build Docker images
docker build --target release .              # Without Chromium (smaller)
docker build --target with-browser .         # With Chromium (for browser checks)
```

### Livereload development

Install [air](https://github.com/air-verse/air?tab=readme-ov-file#installation)
and ensure `SM_AGENT_API_TOKEN` is set prior to running.

```bash
air
```

Any code changes will now trigger air to rebuild the agent and run the
generated executable.

## Architecture Overview

### Core Flow
```
API Server (gRPC) → Updater → Scrapers → Probers → Publisher → Prometheus/Loki
                         ↓
                    Adhoc Handler (on-demand checks)
```

### Key Components

**Updater** (`internal/checks/checks.go`)
- Maintains long-lived gRPC stream to API server via `GetChanges()`
- Receives check ADD/UPDATE/DELETE operations
- Manages life cycle of all scrapers (one per check)
- Handles reconnection with back off on failures
- Responds to SIGUSR1 for graceful disconnect (enables zero-downtime upgrades)

**Scraper** (`internal/scraper/scraper.go`)
- One instance per active check, runs on scheduled interval
- Executes probe via prober interface
- Adds user-defined labels and check metadata to all telemetry
- Implements "republishing" - resends metrics with updated timestamps when check interval > 2min
- Sends stale markers (NaN) on shutdown for proper Prometheus metric life cycle
- Tracks check state machine (passing/failing thresholds)

**Prober** (`internal/prober/prober.go`)
- Factory pattern for check-type-specific implementations
- Types: HTTP, DNS, TCP, ICMP, gRPC, Traceroute, Scripted (k6), Browser, MultiHTTP
- Interface: `Probe(ctx, target, registry, logger) (success bool, duration time.Duration)`
- Each prober registers metrics into Prometheus registry

**k6Runner** (`internal/k6runner/`)
- Handles scripted, browser, and multihttp checks via k6
- Two modes: Local (executes k6 binary) or HTTP (remote k6 runner service)
- HTTP runner includes retry logic with back off for transient failures
- Grace time: adds 20s to timeout for runner communication overhead
- Processor parses k6 output and registers metrics in Prometheus format

**Publisher** (`internal/pusher/v2/publisher.go`)
- V2 is current (V1 exists for compatibility)
- Per-tenant push handlers with automatic life cycle management
- Batches and sends remote write to Prometheus/Loki
- Publishers created via registry/factory pattern

**Adhoc Handler** (`internal/adhoc/adhoc.go`)
- Separate gRPC stream for on-demand checks (instant testing from UI)
- Creates ephemeral runners for single-shot execution
- Results published as logs with special labels
- No retry or scheduling logic

**Tenant Manager** (`internal/tenants/manager.go`)
- Caches tenant metadata with TTL (15min default with jitter)
- Provides tenant info to publisher for authentication
- Thread-safe with per-tenant locks

### Check Types

**Simple probers**: DNS, HTTP, TCP, ICMP, gRPC, Traceroute
- Direct implementation using
  [blackbox_exporter](https://github.com/prometheus/blackbox_exporter) as backend
- ICMP and Traceroute implement the same interface, but are custom
  implementations
- Return metrics via Prometheus registry

**k6-Based**: Scripted, Browser, MultiHTTP
- Convert check settings to k6 script
- Execute via k6runner (local process or remote HTTP service)
- MultiHTTP generates k6 script from assertion-based configuration
- Parse k6's Prometheus output format provided by a custom extension

### Data Flow

![agent process][process]

**Metrics Path:**
```
Prober → Prometheus Registry → Scraper (extract + label) → Publisher → Remote Write → Prometheus
```

**Logs Path:**
```
Prober (logfmt logger) → Scraper (parse + label) → Publisher → Loki Push API
```

**Special Metric:**
- `sm_check_info`: Contains check metadata (geohash, frequency, alert sensitivity)
- Used for joining check data in queries

## Important Patterns

### Error Handling
- Custom error types: `FatalError` (stop scraper), `TransientError` (retry with back off)
- Use `pkg/errors.Wrap()` to add context
- k6 runner retries on retriable errors (timeout, connection refused, etc.)

### Metrics Philosophy
- **Derived metrics**: Creates summaries/histograms from gauge metrics for flexibility
- **Stale markers**: Sends NaN values on scraper shutdown so Prometheus knows metric is gone
- **Republishing**: Resends metrics with updated timestamps to handle long intervals (keeps data fresh)
- **Check info metric**: Special `sm_check_info` with labels for metadata joins

### Concurrency
- Heavy use of `errgroup.Group` for coordinated goroutine management
- Per-tenant locks in publisher to avoid contention
- Each scraper has independent context for life cycle control
- Context-aware sleep functions for clean shutdown

### Configuration
- Check configuration comes from API (not local files)
- Probe capabilities negotiated during registration
- Feature flags control which check types are enabled
- Environment-based configuration via command-line flags

### Tenant Isolation
- Every payload tagged with tenant ID
- Publisher maintains separate handlers per tenant
- Tenant metadata cached with expiration
- Secrets stored in remote secret store (not in agent)

## Code Organization

```
cmd/
  synthetic-monitoring-agent/    # Main agent binary
  test-api/                      # Test API server (outdated)
  synthetic-monitoring-proto/    # Proto manipulation

internal/
  adhoc/          # On-demand check execution
  checks/         # Check lifecycle management (Updater)
  k6runner/       # K6 script execution (local/remote)
  prober/         # Check type implementations
  pusher/         # Metrics/logs publishing (v1 and v2)
  scraper/        # Scheduled check execution
  tenants/        # Tenant metadata caching
  usage/          # Usage reporting
  secrets/        # Secret store integration
  limits/         # Rate limiting
  telemetry/      # Internal metrics
  version/        # Version information

pkg/pb/synthetic_monitoring/     # Generated protobuf code
```

## Development Notes

### Testing
- Tests use CGO for race detection (`CGO_ENABLED=1` in Makefile)
- Use `-short` flag for fast tests that skip integration tests
- Testhelper package (`internal/testhelper/`) provides common test utilities

### Linting
- Configuration in `.golangci.yaml`
- Custom rules via ruleguard in `internal/rules/rules.go`
- Import aliases enforced: `sm` for synthetic_monitoring, `dto` for Prometheus client model
- Maximum cyclomatic complexity: 20

### Docker Images
- Two variants: with/without Chromium (tagged `*-browser`)
- Use `--target release` for non-browser builds to keep images small
- Multi-stage Dockerfile defaults to browser variant

### Signals
- `SIGTERM`: Graceful shutdown
- `SIGUSR1`: Disconnect from API but keep running checks (allows another agent to connect)
  - After 1 minute, attempts to reconnect if no other agent has connected
  - Also available via `/disconnect` HTTP endpoint
  - Use case: Zero-downtime agent upgrades

### HTTP Endpoints
- `/ready`: Returns 200 when connected to API, 503 otherwise
- `/metrics`: Prometheus metrics for agent internals
- `/disconnect`: Sends SIGUSR1 to process
- `/debug/pprof`: Performance profiling (if enabled)

### Build System
- Uses make with modular includes from `scripts/make/`
- Builds for multiple platforms: `linux/amd64`, `linux/arm64`
- Version info injected at build time via ldflags
- Output in `dist/` directory

### API Integration
- gRPC with streaming: `GetChanges()` for continuous check updates
- Registration includes probe info (version, capabilities, location)
- Health checks via `Ping()` RPC with 30s interval
- Tenant metadata fetched via `GetTenant()` RPC and cached
- Adhoc checks via separate `GetAdHocChecks()` stream

### Performance Considerations
- Auto-memory limit based on cgroup/system memory (90% by default)
- Random offset per check within frequency to spread load
- Republishing keeps metrics fresh without overloading backends

[process]: https://www.planttext.com/api/plantuml/svg/dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0
[PlantUML]: https://www.planttext.com/?text=dLHDRy8m3BtdLqGz3wHTEKo88KsxJVi78VLA14swn47ityzE0ZHLcIQua8zdl-Tdf-k0ocFiZqAq2jLE1P3DTjE8WOwDDeEoA9lmOt4Fj5_qpXfqtjXkeGRJI1Ka_Sl_m3kmc0DuLKUGY0xoRLxMruDtFL3A61BajgrXHtV8adWXHEAHYvUaS2MrinOqIdS2Bzy-FrwdW02svTmxaCP-ETyhDCuAfT6S54BHpLWAsMuemeDsliG83nYzBGdUjwA5IUIKpyDtX81Ixq4VP40Fgh-apz2YAGEUnNAv_EFUYiwxE53nRf0alnoVXQJVbJhRotQaMpY3ZgdCXBe8BatWir8M6UwDfkOHtz5r8TsDIXn5P1dEQaZRYhwk_DP8EkeTPkDlKJErpkBci_CKF9oNpchZHbfNSeXXVx6aXYNI0hZwn8shXHPgD3suY8BPKdkdCzAQKERsxk2DCRCv7XxyUuIygJHLJbuVWB3iQ69DWAVyBjc8e7fWe8OG-C7kW6Y1RP0S9CIQblHL-WK0

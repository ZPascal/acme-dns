# Security Hardening and HA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix five security issues in the acme-dns codebase (SQL injection, bcrypt cost, rate limiting, CORS default, JSON error encoding) and add an HA deployment guide.

**Architecture:** All changes are in existing files — no new packages, no new dependencies. The rate limiter uses stdlib `sync` + `time` only. HA support is purely documentation (active-active with shared PostgreSQL requires no code changes).

**Tech Stack:** Go 1.26, `encoding/json`, `sync`, `time` (all stdlib)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `db.go:170-176` | Modify | Fix SQL injection in `NewTXTValuesInTransaction`, raise bcrypt cost |
| `util.go:16-18` | Modify | Fix `jsonError` to use `json.Marshal` |
| `types.go` | Modify | Add `RegisterRateLimit int` to `httpapi` |
| `config.cfg` | Modify | Set `register_ratelimit = 10`, change `corsorigins = []` |
| `main.go` | Modify | Add rate limiter middleware init; change CORS default |
| `api.go` | Modify | Add rate limiter middleware function |
| `api_test.go` | Modify | Update `setupRouter` CORS, add rate limit test |
| `util_test.go` | Modify | Add `jsonError` special-char test |
| `db_test.go` | Modify | Verify bcrypt cost (indirect — existing test still passes) |
| `docs/ha-deployment.md` | Create | Active-active HA deployment guide |
| `CHANGELOG.md` | Modify | Document CORS default breaking change |

---

## Task 1: Fix `jsonError` in `util.go`

**Files:**
- Modify: `util.go:16-18`
- Modify: `util_test.go`

- [ ] **Step 1: Write failing test**

Add to `util_test.go`:

```go
func TestJsonErrorWithSpecialChars(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"simple error", `{"error":"simple error"}`},
		{`error with "quotes"`, `{"error":"error with \"quotes\""}`},
		{"error with\nnewline", `{"error":"error with\nnewline"}`},
	}
	for _, tc := range cases {
		got := string(jsonError(tc.input))
		if got != tc.expected {
			t.Errorf("jsonError(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./... -run "TestJsonErrorWithSpecialChars" 2>&1
```
Expected: FAIL — the `fmt.Sprintf` version breaks on quoted strings.

- [ ] **Step 3: Fix `jsonError` in `util.go`**

Replace lines 16-18 in `util.go`:

```go
// Before
func jsonError(message string) []byte {
	return []byte(fmt.Sprintf("{\"error\": \"%s\"}", message))
}
```

With:

```go
func jsonError(message string) []byte {
	b, _ := json.Marshal(map[string]string{"error": message})
	return b
}
```

Add `"encoding/json"` to imports in `util.go`. Remove `"fmt"` if it is no longer used in `util.go` (check — it is currently only used in `jsonError`; after the fix it won't be needed).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestJsonErrorWithSpecialChars" 2>&1
```
Expected: PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add util.go util_test.go
git commit -m "fix: use json.Marshal in jsonError to handle special characters"
```

---

## Task 2: Fix SQL injection in `db.go`

**Files:**
- Modify: `db.go:170-176`

- [ ] **Step 1: Inspect current code**

Read `db.go` lines 169-176 to confirm the exact current code before editing:

```
func (d *acmedb) NewTXTValuesInTransaction(tx *sql.Tx, subdomain string) error {
    var err error
    instr := fmt.Sprintf("INSERT INTO txt (Subdomain, LastUpdate) values('%s', 0)", subdomain)
    _, _ = tx.Exec(instr)
    _, _ = tx.Exec(instr)
    return err
}
```

- [ ] **Step 2: Fix the function**

Replace `NewTXTValuesInTransaction` in `db.go` with:

```go
func (d *acmedb) NewTXTValuesInTransaction(tx *sql.Tx, subdomain string) error {
	instr := "INSERT INTO txt (Subdomain, LastUpdate) values($1, 0)"
	if Config.Database.Engine == "sqlite3" {
		instr = getSQLiteStmt(instr)
	}
	if _, err := tx.Exec(instr, subdomain); err != nil {
		return err
	}
	if _, err := tx.Exec(instr, subdomain); err != nil {
		return err
	}
	return nil
}
```

Note: the original function silently ignored errors from both `Exec` calls. The fix preserves the two-row insert (required for rolling TXT updates) and now returns errors. This also removes the `fmt` dependency from this function — check if `fmt` is still used elsewhere in `db.go` before removing its import (it is used in `Init()` for the version insert, so keep it).

- [ ] **Step 3: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass (DB upgrade tests exercise this function).

- [ ] **Step 4: Commit**

```bash
git add db.go
git commit -m "fix: use parameterized query in NewTXTValuesInTransaction to prevent SQL injection"
```

---

## Task 3: Raise bcrypt cost factor

**Files:**
- Modify: `db.go:193`

- [ ] **Step 1: Update bcrypt cost**

In `db.go`, find the line (currently 193):

```go
passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.Password), 10)
```

Change it to:

```go
passwordHash, err := bcrypt.GenerateFromPassword([]byte(a.Password), 12)
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass. (Bcrypt verification is cost-agnostic — stored hashes from old cost factor verify correctly against new registrations at cost 12.)

- [ ] **Step 3: Commit**

```bash
git add db.go
git commit -m "fix: raise bcrypt cost factor from 10 to 12"
```

---

## Task 4: Add rate limiting to `/register`

**Files:**
- Modify: `types.go`
- Modify: `config.cfg`
- Modify: `api.go`
- Modify: `main.go`
- Modify: `api_test.go`

- [ ] **Step 1: Write failing rate limit test**

Add to `api_test.go`:

```go
func TestRegisterRateLimit(t *testing.T) {
	// Build router with rate limit of 2/min
	api := httprouter.New()
	var dbcfg = dbsettings{Engine: "sqlite3", Connection: ":memory:"}
	var httpapicfg = httpapi{
		Port:              "8080",
		TLS:               "none",
		CorsOrigins:       []string{"*"},
		UseHeader:         true,
		HeaderName:        "X-Forwarded-For",
		RegisterRateLimit: 2,
	}
	Config = DNSConfig{API: httpapicfg, Database: dbcfg}
	newDB := new(acmedb)
	_ = newDB.Init(Config.Database.Engine, Config.Database.Connection)
	DB = newDB

	limiter := newRateLimiter(Config.API.RegisterRateLimit)
	api.POST("/register", rateLimitMiddleware(limiter, webRegisterPost))

	c := cors.New(cors.Options{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET", "POST"}})
	server := httptest.NewServer(c.Handler(api))
	defer server.Close()
	e := getExpect(t, server)

	// First two should succeed
	e.POST("/register").WithHeader("X-Forwarded-For", "10.0.0.1").Expect().Status(http.StatusCreated)
	e.POST("/register").WithHeader("X-Forwarded-For", "10.0.0.1").Expect().Status(http.StatusCreated)
	// Third from same IP should be rate limited
	e.POST("/register").WithHeader("X-Forwarded-For", "10.0.0.1").Expect().Status(http.StatusTooManyRequests)
	// Different IP should still work
	e.POST("/register").WithHeader("X-Forwarded-For", "10.0.0.2").Expect().Status(http.StatusCreated)
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./... -run "TestRegisterRateLimit" 2>&1
```
Expected: compile error — `RegisterRateLimit`, `newRateLimiter`, `rateLimitMiddleware` not defined.

- [ ] **Step 3: Add `RegisterRateLimit` to `types.go`**

Add to the `httpapi` struct in `types.go`:

```go
RegisterRateLimit int `toml:"register_ratelimit"`
```

- [ ] **Step 4: Add rate limiter to `api.go`**

Add to `api.go`:

```go
import (
	"net"
	"sync"
	"time"
)

type rateBucket struct {
	count    int
	resetAt  time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*rateBucket
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   limit,
		window:  time.Minute,
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok || now.After(b.resetAt) {
		rl.buckets[ip] = &rateBucket{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

func (rl *rateLimiter) cleanup() {
	for range time.Tick(10 * time.Minute) {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.buckets {
			if now.After(b.resetAt) {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func rateLimitMiddleware(rl *rateLimiter, next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		if rl == nil {
			next(w, r, ps)
			return
		}
		ip := r.RemoteAddr
		if Config.API.UseHeader {
			ips := getIPListFromHeader(r.Header.Get(Config.API.HeaderName))
			if len(ips) > 0 {
				ip = ips[0]
			}
		} else {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err == nil {
				ip = host
			}
		}
		if !rl.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(jsonError("rate_limit_exceeded"))
			return
		}
		next(w, r, ps)
	}
}
```

- [ ] **Step 5: Wire rate limiter into `main.go`**

In `startHTTPAPI()`, after the `api := httprouter.New()` line, add:

```go
var registerLimiter *rateLimiter
if config.API.RegisterRateLimit > 0 {
	registerLimiter = newRateLimiter(config.API.RegisterRateLimit)
}
```

Change the registration route registration from:

```go
if !config.API.DisableRegistration {
	api.POST("/register", webRegisterPost)
}
```

To:

```go
if !config.API.DisableRegistration {
	api.POST("/register", rateLimitMiddleware(registerLimiter, webRegisterPost))
}
```

- [ ] **Step 6: Add to `config.cfg`**

Add inside the `[api]` section of `config.cfg`, after `header_name`:

```toml
# max registrations per minute per source IP (0 = unlimited)
register_ratelimit = 10
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./... -run "TestRegisterRateLimit" 2>&1
```
Expected: PASS.

- [ ] **Step 8: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add types.go api.go main.go config.cfg api_test.go
git commit -m "feat: add per-IP rate limiting to /register endpoint"
```

---

## Task 5: Harden CORS default

**Files:**
- Modify: `config.cfg`
- Modify: `CHANGELOG.md`

The CORS default in `config.cfg` is `corsorigins = ["*"]`. Change it to empty (deny all by default). The code in `main.go` already reads from `Config.API.CorsOrigins` — no code change needed. Tests in `api_test.go` already set `CorsOrigins: []string{"*"}` explicitly in `setupRouter`, so they are unaffected.

- [ ] **Step 1: Update `config.cfg`**

Replace in `config.cfg`:

```toml
# CORS AllowOrigins, wildcards can be used
corsorigins = [
    "*"
]
```

With:

```toml
# CORS AllowOrigins, wildcards can be used
# WARNING: empty list denies all cross-origin requests (secure default)
# Set explicitly for browser-based clients, e.g.: corsorigins = ["https://admin.example.com"]
corsorigins = []
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass (tests use `setupRouter` which sets `CorsOrigins: []string{"*"}` explicitly, so they are unaffected).

- [ ] **Step 3: Update `CHANGELOG.md`**

If `CHANGELOG.md` exists, add at the top under a new version header:

```markdown
## Unreleased

### Breaking Changes
- `corsorigins` default changed from `["*"]` to `[]` (deny all cross-origin by default).
  If you rely on the default wildcard CORS behavior, explicitly set `corsorigins = ["*"]` in your config.

### Security
- Fixed SQL injection pattern in `NewTXTValuesInTransaction` (defense-in-depth, input was already sanitized)
- Raised bcrypt cost factor from 10 to 12
- Added per-IP rate limiting on `/register` endpoint (`register_ratelimit` config option)
- `jsonError` now uses `json.Marshal` to safely encode error messages
```

- [ ] **Step 4: Commit**

```bash
git add config.cfg CHANGELOG.md
git commit -m "fix: harden CORS default to deny cross-origin by default; document breaking change"
```

---

## Task 6: Write HA deployment guide

**Files:**
- Create: `docs/ha-deployment.md`

- [ ] **Step 1: Create `docs/` if needed**

```bash
mkdir -p docs
```

- [ ] **Step 2: Create `docs/ha-deployment.md`**

```markdown
# acme-dns High Availability Deployment Guide

## Model: Active-Active with Shared PostgreSQL

Multiple acme-dns instances run simultaneously behind a load balancer. All instances share one PostgreSQL database. The HTTP API is stateless — no session affinity is required. DNS can be distributed across instances using round-robin NS records.

## Prerequisites

- PostgreSQL 14+ (primary/replica HA managed externally, e.g. Patroni, RDS Multi-AZ, Cloud SQL HA)
- A load balancer for HTTP: HAProxy, nginx, or cloud LB (AWS ALB, GCP HTTPS LB)
- Optional: pgBouncer for connection pooling under high registration volume
- At least 2 acme-dns instances

## Database Configuration

All instances share the same connection string:

```toml
[database]
engine = "postgres"
connection = "postgres://acmedns:password@pg-primary.internal/acmedns_db"
```

Recommended PostgreSQL settings (`postgresql.conf`):
```
max_connections = 100          # scale with instance count
idle_in_transaction_session_timeout = 30s
```

Database migrations run automatically on startup. Run only **one instance first** on initial deploy, then start the others once the schema is ready.

## Instance Configuration

Each instance has an identical `config.cfg` except for `[api].ip` (bind address per host). Key settings:

```toml
[database]
engine = "postgres"
connection = "postgres://acmedns:password@pg-primary.internal/acmedns_db"

[api]
ip = "0.0.0.0"     # or specific interface
port = "443"
tls = "cert"       # manage certs externally in HA setups
```

Do **not** use `tls = "letsencrypt"` on multiple instances — each would race for certificate renewal. Use a shared cert (wildcard or SAN) or terminate TLS at the load balancer.

## DNS Load Balancing

Configure multiple A records for the acme-dns NS hostname with a low TTL (60s) for fast failover:

```
auth.example.org.  60  IN  A  203.0.113.1   ; instance 1
auth.example.org.  60  IN  A  203.0.113.2   ; instance 2
```

Resolvers will round-robin between instances for DNS queries.

## HTTP API Load Balancing

### HAProxy Example

```haproxy
frontend acmedns-api
    bind *:443 ssl crt /etc/ssl/acmedns.pem
    default_backend acmedns-instances

backend acmedns-instances
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server acmedns1 10.0.0.1:443 check ssl verify none
    server acmedns2 10.0.0.2:443 check ssl verify none
```

### pgBouncer (optional, for high registration volume)

```ini
[databases]
acmedns_db = host=pg-primary.internal port=5432 dbname=acmedns_db

[pgbouncer]
pool_mode = transaction
max_client_conn = 200
default_pool_size = 20
```

Then point acme-dns at pgBouncer instead of PostgreSQL directly:
```toml
connection = "postgres://acmedns:password@pgbouncer.internal:5432/acmedns_db"
```

## Health Check

The `/health` endpoint returns HTTP 200 when the instance is ready. Use it for:
- Load balancer health probes (see HAProxy config above)
- Kubernetes readiness/liveness probes:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 3
  periodSeconds: 5
```

## Failure Modes

| Failure | Impact | Recovery |
|---------|--------|----------|
| One instance dies | DNS queries and HTTP API handled by remaining instances | Automatic — load balancer stops routing to failed instance |
| PostgreSQL unreachable | HTTP API returns 500; DNS continues serving cached records from resolver caches | Restore PostgreSQL; instances reconnect automatically on next request |
| PostgreSQL replica failover | Transient errors during failover window (~30s for Patroni) | Use pgBouncer to buffer connections; instances retry |

## Upgrade Procedure (Rolling Restart)

1. Run DB migrations once: `acme-dns -c config.cfg --migrate` (or start one instance; migrations are idempotent)
2. Stop and restart instances one at a time, verifying `/health` before moving to the next
3. DNS resolvers cache responses — TTL (default 60s for NS records) means no visible downtime

## Security Notes in HA

- Rate limiting (`register_ratelimit`) is per-instance, per-IP (in-memory). In active-active, a single IP can register up to `register_ratelimit * N` times per minute across N instances. For stricter enforcement, place rate limiting at the load balancer (HAProxy `stick-tables`, nginx `limit_req`).
- All instances share the same `[api.admin].token` — rotate it by updating all instances' configs and restarting them.
```

- [ ] **Step 3: Commit**

```bash
git add docs/ha-deployment.md
git commit -m "docs: add active-active HA deployment guide with HAProxy and pgBouncer examples"
```

---

## Task 7: Final integration check

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v 2>&1 | tail -40
```
Expected: all tests pass.

- [ ] **Step 2: Build binary**

```bash
go build ./... 2>&1
```
Expected: builds with no errors.

- [ ] **Step 3: Vet**

```bash
go vet ./... 2>&1
```
Expected: no issues.

- [ ] **Step 4: Final commit if needed**

```bash
git status
```
If unstaged files remain:
```bash
git add -A
git commit -m "chore: finalize security-ha feature"
```

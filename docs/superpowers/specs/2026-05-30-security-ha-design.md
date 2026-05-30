# Design: feature/security-ha

**Date:** 2026-05-30  
**Branch:** `feature/security-ha`  
**Status:** Approved

## Overview

Two concerns addressed together: code-level security hardening and HA deployment documentation. No new infrastructure dependencies are introduced. The HA model is active-active with a shared PostgreSQL backend — no changes to the acme-dns application code are needed for HA beyond what already exists; the work is in documentation and operational guidance.

## Security Fixes

### 1. SQL Injection Fix — `db.go:172`

Replace string interpolation with a parameterized query. The existing code in `NewTXTValuesInTransaction` uses `fmt.Sprintf` to build an INSERT statement with a subdomain value:

```go
// Before (db.go:172)
instr := fmt.Sprintf("INSERT INTO txt (Subdomain, LastUpdate) values('%s', 0)", subdomain)
db.DB.Exec(instr)

// After
_, err = db.DB.Exec("INSERT INTO txt (Subdomain, LastUpdate) values($1, 0)", subdomain)
```

The SQLite variant uses `?` instead of `$1` — the fix handles both via the existing dual-query pattern already used elsewhere in `db.go`.

Input is already sanitized before this point (via `sanitizeString`), so this is defense-in-depth rather than an emergency fix. It is still the correct pattern.

### 2. Bcrypt Cost Factor — `db.go:193`

Raise cost from 10 to 12. Registration is an infrequent operation; the extra ~300ms per registration is imperceptible to users and meaningfully increases brute-force cost.

```go
// Before
bcrypt.GenerateFromPassword([]byte(a.Password), 10)
// After
bcrypt.GenerateFromPassword([]byte(a.Password), 12)
```

Existing stored hashes are unaffected — bcrypt verification works across cost factors.

### 3. Rate Limiting on `/register`

Add an in-memory token bucket rate limiter using only stdlib (`sync`, `time`). One bucket per source IP, cleaned up after 10 minutes of inactivity.

New config field in `[api]`:

```toml
[api]
register_ratelimit = 10   # max registrations per minute per IP; 0 = unlimited
```

The middleware returns HTTP 429 with `{"error": "rate limit exceeded"}` when the bucket is exhausted. The rate limiter is skipped entirely when `register_ratelimit = 0`.

### 4. CORS Default Hardening

Change the default `corsorigins` from `["*"]` to `[]` (empty — deny all cross-origin by default). Operators deploying behind a web frontend must explicitly set allowed origins.

```toml
# config.cfg default changed from:
corsorigins = ["*"]
# to:
corsorigins = []
```

Existing deployments that already set `corsorigins` explicitly are unaffected. This is a breaking change for deployments relying on the default `*` — documented in the changelog.

### 5. JSON Error Response — `util.go:17`

Replace `fmt.Sprintf` string interpolation in `toErrorJSON` with proper `json.Marshal`:

```go
// Before
return []byte(fmt.Sprintf("{\"error\": \"%s\"}", message))

// After
b, _ := json.Marshal(map[string]string{"error": message})
return b
```

Error strings are currently all hardcoded so the risk is theoretical, but the fix is two lines and eliminates the class of bug entirely.

## HA Deployment

### Model: Active-Active with Shared PostgreSQL

Multiple acme-dns instances run simultaneously behind a load balancer. All instances share one PostgreSQL database. The acme-dns HTTP API is stateless — no session affinity is required. DNS queries can be distributed across instances using round-robin NS records.

### New Documentation: `docs/ha-deployment.md`

Covers:

1. **Prerequisites** — PostgreSQL 14+, a load balancer (HAProxy, nginx, or cloud LB), optional pgBouncer for connection pooling
2. **Database configuration** — connection string format, recommended PostgreSQL settings (`max_connections`, `pool_size`)
3. **Instance configuration** — identical `config.cfg` on each node except `[api].ip`; `[database].engine = "postgres"`
4. **DNS load balancing** — configure multiple A records for the NS hostname, or use anycast; round-robin TTL recommendation (60s)
5. **HTTP API load balancing** — HAProxy config snippet for `/register`, `/update`, `/health`; use `GET /health` as the health check probe
6. **pgBouncer** — connection pooling config for high-registration-volume deployments
7. **Failure modes** — single instance failure: DNS queries handled by remaining instances, no data loss; PostgreSQL failure: DNS continues serving cached records, HTTP API returns 500 until DB recovers
8. **Upgrade procedure** — rolling restart (one instance at a time), DB migrations run once before restart

## Files Changed

| File | Change |
|------|--------|
| `db.go` | Parameterized INSERT in `NewTXTValuesInTransaction`, bcrypt cost 10 → 12 |
| `util.go` | `toErrorJSON` uses `json.Marshal` |
| `api.go` | Rate limiter middleware on `/register` |
| `main.go` | Wire rate limiter middleware; initialize limiter from config |
| `config.cfg` | `register_ratelimit = 10`, `corsorigins = []` |
| `types.go` | Add `RegisterRateLimit int` field to `APIConfig` |
| `docs/ha-deployment.md` | New HA deployment guide |
| `CHANGELOG.md` | Note CORS default change as breaking |

## Testing

- Existing tests updated for new CORS default (tests that assumed `*` need explicit origin config)
- New test: rate limiter returns 429 after limit exceeded, resets after window
- New test: `toErrorJSON` with strings containing quotes/special characters produces valid JSON
- Verify existing auth and DB tests still pass (bcrypt cost change is transparent to test suite)

## Non-Goals

- Redis-based distributed rate limiting (in-memory is sufficient for single-instance and active-active where per-instance limits are acceptable)
- DNSSEC
- Mutual TLS between instances
- Automatic failover orchestration (that belongs to the load balancer / PostgreSQL HA layer)

# Design: feature/dns-api-extended

**Date:** 2026-05-30  
**Branch:** `feature/dns-api-extended`  
**Status:** Approved

## Overview

Extend acme-dns with a privileged admin HTTP API for full DNS record management (A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, PTR). The existing `/register` and `/update` ACME workflow is unchanged. Admin access is controlled by a single token defined in `config.cfg`. The DNS server serves both the existing ACME TXT records and the new managed records from the database.

## Config Changes

New `[api.admin]` section in `config.cfg` and corresponding `AdminConfig` struct in `types.go`:

```toml
[api.admin]
token = ""   # required; empty string = admin API disabled
```

The admin API is opt-in. An empty or absent token disables all `/admin/*` routes at startup.

## Database

New table `dns_records` added to the existing schema (both SQLite and PostgreSQL variants):

```sql
CREATE TABLE IF NOT EXISTS dns_records (
    id      TEXT PRIMARY KEY,
    name    TEXT NOT NULL,
    type    TEXT NOT NULL,
    value   TEXT NOT NULL,
    ttl     INTEGER NOT NULL DEFAULT 300,
    created INTEGER NOT NULL
);
```

- `id`: UUIDv4, generated server-side on creation
- `name`: fully qualified domain name (e.g. `sub.example.com`)
- `type`: one of `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `SRV`, `CAA`, `PTR`
- `value`: record value (format varies by type; validated server-side)
- `ttl`: time-to-live in seconds, default 300
- `created`: Unix timestamp

## API Endpoints

All endpoints require `Authorization: Bearer <token>` header. Token comparison uses `subtle.ConstantTimeCompare` to prevent timing attacks.

| Method | Path | Status | Description |
|--------|------|--------|-------------|
| `GET` | `/admin/records` | 200 | List all records. Optional query params: `?type=A`, `?name=sub.example.com` |
| `POST` | `/admin/records` | 201 | Create a record. Body: `{name, type, value, ttl}` |
| `PUT` | `/admin/records/{id}` | 200 | Update a record. Body: `{name, type, value, ttl}` |
| `DELETE` | `/admin/records/{id}` | 204 | Delete a record |

### Request/Response Shapes

**POST /admin/records**
```json
{ "name": "sub.example.com", "type": "A", "value": "1.2.3.4", "ttl": 300 }
```
Response (201):
```json
{ "id": "<uuid>", "name": "sub.example.com", "type": "A", "value": "1.2.3.4", "ttl": 300, "created": 1748563200 }
```

**GET /admin/records**
Response (200):
```json
[{ "id": "...", "name": "...", "type": "A", "value": "...", "ttl": 300, "created": 1748563200 }]
```

**Error responses** follow existing pattern: `{"error": "<message>"}` with appropriate HTTP status codes.

## DNS Integration

`dns.go`'s `answer()` gains a second lookup step: after checking ACME TXT records (existing logic, takes precedence), it queries `dns_records` by name and type. Matched records are returned as authoritative answers with the stored TTL.

The existing ACME TXT flow is untouched — records in the `txt` table always win over `dns_records` for the same name.

## Validation

`validation.go` additions:
- `validRecordType(t string) bool` — allowlist of 9 supported types
- `validRecordValue(t, v string) bool` — per-type format checks (IP for A/AAAA, hostname for CNAME/NS/MX, etc.)
- `validTTL(n int) bool` — range check (1–86400)

## Files Changed

| File | Change |
|------|--------|
| `config.cfg` | Add `[api.admin]` section |
| `types.go` | Add `AdminConfig` struct, wire into `Config` |
| `db.go` | Add `dns_records` table creation, `CreateRecord`, `ListRecords`, `UpdateRecord`, `DeleteRecord` methods |
| `api.go` | Add `adminListRecords`, `adminCreateRecord`, `adminUpdateRecord`, `adminDeleteRecord` handlers |
| `main.go` | Add `/admin` route group with Bearer middleware; skip if token empty |
| `dns.go` | Extend `answer()` to fall through to `dns_records` lookup |
| `validation.go` | Add record type/value/TTL validators |

## Testing

- Unit tests for each new validator in `validation_test.go`
- HTTP tests for all 4 admin endpoints in `api_test.go` (auth required, auth missing, invalid input, DB error)
- DNS integration test verifying managed records are served correctly
- Test that existing ACME TXT tests are unaffected

## Non-Goals

- Zone import/export (BIND format)
- DNSSEC signing
- Record history or audit log
- Multi-tenant admin access (single token only)

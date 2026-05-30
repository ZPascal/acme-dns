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

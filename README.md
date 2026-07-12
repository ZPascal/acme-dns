[![Go](https://github.com/fritterhoff/acme-dns/actions/workflows/go_cov.yml/badge.svg)](https://github.com/fritterhoff/acme-dns/actions/workflows/go_cov.yml) [![codecov](https://codecov.io/gh/fritterhoff/acme-dns/branch/master/graph/badge.svg?token=NA6E3FJ5Z5)](https://codecov.io/gh/fritterhoff/acme-dns) [![Go Report Card](https://goreportcard.com/badge/github.com/fritterhoff/acme-dns)](https://goreportcard.com/report/github.com/fritterhoff/acme-dns)

# acme-dns

A simplified DNS server with a RESTful HTTP API to provide a simple way to automate ACME DNS challenges.

## Why?

Many DNS servers do not provide an API to enable automation for the ACME DNS challenges. Those which do, give the keys way too much power.
Leaving the keys lying around your random boxes is too often a requirement to have a meaningful process automation.

Acme-dns provides a simple API exclusively for TXT record updates and should be used with ACME magic "\_acme-challenge" - subdomain CNAME records. This way, in the unfortunate exposure of API keys, the effects are limited to the subdomain TXT record in question.

So basically it boils down to **accessibility** and **security**.

For longer explanation of the underlying issue and other proposed solutions, see a blog post on the topic from EFF deeplinks blog: https://www.eff.org/deeplinks/2018/02/technical-deep-dive-securing-automation-acme-dns-challenge-validation

## Features

- Simplified DNS server, serving your ACME DNS challenges (TXT)
- Custom records (have your required A, AAAA, NS, etc. records served)
- Admin API for managing DNS records with Bearer token authentication
- OpenAPI 3.1 specification served at `GET /openapi.json`
- MCP server binary (`acme-dns-mcp`) exposing acme-dns as structured tools for MCP-compatible AI agents
- HTTP API automatically acquires and uses Let's Encrypt TLS certificate
- Limit /update API endpoint access to specific CIDR mask(s), defined in the /register request
- Supports SQLite & PostgreSQL as DB backends
- Rolling update of two TXT records to be able to answer to challenges for certificates that have both names: `yourdomain.tld` and `*.yourdomain.tld`, as both of the challenges point to the same subdomain.
- Simple deployment (it's Go after all)

## Usage

A client application for acme-dns with support for Certbot authentication hooks is available at: [https://github.com/acme-dns/acme-dns-client](https://github.com/acme-dns/acme-dns-client).

Using acme-dns is a three-step process (provided you already have the self-hosted server set up):

- Get credentials and unique subdomain (simple POST request to e.g. https://auth.acme-dns.io/register)
- Create a (ACME magic) CNAME record to your existing zone, pointing to the subdomain you got from the registration. (e.g. `_acme-challenge.domainiwantcertfor.tld. CNAME a097455b-52cc-4569-90c8-7a4b97c6eba8.auth.example.org`)
- Use your credentials to POST new DNS challenge values to an acme-dns server for the CA to validate from.
- Crontab and forget.

## API

### Admin API

The Admin API allows authenticated administrators to manage DNS records stored in the database. All endpoints require Bearer token authentication and are disabled by default.

**Enabling the Admin API**

To enable the Admin API, set an admin token in your configuration:

```toml
[api.admin]
token = "your-secret-admin-token-here"
```

**Authentication**

All Admin API requests require the `Authorization` header with a Bearer token:

```
Authorization: Bearer your-secret-admin-token-here
```

#### List records

Retrieve all DNS records, optionally filtered by record type or name.

`GET /admin/records?type=TYPE&name=NAME`

**Query Parameters:**
- `type` (optional): Filter by DNS record type (A, AAAA, NS, TXT, CNAME, MX, etc.)
- `name` (optional): Filter by record name (domain name)

**Response**

`Status: 200 OK`

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "example.org.",
    "type": "A",
    "value": "192.0.2.1",
    "ttl": 3600,
    "created": 1719259200
  }
]
```

#### Create record

Create a new DNS record.

`POST /admin/records`

**Request body:**

```json
{
  "name": "example.org",
  "type": "A",
  "value": "192.0.2.1",
  "ttl": 3600
}
```

**Fields:**
- `name`: Domain name for the record (required)
- `type`: DNS record type: A, AAAA, NS, TXT, CNAME, MX, SOA, SRV, PTR (required)
- `value`: Record value (required)
  - A: IPv4 address
  - AAAA: IPv6 address
  - NS, CNAME, PTR: Hostname
  - MX: `<preference> <exchange hostname>`, e.g. `0 mail.example.com.`
  - TXT: Text string (automatically quoted if not already)
  - SRV: Service record format
- `ttl`: Time-to-live in seconds (optional, defaults to 300)

**Response**

`Status: 201 Created`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "example.org.",
  "type": "A",
  "value": "192.0.2.1",
  "ttl": 3600,
  "created": 1719259200
}
```

#### Update record

Update an existing DNS record by ID.

`PUT /admin/records/:id`

**Request body:**

```json
{
  "name": "example.org",
  "type": "A",
  "value": "192.0.2.2",
  "ttl": 3600
}
```

**Response**

`Status: 200 OK`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "example.org.",
  "type": "A",
  "value": "192.0.2.2",
  "ttl": 3600,
  "created": 1719259200
}
```

#### Delete record

Delete a DNS record by ID.

`DELETE /admin/records/:id`

**Response**

`Status: 204 No Content`

### Register endpoint

The method returns a new unique subdomain and credentials needed to update your record.
Fulldomain is where you can point your own `_acme-challenge` subdomain CNAME record to.
With the credentials, you can update the TXT response in the service to match the challenge token, later referred as \_\_\_validation_token_received_from_the_ca\_\_\_, given out by the Certificate Authority.

**Optional:**: You can POST JSON data to limit the `/update` requests to predefined source networks using CIDR notation.

`POST /register`

#### OPTIONAL Example input

```json
{
  "allowfrom": ["192.168.100.1/24", "1.2.3.4/32", "2002:c0a8:2a00::0/40"]
}
```

`Status: 201 Created`

```json
{
  "allowfrom": ["192.168.100.1/24", "1.2.3.4/32", "2002:c0a8:2a00::0/40"],
  "fulldomain": "8e5700ea-a4bf-41c7-8a77-e990661dcc6a.auth.acme-dns.io",
  "password": "htB9mR9DYgcu9bX_afHF62erXaH2TS7bg9KW3F7Z",
  "subdomain": "8e5700ea-a4bf-41c7-8a77-e990661dcc6a",
  "username": "c36f50e8-4632-44f0-83fe-e070fef28a10"
}
```

#### Rate limiting

`/register` is rate limited per source IP (`register_ratelimit` in `[api]`, default `10` registrations per minute; `0` disables the limit). The limit is enforced in-memory per instance, keyed by the client IP (or the first address from `header_name` when `use_header = true`). Exceeding it returns:

`Status: 429 Too Many Requests`

```json
{
  "error": "rate_limit_exceeded"
}
```

### Update endpoint

The method allows you to update the TXT answer contents of your unique subdomain. Usually carried automatically by automated ACME client.

`POST /update`

#### Required headers

| Header name | Description                                | Example                                               |
| ----------- | ------------------------------------------ | ----------------------------------------------------- |
| X-Api-User  | UUIDv4 username received from registration | `X-Api-User: c36f50e8-4632-44f0-83fe-e070fef28a10`    |
| X-Api-Key   | Password received from registration        | `X-Api-Key: htB9mR9DYgcu9bX_afHF62erXaH2TS7bg9KW3F7Z` |

#### Example input

```json
{
  "subdomain": "8e5700ea-a4bf-41c7-8a77-e990661dcc6a",
  "txt": "___validation_token_received_from_the_ca___"
}
```

#### Response

`Status: 200 OK`

```json
{
  "txt": "___validation_token_received_from_the_ca___"
}
```

### Health check endpoint

The method can be used to check readiness and/or liveness of the server. It will return status code 200 on success or won't be reachable.

`GET /health`

### OpenAPI specification

A static OpenAPI 3.1 document describing every endpoint above (`/register`, `/update`, `/health`, and the admin record CRUD endpoints) is embedded in the binary and served without authentication:

`GET /openapi.json`

Point any OpenAPI-compatible tool (Swagger UI, Postman, code generators, etc.) at this URL to explore or generate a client for the API.

## MCP Server

`cmd/acme-dns-mcp` builds a separate binary, `acme-dns-mcp`, that exposes the acme-dns HTTP API as [Model Context Protocol](https://modelcontextprotocol.io) (MCP) tools over stdio, so MCP-compatible AI agents (Claude Desktop, Claude Code, etc.) can register subdomains, publish ACME challenge records, and manage DNS records on your behalf.

It is a thin, stateless JSON-RPC 2.0 wrapper: every tool call becomes a single HTTP request against a running acme-dns instance (configured via `base_url`) and the HTTP response is translated back into an MCP tool result. It does not talk to the database or DNS server directly.

### Installing

`acme-dns-mcp` ships in the same release archive as `acme-dns` (see [Installation](#installation)). Download the archive for your platform from the [latest release](https://github.com/zpascal/acme-dns/releases/latest), extract it, and move the binary to a directory in your $PATH, for example:

`sudo mv acme-dns-mcp /usr/local/bin`

### Configuring

Configuration is read from a TOML file, then overridden by environment variables. Both are optional — a missing config file is not an error, and any field can be supplied purely through the environment.

- Default file location: `~/.acme-dns-mcp/config.toml`
- Override the file location with `ACMEDNS_MCP_CONFIG=/path/to/config.toml`

```toml
# ~/.acme-dns-mcp/config.toml
base_url = "https://auth.example.org"
admin_token = "your-secret-admin-token-here"
username = "c36f50e8-4632-44f0-83fe-e070fef28a10"
password = "paasword"
```

| TOML key      | Environment variable  | Used by                                                                           |
|---------------|-----------------------|-----------------------------------------------------------------------------------|
| `base_url`    | `ACMEDNS_BASE_URL`    | All tools — the acme-dns instance to talk to                                      |
| `admin_token` | `ACMEDNS_ADMIN_TOKEN` | `list_dns_records`, `create_dns_record`, `update_dns_record`, `delete_dns_record` |
| `username`    | `ACMEDNS_USERNAME`    | `update_txt_record`                                                               |
| `password`    | `ACMEDNS_PASSWORD`    | `update_txt_record`                                                               |

Credentials are only ever read from this configuration — MCP tool arguments never accept a token, username, or password, so a malicious or careless prompt cannot exfiltrate credentials through tool call arguments.

### Registering with an MCP client

Most MCP clients (e.g. Claude Desktop, Claude Code) accept a `command`+`env` style server definition:

```json
{
  "mcpServers": {
    "acme-dns": {
      "command": "/usr/local/bin/acme-dns-mcp",
      "env": {
        "ACMEDNS_BASE_URL": "https://auth.example.org",
        "ACMEDNS_ADMIN_TOKEN": "your-secret-admin-token-here"
      }
    }
  }
}
```

### Available tools

| Tool                 | Requires               | Arguments                                                         | Description                                                                    |
|----------------------|------------------------|-------------------------------------------------------------------|--------------------------------------------------------------------------------|
| `health_check`       | —                      | none                                                              | Checks that the acme-dns instance is reachable.                                |
| `register_subdomain` | —                      | `allowfrom` (optional array of CIDRs)                             | Registers a new subdomain and returns credentials, mirroring `POST /register`. |
| `update_txt_record`  | `username`, `password` | `subdomain` (required), `txt` (required, exactly 43 characters)   | Publishes an ACME challenge TXT value, mirroring `POST /update`.               |
| `list_dns_records`   | `admin_token`          | `type`, `name` (both optional filters)                            | Lists managed DNS records, mirroring `GET /admin/records`.                     |
| `create_dns_record`  | `admin_token`          | `name`, `type`, `value` (required), `ttl` (optional, default 300) | Creates a managed DNS record.                                                  |
| `update_dns_record`  | `admin_token`          | `id`, `name`, `type`, `value` (required), `ttl` (optional)        | Updates a managed DNS record by ID.                                            |
| `delete_dns_record`  | `admin_token`          | `id` (required)                                                   | Deletes a managed DNS record by ID.                                            |

Tools that require `admin_token` or `username`/`password` return `{"error": "..."}` (with `isError: true`, see below) instead of making a request when the corresponding configuration is missing.

### Error handling

Every `tools/call` result carries an `isError` flag alongside its content, per the MCP specification:

- **`isError: false`** — the underlying acme-dns API call succeeded (2xx); the content is the API's response.
- **`isError: true`** — the acme-dns API rejected the request (e.g. `invalid_ttl`, `record_not_found`, `unauthorized`) or the required configuration was missing; the content carries the `{"error": "..."}` body so an MCP client can distinguish a failed operation from a successful one, rather than a failure being reported as an apparently-successful result.

A JSON-RPC-level error (the `error` field on the response, separate from `isError`) is only used for protocol-level failures: an unknown tool name, an unreachable acme-dns instance, or a malformed request.

## Self-hosted

You are encouraged to run your own acme-dns instance, because you are effectively authorizing the acme-dns server to act on your behalf in providing the answer to the challenging CA, making the instance able to request (and get issued) a TLS certificate for the domain that has CNAME pointing to it.

See the INSTALL section for information on how to do this.

## Installation

1. Download the archive for your platform from the [latest release](https://github.com/zpascal/acme-dns/releases/latest) (e.g. `acme-dns_<version>_linux_amd64.tar.gz`, also available for `darwin_amd64`, `darwin_arm64`, `linux_arm64`, and `linux_386`). Each archive contains both the `acme-dns` server binary and the `acme-dns-mcp` binary (see [MCP Server](#mcp-server)).
2. Extract the archive and move the acme-dns binary to a directory in your $PATH, for example:
   `sudo mv acme-dns /usr/local/bin`
3. Edit config.cfg to suit your needs (see [configuration](#configuration)). `acme-dns` will read the configuration file from `/etc/acme-dns/config.cfg` or `./config.cfg`, or a location specified with the `-c` flag.
4. If your system has systemd, you can optionally install acme-dns as a service so that it will start on boot and be tracked by systemd. This also allows us to add the `CAP_NET_BIND_SERVICE` capability so that acme-dns can be run by a user other than root.
   1. Make sure that you have moved the configuration file to `/etc/acme-dns/config.cfg` so that acme-dns can access it globally.
   2. Make sure that the acme-dns executable is at `/usr/local/bin/acme-dns` (any location will work, just be sure to change `acme-dns.service` to match).
   3. Create a minimal acme-dns user: `sudo adduser --system --gecos "acme-dns Service" --disabled-password --group --home /var/lib/acme-dns acme-dns`.
   4. Move the systemd service unit from `acme-dns.service` to `/etc/systemd/system/acme-dns.service`.
   5. Reload systemd units: `sudo systemctl daemon-reload`.
   6. Enable acme-dns on boot: `sudo systemctl enable acme-dns.service`.
   7. Run acme-dns: `sudo systemctl start acme-dns.service`.
5. If you did not install the systemd service, run `acme-dns`. Please note that acme-dns needs to open a privileged port (53, domain), so it needs to be run with elevated privileges.

### Upgrading to v1.0.0

**Breaking Changes:**

- **CORS Configuration**: The default `corsorigins` has changed from `["*"]` (allow all) to `[]` (deny all). This is a security-focused change to deny cross-origin requests by default.
  - If you rely on the previous wildcard CORS behavior, explicitly set `corsorigins = ["*"]` in your `[api]` config section.
  - If you're integrating acme-dns with a web application on a different origin, configure `corsorigins` with the specific origins you trust.

### Using Docker

1. Pull the latest acme-dns Docker image: `docker pull ghcr.io/zpascal/acme-dns`.
2. Create directories: `config` for the configuration file, and `data` for the sqlite3 database.
3. Copy [configuration template](https://raw.githubusercontent.com/zpascal/acme-dns/master/config.cfg) to `config/config.cfg`.
4. Modify the `config.cfg` to suit your needs.
5. Run Docker. This example expects that you have `port = "80"` in your `config.cfg`:

   ```bash
   docker run --rm --name acmedns                \
   -p 53:53                                      \
   -p 53:53/udp                                  \
   -p 80:80                                      \
   -v /path/to/your/config:/etc/acme-dns:ro      \
   -v /path/to/your/data:/var/lib/acme-dns       \
   -d ghcr.io/zpascal/acme-dns
   ```

### Docker Compose

1. Create directories: `config` for the configuration file, and `data` for the sqlite3 database.
2. Copy [configuration template](https://raw.githubusercontent.com/zpascal/acme-dns/master/config.cfg) to `config/config.cfg`.
3. Copy [docker-compose.yml from the project](https://raw.githubusercontent.com/zpascal/acme-dns/master/docker-compose.yml), or create your own.
4. Edit the `config/config.cfg` and `docker-compose.yml` to suit your needs, and run `docker-compose up -d`.

## DNS Records

Note: In this documentation:

- `auth.example.org` is the hostname of the acme-dns server
- acme-dns will serve `*.auth.example.org` records
- `198.51.100.1` is the **public** IP address of the system running acme-dns

These values should be changed based on your environment.

You will need to add some DNS records on your domain's regular DNS server:

- `NS` record for `auth.example.org` pointing to `auth.example.org` (this means, that `auth.example.org` is responsible for any `*.auth.example.org` records)
- `A` record for `auth.example.org` pointing to `198.51.100.1`
- If using IPv6, an `AAAA` record pointing to the IPv6 address.
- Each domain you will be authenticating will need a `_acme-challenge` `CNAME` subdomain added. The [client](README.md#clients) you use will explain how to do this.

## Testing It Out

You may want to test that acme-dns is working before using it for real queries.

1. Confirm that DNS lookups for the acme-dns subdomain works as expected: `dig auth.example.org`.
2. Call the `/register` API endpoint to register a test domain:

   ```bash
   $ curl -X POST https://auth.example.org/register
   {"username":"eabcdb41-d89f-4580-826f-3e62e9755ef2","password":"pbAXVjlIOE01xbut7YnAbkhMQIkcwoHO0ek2j4Q0","fulldomain":"d420c923-bbd7-4056-ab64-c3ca54c9b3cf.auth.example.org","subdomain":"d420c923-bbd7-4056-ab64-c3ca54c9b3cf","allowfrom":[]}
   ```

3. Call the `/update` API endpoint to set a test TXT record. Pass the `username`, `password` and `subdomain` received from the `register` call performed above:

   ```bash
   $ curl -X POST \
   -H "X-Api-User: eabcdb41-d89f-4580-826f-3e62e9755ef2" \
   -H "X-Api-Key: pbAXVjlIOE01xbut7YnAbkhMQIkcwoHO0ek2j4Q0" \
   -d '{"subdomain": "d420c923-bbd7-4056-ab64-c3ca54c9b3cf", "txt": "___validation_token_received_from_the_ca___"}' \
   https://auth.example.org/update
   ```

   Note: The `txt` field must be exactly 43 characters long, otherwise acme-dns will reject it

4. Perform a DNS lookup to the test subdomain to confirm the updated TXT record is being served:

   ```bash
   $ dig -t txt @auth.example.org d420c923-bbd7-4056-ab64-c3ca54c9b3cf.auth.example.org
   ```

## Configuration

```toml
[general]
# DNS interface. Note that systemd-resolved may reserve port 53 on 127.0.0.53
# In this case acme-dns will error out and you will need to define the listening interface
# for example: listen = "127.0.0.1:53"
listen = "127.0.0.1:53"
# protocol, "both", "both4", "both6", "udp", "udp4", "udp6" or "tcp", "tcp4", "tcp6"
protocol = "both"
# domain name to serve the requests off of
domain = "auth.example.org"
# zone name server
nsname = "auth.example.org"
# admin email address, where @ is substituted with .
nsadmin = "admin.example.org"
# predefined records served in addition to the TXT
records = [
    # domain pointing to the public IP of your acme-dns server
    "auth.example.org. A 198.51.100.1",
    # specify that auth.example.org will resolve any *.auth.example.org records
    "auth.example.org. NS auth.example.org.",
]
# debug messages from CORS etc
debug = false

[database]
# Database engine to use, sqlite3 or postgres
engine = "sqlite3"
# Connection string, filename for sqlite3 and postgres://$username:$password@$host/$db_name for postgres
# Please note that the default Docker image uses path /var/lib/acme-dns/acme-dns.db for sqlite3
connection = "/var/lib/acme-dns/acme-dns.db"
# connection = "postgres://user:password@localhost/acmedns_db"

[api]
# listen ip eg. 127.0.0.1
ip = "0.0.0.0"
# disable registration endpoint
disable_registration = false
# listen port, eg. 443 for default HTTPS
port = "443"
# possible values: "letsencrypt", "letsencryptstaging", "cert", "none"
tls = "letsencryptstaging"
# only used if tls = "cert"
tls_cert_privkey = "/etc/tls/example.org/privkey.pem"
tls_cert_fullchain = "/etc/tls/example.org/fullchain.pem"
# only used if tls = "letsencrypt"
acme_cache_dir = "api-certs"
# optional e-mail address to which Let's Encrypt will send expiration notices for the API's cert
notification_email = ""
# CORS AllowOrigins, wildcards can be used
# NOTE: v1.0.0+ defaults to [] (deny all). Set to ["*"] to allow all origins.
corsorigins = []
# use HTTP header to get the client ip
use_header = false
# header name to pull the ip address / list of ip addresses from
header_name = "X-Forwarded-For"
# max registrations per minute per source IP on the /register endpoint (0 = unlimited)
register_ratelimit = 10
# Admin API — leave token empty to disable admin endpoints
# token = "your-secret-admin-token-here"
token = ""

[logconfig]
# logging level: "error", "warning", "info" or "debug"
loglevel = "debug"
# possible values: stdout, TODO file & integrations
logtype = "stdout"
# file path for logfile TODO
# logfile = "./acme-dns.log"
# format, either "json" or "text"
logformat = "text"
```

## High Availability Deployment

Multiple acme-dns instances run simultaneously behind a load balancer. All instances share one PostgreSQL database. The HTTP API is stateless — no session affinity is required. DNS can be distributed across instances using round-robin NS records.

### Prerequisites

- PostgreSQL 14+ (primary/replica HA, e.g. PostgreSQL cluster, RDS Multi-AZ, Cloud SQL HA)
- A load balancer for HTTP: Nginx, Apache2, or cloud ALB (AWS ELB, GCP HTTPS LB)
- At least 2 acme-dns instances

### HA Database Configuration

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

### HA Instance Configuration

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

### DNS Load Balancing

Configure multiple A records for the acme-dns NS hostname with a low TTL (60s) for fast failover:

```
auth.example.org.  60  IN  A  203.0.113.1   ; instance 1
auth.example.org.  60  IN  A  203.0.113.2   ; instance 2
```

Resolvers will round-robin between instances for DNS queries.

### HTTP API Load Balancing

#### Nginx Example

```nginx
upstream acmedns_instances {
    server 10.0.0.1:443;
    server 10.0.0.2:443;
}

server {
    listen 443 ssl;
    ssl_certificate     /etc/ssl/acmedns.pem;
    ssl_certificate_key /etc/ssl/acmedns.key;

    location /health {
        proxy_pass https://acmedns_instances/health;
        access_log off;
    }

    location / {
        proxy_pass https://acmedns_instances;
        proxy_ssl_verify off;
    }
}
```

#### Apache2 Example

```apache
<VirtualHost *:443>
    SSLEngine on
    SSLCertificateFile /etc/ssl/acmedns.pem
    SSLCertificateKeyFile /etc/ssl/acmedns.key

    <Proxy "balancer://acmedns-instances">
        BalancerMember "https://10.0.0.1:443"
        BalancerMember "https://10.0.0.2:443"
        ProxySet lbmethod=byrequests
    </Proxy>

    ProxyPreserveHost On
    ProxyPass "/" "balancer://acmedns-instances/"
    ProxyPassReverse "/" "balancer://acmedns-instances/"
</VirtualHost>
```

### HA Health Check

The `/health` endpoint returns HTTP 200 when the instance is ready. Use it for:
- Load balancer health probes (see Nginx/Apache2 config above)
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

### Failure Modes

| Failure | Impact | Recovery |
|---------|--------|----------|
| One instance dies | DNS queries and HTTP API handled by remaining instances | Automatic — load balancer stops routing to failed instance |
| PostgreSQL unreachable | HTTP API returns 500; DNS continues serving cached records from resolver caches | Restore PostgreSQL; instances reconnect automatically on next request |
| PostgreSQL replica failover | Transient errors during failover window | Instances retry automatically on next request |

### Upgrade Procedure (Rolling Restart)

1. Start one instance first; migrations are idempotent and run automatically
2. Stop and restart instances one at a time, verifying `/health` before moving to the next
3. DNS resolvers cache responses — TTL (default 60s for NS records) means no visible downtime

### Security Notes in HA

- Rate limiting (`register_ratelimit`) is per-instance, per-IP (in-memory). In active-active, a single IP can register up to `register_ratelimit * N` times per minute across N instances. For stricter enforcement, place rate limiting at the load balancer (nginx `limit_req`).
- All instances share the same `[api.admin].token` — rotate it by updating all instances' configs and restarting them.

## HTTPS API

The RESTful acme-dns API can be exposed over HTTPS in two ways:

1. Using `tls = "letsencrypt"` and letting acme-dns issue its own certificate
   automatically with Let's Encrypt.
2. Using `tls = "cert"` and providing your own HTTPS certificate chain and
   private key with `tls_cert_fullchain` and `tls_cert_privkey`.

Where possible, the first option is recommended. This is the easiest and safest
way to have acme-dns expose its API over HTTPS.

**Warning**: If you choose to use `tls = "cert"` you must take care that the
certificate _does not expire_! If it does and the ACME client you use to issue the
certificate depends on the ACME DNS API to update TXT records, you will be stuck
in a position where the API certificate has expired, but it can't be renewed
because the ACME client will refuse to connect to the ACME DNS API it needs to
use for the renewal.

## Clients

- acme.sh: [https://github.com/Neilpang/acme.sh](https://github.com/Neilpang/acme.sh)
- Certify The Web: [https://github.com/webprofusion/certify](https://github.com/webprofusion/certify)
- cert-manager: [https://github.com/jetstack/cert-manager](https://github.com/jetstack/cert-manager)
- Lego: [https://github.com/xenolf/lego](https://github.com/xenolf/lego)
- Posh-ACME: [https://github.com/rmbolger/Posh-ACME](https://github.com/rmbolger/Posh-ACME)
- Sewer: [https://github.com/komuw/sewer](https://github.com/komuw/sewer)
- Traefik: [https://github.com/containous/traefik](https://github.com/containous/traefik)
- Windows ACME Simple (WACS): [https://www.win-acme.com](https://www.win-acme.com)

### Authentication hooks

- acme-dns-client with Certbot authentication hook: [https://github.com/acme-dns/acme-dns-client](https://github.com/acme-dns/acme-dns-client)
- Certbot authentication hook in Python: [https://github.com/joohoi/acme-dns-certbot-joohoi](https://github.com/joohoi/acme-dns-certbot-joohoi)
- Certbot authentication hook in Go: [https://github.com/koesie10/acme-dns-certbot-hook](https://github.com/koesie10/acme-dns-certbot-hook)

### Libraries

- Generic client library in Python ([PyPI](https://pypi.python.org/pypi/pyacmedns/)): [https://github.com/joohoi/pyacmedns](https://github.com/joohoi/pyacmedns)
- Generic client library in Go: [https://github.com/cpu/goacmedns](https://github.com/cpu/goacmedns)

## [Changelog](CHANGELOG.md)

## TODO

- Logging to a file
- DNSSEC
- Want to see something implemented, make a feature request!

## Contributing

acme-dns is open for contributions.
If you have an idea for improvement, please open a new issue or feel free to write a PR!

## License

acme-dns is released under the [MIT License](http://www.opensource.org/licenses/MIT).

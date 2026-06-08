# DNS API Extended Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a privileged admin HTTP API for full DNS record management (A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, PTR) on top of the existing acme-dns config-file architecture.

**Architecture:** A new `[api.admin]` section in `config.cfg` holds a single bearer token. All admin endpoints live under `/admin/records` and are protected by a constant-time Bearer middleware. A new `dns_records` DB table stores managed records; `dns.go`'s `answer()` falls through to it after checking ACME TXT records.

**Tech Stack:** Go 1.26, `github.com/julienschmidt/httprouter`, `github.com/google/uuid`, `database/sql` (SQLite + PostgreSQL via existing dual-query pattern), `github.com/miekg/dns`, `crypto/subtle`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `types.go` | Modify | Add `AdminConfig` struct + `Admin AdminConfig` field to `httpapi` |
| `config.cfg` | Modify | Add `[api.admin]` section |
| `db.go` | Modify | Add `dns_records` table DDL + 4 CRUD methods + extend `database` interface |
| `api.go` | Modify | Add 4 admin handler functions + `GET /openapi.json` stub handler |
| `main.go` | Modify | Register `/admin` route group with Bearer middleware; skip if token empty; add `PUT` + `DELETE` to CORS allowed methods |
| `dns.go` | Modify | Extend `answer()` to fall through to `dns_records` lookup |
| `validation.go` | Modify | Add `validRecordType`, `validRecordValue`, `validTTL` |
| `validation_test.go` | Modify | Tests for new validators |
| `api_test.go` | Modify | Tests for all 4 admin endpoints |
| `dns_test.go` | Modify | Test that managed records are served by DNS |
| `db_test.go` | Modify | Tests for new DB methods |

---

## Task 1: Add `AdminConfig` to types and config

**Files:**
- Modify: `types.go:41-55`
- Modify: `config.cfg`

- [ ] **Step 1: Add `AdminConfig` struct and wire into `httpapi`**

In `types.go`, add after the `httpapi` struct's closing brace (currently line 55):

```go
// Admin API config
type adminconfig struct {
	Token string
}
```

And add `Admin adminconfig` as the last field of `httpapi`:

```go
type httpapi struct {
	Domain              string `toml:"api_domain"`
	IP                  string
	DisableRegistration bool   `toml:"disable_registration"`
	AutocertPort        string `toml:"autocert_port"`
	Port                string `toml:"port"`
	TLS                 string
	TLSCertPrivkey      string `toml:"tls_cert_privkey"`
	TLSCertFullchain    string `toml:"tls_cert_fullchain"`
	ACMECacheDir        string `toml:"acme_cache_dir"`
	NotificationEmail   string `toml:"notification_email"`
	CorsOrigins         []string
	UseHeader           bool   `toml:"use_header"`
	HeaderName          string `toml:"header_name"`
	Admin               adminconfig
}
```

- [ ] **Step 2: Add `[api.admin]` section to `config.cfg`**

Append after the `[api]` section in `config.cfg`:

```toml
# Admin API — leave token empty to disable admin endpoints
[api.admin]
# token = "your-secret-admin-token-here"
token = ""
```

- [ ] **Step 3: Run tests to verify nothing broke**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add types.go config.cfg
git commit -m "feat: add AdminConfig struct and [api.admin] config section"
```

---

## Task 2: Add record validators

**Files:**
- Modify: `validation.go`
- Modify: `validation_test.go`

- [ ] **Step 1: Write failing tests for `validRecordType`**

Add to `validation_test.go`:

```go
func TestValidRecordType(t *testing.T) {
	valid := []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "PTR"}
	for _, rt := range valid {
		if !validRecordType(rt) {
			t.Errorf("expected %s to be valid", rt)
		}
	}
	invalid := []string{"SOA", "AXFR", "ANY", "", "a", "aaaa"}
	for _, rt := range invalid {
		if validRecordType(rt) {
			t.Errorf("expected %s to be invalid", rt)
		}
	}
}

func TestValidTTL(t *testing.T) {
	valid := []int{1, 60, 300, 3600, 86400}
	for _, ttl := range valid {
		if !validTTL(ttl) {
			t.Errorf("expected TTL %d to be valid", ttl)
		}
	}
	invalid := []int{0, -1, 86401, 999999}
	for _, ttl := range invalid {
		if validTTL(ttl) {
			t.Errorf("expected TTL %d to be invalid", ttl)
		}
	}
}

func TestValidRecordValue(t *testing.T) {
	cases := []struct {
		rtype string
		value string
		valid bool
	}{
		{"A", "1.2.3.4", true},
		{"A", "256.1.1.1", false},
		{"A", "not-an-ip", false},
		{"AAAA", "2001:db8::1", true},
		{"AAAA", "1.2.3.4", false},
		{"CNAME", "example.com", true},
		{"CNAME", "", false},
		{"MX", "mail.example.com", true},
		{"MX", "", false},
		{"NS", "ns1.example.com", true},
		{"TXT", "any text value here", true},
		{"TXT", "", false},
		{"SRV", "_sip._tcp.example.com", true},
		{"CAA", "0 issue \"letsencrypt.org\"", true},
		{"PTR", "host.example.com", true},
	}
	for _, tc := range cases {
		got := validRecordValue(tc.rtype, tc.value)
		if got != tc.valid {
			t.Errorf("validRecordValue(%q, %q) = %v, want %v", tc.rtype, tc.value, got, tc.valid)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestValidRecordType|TestValidTTL|TestValidRecordValue" 2>&1
```
Expected: compile error — functions not defined yet.

- [ ] **Step 3: Implement validators in `validation.go`**

Add to `validation.go`:

```go
func validRecordType(t string) bool {
	switch t {
	case "A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "PTR":
		return true
	}
	return false
}

func validTTL(ttl int) bool {
	return ttl >= 1 && ttl <= 86400
}

func validRecordValue(rtype, value string) bool {
	if value == "" {
		return false
	}
	switch rtype {
	case "A":
		ip := net.ParseIP(value)
		return ip != nil && ip.To4() != nil
	case "AAAA":
		ip := net.ParseIP(value)
		return ip != nil && ip.To4() == nil
	case "CNAME", "MX", "NS", "PTR", "SRV":
		return len(value) > 0
	case "TXT", "CAA":
		return len(value) > 0
	}
	return false
}
```

Add `"net"` to the imports in `validation.go`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestValidRecordType|TestValidTTL|TestValidRecordValue" 2>&1
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add validation.go validation_test.go
git commit -m "feat: add DNS record type/value/TTL validators"
```

---

## Task 3: Add `dns_records` table and CRUD methods to DB

**Files:**
- Modify: `db.go`

- [ ] **Step 1: Write failing DB tests**

The test suite uses a global `DB` set up in `TestMain` in `main_test.go` (in-memory SQLite). Add directly to `db_test.go` using `DB` directly:

```go
func TestCreateAndListRecord(t *testing.T) {
	rec := DNSRecord{
		ID:      "test-uuid-1",
		Name:    "sub.example.com",
		Type:    "A",
		Value:   "1.2.3.4",
		TTL:     300,
		Created: 0,
	}
	err := DB.CreateRecord(rec)
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	t.Cleanup(func() { _ = DB.DeleteRecord("test-uuid-1") })

	records, err := DB.ListRecords("", "")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	found := false
	for _, r := range records {
		if r.ID == "test-uuid-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected record test-uuid-1 in list, got %v", records)
	}
}

func TestUpdateRecord(t *testing.T) {
	rec := DNSRecord{ID: "upd-1", Name: "a.example.com", Type: "A", Value: "1.1.1.1", TTL: 60, Created: 0}
	_ = DB.CreateRecord(rec)
	t.Cleanup(func() { _ = DB.DeleteRecord("upd-1") })

	rec.Value = "2.2.2.2"
	err := DB.UpdateRecord(rec)
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	records, _ := DB.ListRecords("A", "a.example.com")
	if len(records) == 0 || records[0].Value != "2.2.2.2" {
		t.Fatalf("expected updated value 2.2.2.2, got %v", records)
	}
}

func TestDeleteRecord(t *testing.T) {
	rec := DNSRecord{ID: "del-1", Name: "b.example.com", Type: "A", Value: "3.3.3.3", TTL: 60, Created: 0}
	_ = DB.CreateRecord(rec)

	err := DB.DeleteRecord("del-1")
	if err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	records, _ := DB.ListRecords("A", "b.example.com")
	if len(records) != 0 {
		t.Fatalf("expected 0 records after delete, got %d", len(records))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestCreateAndListRecord|TestUpdateRecord|TestDeleteRecord" 2>&1
```
Expected: compile error — `DNSRecord` and methods not defined.

- [ ] **Step 3: Add `DNSRecord` struct, table DDL, and CRUD methods to `db.go`**

After the `txtTablePG` variable (line 49), add:

```go
var dnsRecordsTable = `
	CREATE TABLE IF NOT EXISTS dns_records (
		id      TEXT PRIMARY KEY,
		name    TEXT NOT NULL,
		type    TEXT NOT NULL,
		value   TEXT NOT NULL,
		ttl     INTEGER NOT NULL DEFAULT 300,
		created INTEGER NOT NULL
	);`

// DNSRecord represents a managed DNS record
type DNSRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	TTL     int    `json:"ttl"`
	Created int64  `json:"created"`
}
```

In `Init()`, after the existing `txt` table creation block (after line 82), add:

```go
_, _ = d.DB.Exec(dnsRecordsTable)
```

Add these methods after the existing `Update()` method:

```go
func (d *acmedb) CreateRecord(rec DNSRecord) error {
	d.Lock()
	defer d.Unlock()
	sql := `INSERT INTO dns_records (id, name, type, value, ttl, created) VALUES ($1, $2, $3, $4, $5, $6)`
	if Config.Database.Engine == "sqlite3" {
		sql = getSQLiteStmt(sql)
	}
	_, err := d.DB.Exec(sql, rec.ID, rec.Name, rec.Type, rec.Value, rec.TTL, rec.Created)
	return err
}

func (d *acmedb) ListRecords(filterType, filterName string) ([]DNSRecord, error) {
	d.Lock()
	defer d.Unlock()
	q := `SELECT id, name, type, value, ttl, created FROM dns_records WHERE ($1 = '' OR type = $1) AND ($2 = '' OR name = $2)`
	if Config.Database.Engine == "sqlite3" {
		q = getSQLiteStmt(q)
	}
	rows, err := d.DB.Query(q, filterType, filterName)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var records []DNSRecord
	for rows.Next() {
		var r DNSRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Value, &r.TTL, &r.Created); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	if records == nil {
		records = []DNSRecord{}
	}
	return records, nil
}

func (d *acmedb) UpdateRecord(rec DNSRecord) error {
	d.Lock()
	defer d.Unlock()
	sql := `UPDATE dns_records SET name=$1, type=$2, value=$3, ttl=$4 WHERE id=$5`
	if Config.Database.Engine == "sqlite3" {
		sql = getSQLiteStmt(sql)
	}
	_, err := d.DB.Exec(sql, rec.Name, rec.Type, rec.Value, rec.TTL, rec.ID)
	return err
}

func (d *acmedb) DeleteRecord(id string) error {
	d.Lock()
	defer d.Unlock()
	sql := `DELETE FROM dns_records WHERE id=$1`
	if Config.Database.Engine == "sqlite3" {
		sql = getSQLiteStmt(sql)
	}
	_, err := d.DB.Exec(sql, id)
	return err
}
```

Add these methods to the `database` interface in `types.go`:

```go
CreateRecord(DNSRecord) error
ListRecords(filterType, filterName string) ([]DNSRecord, error)
UpdateRecord(DNSRecord) error
DeleteRecord(id string) error
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestCreateAndListRecord|TestUpdateRecord|TestDeleteRecord" 2>&1
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add db.go types.go db_test.go
git commit -m "feat: add dns_records table and CRUD methods to DB"
```

---

## Task 4: Add admin HTTP handlers

**Files:**
- Modify: `api.go`

- [ ] **Step 1: Write failing HTTP tests for admin endpoints**

The existing test pattern (see `api_test.go`) uses `setupRouter()` to build an `http.Handler`, wraps it in `httptest.NewServer`, and uses `getExpect()` for assertions. Admin tests follow the same pattern but register the admin routes too.

Add a new router helper and tests to `api_test.go`:

```go
func setupAdminRouter(token string) http.Handler {
	api := httprouter.New()
	var dbcfg = dbsettings{Engine: "sqlite3", Connection: ":memory:"}
	var httpapicfg = httpapi{
		Port:        "8080",
		TLS:         "none",
		CorsOrigins: []string{"*"},
		UseHeader:   false,
		HeaderName:  "X-Forwarded-For",
		Admin:       adminconfig{Token: token},
	}
	Config = DNSConfig{API: httpapicfg, Database: dbcfg}
	newDB := new(acmedb)
	_ = newDB.Init(Config.Database.Engine, Config.Database.Connection)
	DB = newDB

	api.POST("/register", webRegisterPost)
	api.GET("/health", healthCheck)
	api.POST("/update", Auth(webUpdatePost))
	if token != "" {
		api.GET("/admin/records", adminBearerMiddleware(adminListRecords))
		api.POST("/admin/records", adminBearerMiddleware(adminCreateRecord))
		api.PUT("/admin/records/:id", adminBearerMiddleware(adminUpdateRecord))
		api.DELETE("/admin/records/:id", adminBearerMiddleware(adminDeleteRecord))
	}
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
	})
	return c.Handler(api)
}

func TestAdminListRecordsUnauthorized(t *testing.T) {
	router := setupAdminRouter("test-token")
	server := httptest.NewServer(router)
	defer server.Close()
	e := getExpect(t, server)
	e.GET("/admin/records").Expect().Status(http.StatusUnauthorized)
}

func TestAdminCreateRecord(t *testing.T) {
	router := setupAdminRouter("test-token")
	server := httptest.NewServer(router)
	defer server.Close()
	e := getExpect(t, server)

	body := map[string]interface{}{"name": "test.example.com", "type": "A", "value": "1.2.3.4", "ttl": 300}
	e.POST("/admin/records").
		WithHeader("Authorization", "Bearer test-token").
		WithJSON(body).
		Expect().
		Status(http.StatusCreated).
		JSON().Object().ContainsKey("id")
}

func TestAdminCreateRecordInvalidType(t *testing.T) {
	router := setupAdminRouter("test-token")
	server := httptest.NewServer(router)
	defer server.Close()
	e := getExpect(t, server)

	body := map[string]interface{}{"name": "test.example.com", "type": "INVALID", "value": "1.2.3.4", "ttl": 300}
	e.POST("/admin/records").
		WithHeader("Authorization", "Bearer test-token").
		WithJSON(body).
		Expect().
		Status(http.StatusBadRequest)
}

func TestAdminListRecords(t *testing.T) {
	router := setupAdminRouter("test-token")
	server := httptest.NewServer(router)
	defer server.Close()
	e := getExpect(t, server)

	e.GET("/admin/records").
		WithHeader("Authorization", "Bearer test-token").
		Expect().
		Status(http.StatusOK).
		JSON().Array()
}

func TestAdminDeleteRecord(t *testing.T) {
	router := setupAdminRouter("test-token")
	server := httptest.NewServer(router)
	defer server.Close()
	e := getExpect(t, server)

	body := map[string]interface{}{"name": "del.example.com", "type": "A", "value": "1.2.3.4", "ttl": 60}
	id := e.POST("/admin/records").
		WithHeader("Authorization", "Bearer test-token").
		WithJSON(body).
		Expect().Status(http.StatusCreated).
		JSON().Object().Value("id").String().Raw()

	e.DELETE("/admin/records/"+id).
		WithHeader("Authorization", "Bearer test-token").
		Expect().Status(http.StatusNoContent)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestAdmin" 2>&1
```
Expected: compile error — handlers not defined yet.

- [ ] **Step 3: Implement admin handlers in `api.go`**

Add to `api.go` after `healthCheck`:

```go
// adminRecordRequest is the request body for creating/updating a DNS record
type adminRecordRequest struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

func adminBearerMiddleware(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		token := Config.API.Admin.Token
		auth := r.Header.Get("Authorization")
		provided := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(jsonError("unauthorized"))
			return
		}
		next(w, r, ps)
	}
}

func adminListRecords(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	filterType := r.URL.Query().Get("type")
	filterName := r.URL.Query().Get("name")
	records, err := DB.ListRecords(filterType, filterName)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(records)
}

func adminCreateRecord(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var req adminRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("malformed_json_payload"))
		return
	}
	if !validRecordType(req.Type) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_record_type"))
		return
	}
	if !validRecordValue(req.Type, req.Value) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_record_value"))
		return
	}
	ttl := req.TTL
	if ttl == 0 {
		ttl = 300
	}
	if !validTTL(ttl) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_ttl"))
		return
	}
	rec := DNSRecord{
		ID:      uuid.New().String(),
		Name:    req.Name,
		Type:    req.Type,
		Value:   req.Value,
		TTL:     ttl,
		Created: time.Now().Unix(),
	}
	if err := DB.CreateRecord(rec); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(rec)
}

func adminUpdateRecord(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("id")
	var req adminRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("malformed_json_payload"))
		return
	}
	if !validRecordType(req.Type) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_record_type"))
		return
	}
	if !validRecordValue(req.Type, req.Value) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_record_value"))
		return
	}
	if req.TTL != 0 && !validTTL(req.TTL) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(jsonError("invalid_ttl"))
		return
	}
	ttl := req.TTL
	if ttl == 0 {
		ttl = 300
	}
	rec := DNSRecord{ID: id, Name: req.Name, Type: req.Type, Value: req.Value, TTL: ttl}
	if err := DB.UpdateRecord(rec); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rec)
}

func adminDeleteRecord(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("id")
	if err := DB.DeleteRecord(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(jsonError("db_error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add `"crypto/subtle"`, `"strings"`, `"time"` to imports in `api.go` (alongside existing ones).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestAdmin" 2>&1
```
Expected: all admin tests PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add api.go api_test.go
git commit -m "feat: add admin HTTP handlers for DNS record CRUD"
```

---

## Task 5: Register admin routes in `main.go`

**Files:**
- Modify: `main.go:117-133`

- [ ] **Step 1: Register `/admin` routes and extend CORS**

In `startHTTPAPI()`, replace the existing route setup block (lines 117-133) with:

```go
api := httprouter.New()
c := cors.New(cors.Options{
	AllowedOrigins:     config.API.CorsOrigins,
	AllowedMethods:     []string{"GET", "POST", "PUT", "DELETE"},
	AllowedHeaders:     []string{"Authorization", "Content-Type", "X-Api-User", "X-Api-Key"},
	OptionsPassthrough: false,
	Debug:              config.General.Debug,
})
if config.General.Debug {
	c.Log = stdlog.New(logWriter, "", 0)
}
if !config.API.DisableRegistration {
	api.POST("/register", webRegisterPost)
}
api.POST("/update", Auth(webUpdatePost))
api.GET("/health", healthCheck)

// Admin API — only register routes if token is configured
if config.API.Admin.Token != "" {
	api.GET("/admin/records", adminBearerMiddleware(adminListRecords))
	api.POST("/admin/records", adminBearerMiddleware(adminCreateRecord))
	api.PUT("/admin/records/:id", adminBearerMiddleware(adminUpdateRecord))
	api.DELETE("/admin/records/:id", adminBearerMiddleware(adminDeleteRecord))
}
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: register admin routes with Bearer middleware in HTTP API"
```

---

## Task 6: Extend DNS server to serve managed records

**Files:**
- Modify: `dns.go:193-218`
- Modify: `dns_test.go`

- [ ] **Step 1: Write failing DNS test for managed record**

The test suite uses a global `dnsserver` (type `*DNSServer`) and global `DB` set up in `main_test.go`. Add a test to `dns_test.go` that pre-seeds a record and then calls `dnsserver.answer()`:

```go
func TestAnswerManagedARecord(t *testing.T) {
	rec := DNSRecord{
		ID: "managed-test-1", Name: "managed.auth.example.org.", Type: "A", Value: "5.6.7.8", TTL: 300, Created: 0,
	}
	err := DB.CreateRecord(rec)
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	t.Cleanup(func() { _ = DB.DeleteRecord("managed-test-1") })

	q := dns.Question{Name: "managed.auth.example.org.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	rrs, rcode, _, err := dnsserver.answer(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR, got %s", dns.RcodeToString[rcode])
	}
	if len(rrs) == 0 {
		t.Fatal("expected at least one RR in answer")
	}
}
```

Note: the `database` interface in `types.go` must include the 4 new methods before this will compile — that happens in Task 3.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./... -run "TestAnswerManagedARecord" 2>&1
```
Expected: FAIL — managed record not served yet.

- [ ] **Step 3: Extend `answer()` in `dns.go`**

In `answer()` (line 193), after the existing `if len(r) > 0` check (line 213), add a fallthrough to `dns_records`:

```go
func (d *DNSServer) answer(q dns.Question) ([]dns.RR, int, bool, error) {
	var rcode int
	var err error
	var txtRRs []dns.RR
	var authoritative = d.isAuthoritative(q)
	if !d.isOwnChallenge(q.Name) && !d.answeringForDomain(q.Name) {
		rcode = dns.RcodeNameError
	}
	r, _ := d.getRecord(q)
	if q.Qtype == dns.TypeTXT {
		if d.isOwnChallenge(q.Name) {
			txtRRs, err = d.answerOwnChallenge(q)
		} else {
			txtRRs, err = d.answerTXT(q)
		}
		if err == nil {
			r = append(r, txtRRs...)
		}
	}
	// Fall through to managed dns_records if no static or ACME records matched
	if len(r) == 0 {
		r = d.answerManaged(q)
	}
	if len(r) > 0 {
		rcode = dns.RcodeSuccess
	}
	log.WithFields(log.Fields{"qtype": dns.TypeToString[q.Qtype], "domain": q.Name, "rcode": dns.RcodeToString[rcode]}).Debug("Answering question for domain")
	return r, rcode, authoritative, nil
}
```

Add the `answerManaged` method after `answerOwnChallenge`:

```go
func (d *DNSServer) answerManaged(q dns.Question) []dns.RR {
	name := strings.ToLower(q.Name)
	records, err := d.DB.ListRecords(dns.TypeToString[q.Qtype], name)
	if err != nil {
		log.WithFields(log.Fields{"error": err.Error()}).Debug("Error querying managed records")
		return nil
	}
	var rrs []dns.RR
	for _, rec := range records {
		rrStr := fmt.Sprintf("%s %d IN %s %s", rec.Name, rec.TTL, rec.Type, rec.Value)
		rr, err := dns.NewRR(rrStr)
		if err != nil {
			log.WithFields(log.Fields{"error": err.Error(), "record": rrStr}).Warning("Could not parse managed RR")
			continue
		}
		rrs = append(rrs, rr)
	}
	return rrs
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./... -run "TestAnswerManagedARecord" 2>&1
```
Expected: PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add dns.go dns_test.go
git commit -m "feat: extend DNS server to serve managed records from dns_records table"
```

---

## Task 7: Final integration check and branch summary

- [ ] **Step 1: Build the binary**

```bash
go build ./... 2>&1
```
Expected: builds with no errors.

- [ ] **Step 2: Run full test suite one last time**

```bash
go test ./... -v 2>&1 | tail -30
```
Expected: all tests pass, no failures.

- [ ] **Step 3: Verify config parses correctly**

```bash
go vet ./... 2>&1
```
Expected: no issues.

- [ ] **Step 4: Final commit**

```bash
git add -A
git status
```
If there are any unstaged changes (e.g. `config.cfg`), commit them:
```bash
git commit -m "chore: finalize dns-api-extended feature"
```

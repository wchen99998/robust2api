package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Checker provides health probe endpoints.
type Checker struct {
	db    *sql.DB
	rdb   *redis.Client
	ready atomic.Bool
}

// NewChecker creates a Checker with DB and Redis clients.
func NewChecker(db *sql.DB, rdb *redis.Client) *Checker {
	return &Checker{
		db:  db,
		rdb: rdb,
	}
}

// SetReady marks the service as ready to receive traffic.
func (c *Checker) SetReady() {
	c.ready.Store(true)
}

// Livez always returns 200 — process is alive.
func (c *Checker) Livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Readyz returns 200 if DB and Redis are reachable, 503 otherwise.
func (c *Checker) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	type checkResult struct {
		name string
		ok   bool
	}

	results := make([]checkResult, 0, 2)

	// Check PostgreSQL
	if c.db != nil {
		err := c.db.PingContext(ctx)
		results = append(results, checkResult{"postgresql", err == nil})
	}

	// Check Redis
	if c.rdb != nil {
		err := c.rdb.Ping(ctx).Err()
		results = append(results, checkResult{"redis", err == nil})
	}

	allOK := true
	checks := make(map[string]string, len(results))
	for _, r := range results {
		if r.ok {
			checks[r.name] = "ok"
		} else {
			checks[r.name] = "fail"
			allOK = false
		}
	}

	resp := map[string]any{
		"status": "ok",
		"checks": checks,
	}
	if !allOK {
		resp["status"] = "fail"
	}

	w.Header().Set("Content-Type", "application/json")
	if !allOK {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// Startupz returns 200 if SetReady() has been called, 503 otherwise.
func (c *Checker) Startupz(w http.ResponseWriter, _ *http.Request) {
	if c.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"starting"}`))
	}
}

// RegisterOnMux registers all probe endpoints on a standard http.ServeMux.
// Used by the worker's internal health server.
func (c *Checker) RegisterOnMux(mux *http.ServeMux) {
	mux.HandleFunc("/livez", c.Livez)
	mux.HandleFunc("/readyz", c.Readyz)
	mux.HandleFunc("/startupz", c.Startupz)
	mux.HandleFunc("/health", c.Readyz) // transitional alias
}

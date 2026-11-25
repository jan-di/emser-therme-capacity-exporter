package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func serve(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestScrape(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    float64
		wantErr bool
	}{
		{"valid", 200, `<div class="capacity--area"><h3> 42% </h3></div>`, 42, false},
		{"nested in mask", 200, `<div class="capacity--area text-center"><div class="capacity--area_mask"><p>Auslastung</p><h3 class="bold mb-0">2%</h3></div></div>`, 2, false},
		{"without percent sign", 200, `<div class="capacity--area"><h3>7</h3></div>`, 7, false},
		{"missing element", 200, `<div><h3>42%</h3></div>`, 0, true},
		{"not a number", 200, `<div class="capacity--area"><h3>n/a</h3></div>`, 0, true},
		{"http error", 500, `oops`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newExporter(prometheus.NewRegistry(), serve(t, tt.status, tt.body))
			err := e.scrape(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			wantSuccess := 1.0
			if tt.wantErr {
				wantSuccess = 0
			}
			if got := testutil.ToFloat64(e.scrapeSuccess); got != wantSuccess {
				t.Errorf("scrape_success = %v, want %v", got, wantSuccess)
			}
			if tt.wantErr {
				if n := testutil.CollectAndCount(e.capacity); n != 0 {
					t.Errorf("capacity series = %d, want none after failed scrape", n)
				}
				return
			}
			if got := testutil.ToFloat64(e.capacity); got != tt.want {
				t.Errorf("capacity = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCapacityRemovedAfterFailure(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<div class="capacity--area"><h3>30%</h3></div>`))
	}))
	defer srv.Close()

	e := newExporter(prometheus.NewRegistry(), srv.URL)
	if err := e.scrape(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(e.capacity); got != 30 {
		t.Fatalf("capacity = %v, want 30", got)
	}

	status = http.StatusInternalServerError
	if err := e.scrape(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if n := testutil.CollectAndCount(e.capacity); n != 0 {
		t.Errorf("stale capacity series still exported (%d)", n)
	}
}

func TestRequireBearerToken(t *testing.T) {
	h := requireBearerToken("s3cret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer s3cret", 200},
		{"missing", "", 401},
		{"wrong token", "Bearer nope", 401},
		{"wrong scheme", "Basic s3cret", 401},
		{"no scheme", "s3cret", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestLoadToken(t *testing.T) {
	t.Setenv("EXPORTER_AUTH_TOKEN", "from-env")
	if got, _ := loadToken(""); got != "from-env" {
		t.Errorf("env token = %q", got)
	}
	f := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(f, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadToken(f); err != nil || got != "from-file" {
		t.Errorf("file token = %q, %v", got, err)
	}
	if _, err := loadToken(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected error for missing file")
	}
}

package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const capacitySelector = "div.capacity--area h3"

type exporter struct {
	url    string
	client *http.Client

	// capacity is a label-less GaugeVec so the series can be removed (Reset)
	// when a scrape fails, instead of exporting a misleading 0.
	capacity       *prometheus.GaugeVec
	scrapeSuccess  prometheus.Gauge
	scrapeDuration prometheus.Summary
}

func newExporter(reg prometheus.Registerer, url string) *exporter {
	e := &exporter{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		capacity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "emser_therme_capacity_percentage",
			Help: "Current percentage of capacity of Emser Therme. Absent if the last scrape failed.",
		}, nil),
		scrapeSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "emser_therme_scrape_success",
			Help: "1 if the last scrape of the Emser Therme website succeeded, 0 otherwise",
		}),
		scrapeDuration: prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "emser_therme_scrape_duration_seconds",
			Help: "Duration of scraping to Emser Therme website",
		}),
	}
	reg.MustRegister(e.capacity, e.scrapeSuccess, e.scrapeDuration)
	return e
}

// scrape fetches the capacity and updates all metrics accordingly.
func (e *exporter) scrape(ctx context.Context) error {
	value, err := e.fetchCapacity(ctx)
	if err != nil {
		e.capacity.Reset()
		e.scrapeSuccess.Set(0)
		return err
	}
	e.capacity.WithLabelValues().Set(float64(value))
	e.scrapeSuccess.Set(1)
	return nil
}

func (e *exporter) fetchCapacity(ctx context.Context) (int, error) {
	start := time.Now()
	defer func() { e.scrapeDuration.Observe(time.Since(start).Seconds()) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, err
	}
	sel := doc.Find(capacitySelector).First()
	if sel.Length() == 0 {
		return 0, fmt.Errorf("capacity element %q not found", capacitySelector)
	}
	text := strings.TrimSpace(strings.ReplaceAll(sel.Text(), "%", ""))
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("parse capacity %q: %w", text, err)
	}

	return value, nil
}

// requireBearerToken rejects requests that do not carry the expected
// "Authorization: Bearer <token>" header. The comparison is constant-time.
func requireBearerToken(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sum := sha256.Sum256([]byte(got))
		if !ok || subtle.ConstantTimeCompare(sum[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loadToken reads the auth token from a file (preferred) or the
// EXPORTER_AUTH_TOKEN environment variable. Empty means auth is disabled.
func loadToken(file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(b))
		if token == "" {
			return "", fmt.Errorf("auth token file %q is empty", file)
		}
		return token, nil
	}
	return os.Getenv("EXPORTER_AUTH_TOKEN"), nil
}

// healthcheck probes the local /healthz endpoint; used by the container
// HEALTHCHECK since the scratch image has no shell or curl.
func healthcheck(port int) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: %s", resp.Status)
	}
	return nil
}

func (e *exporter) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := e.scrape(ctx); err != nil && ctx.Err() == nil {
			slog.Error("scrape failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func main() {
	port := flag.Int("port", 8838, "Port to listen on")
	interval := flag.Duration("interval", 60*time.Second, "Scraping interval")
	url := flag.String("url", "https://www.emser-therme.de", "URL of the Emser Therme website")
	tokenFile := flag.String("auth-token-file", "", "File containing a bearer token required to read /metrics (alternatively set EXPORTER_AUTH_TOKEN)")
	check := flag.Bool("healthcheck", false, "Probe the local health endpoint and exit (for container HEALTHCHECK)")
	flag.Parse()

	if *check {
		if err := healthcheck(*port); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	token, err := loadToken(*tokenFile)
	if err != nil {
		slog.Error("loading auth token failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	exp := newExporter(reg, *url)

	mux := http.NewServeMux()
	var metrics http.Handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	if token != "" {
		metrics = requireBearerToken(token, metrics)
	} else {
		slog.Warn("no auth token configured, /metrics is unauthenticated")
	}
	mux.Handle("/metrics", metrics)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("starting Emser Therme Capacity Exporter", "port", *port, "auth", token != "")
	go exp.run(ctx, *interval)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

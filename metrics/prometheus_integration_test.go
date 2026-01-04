package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// TestPrometheusHTTPEndpoint tests the Prometheus metrics HTTP endpoint
func TestPrometheusHTTPEndpoint(t *testing.T) {
	// Create metrics instance
	m := &Metrics{
		StartTime:       time.Now().Add(-1 * time.Hour),
		BlocksProcessed: 100,
		BlocksAccepted:  95,
		LastBlockHeight: 1000,
		PeersConnected:  5,
	}

	// Create collector and registry
	collector := NewPrometheusCollector(m)
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Failed to register collector: %v", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := &http.Server{
		Addr:    "127.0.0.1:19999", // Use a high port to avoid conflicts
		Handler: mux,
	}

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("HTTP server error: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Make HTTP request
	resp, err := http.Get("http://127.0.0.1:19999/metrics")
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)

	// Verify expected metrics are present
	expectedMetrics := []string{
		"nmcd_blocks_processed_total 100",
		"nmcd_blocks_accepted_total 95",
		"nmcd_last_block_height 1000",
		"nmcd_peers_connected 5",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(bodyStr, expected) {
			t.Errorf("Expected metric %q not found in response", expected)
		}
	}

	// Verify metric types are present
	expectedTypes := []string{
		"# TYPE nmcd_blocks_processed_total counter",
		"# TYPE nmcd_last_block_height gauge",
		"# TYPE nmcd_uptime_seconds gauge",
	}

	for _, expectedType := range expectedTypes {
		if !strings.Contains(bodyStr, expectedType) {
			t.Errorf("Expected type declaration %q not found in response", expectedType)
		}
	}
}

// TestPrometheusMetricsFormat tests that the Prometheus output is in the correct format
func TestPrometheusMetricsFormat(t *testing.T) {
	m := &Metrics{
		StartTime:       time.Now(),
		BlocksProcessed: 42,
	}

	collector := NewPrometheusCollector(m)
	registry := prometheus.NewRegistry()
	registry.Register(collector)

	// Gather metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Verify we have some metrics
	if len(metricFamilies) == 0 {
		t.Fatal("No metrics were gathered")
	}

	// Verify each metric family has required fields
	for _, mf := range metricFamilies {
		if mf.Name == nil || *mf.Name == "" {
			t.Error("Metric family has no name")
		}
		if mf.Type == nil {
			t.Error("Metric family has no type")
		}
		if mf.Help == nil || *mf.Help == "" {
			t.Error("Metric family has no help text")
		}
		if len(mf.Metric) == 0 {
			t.Errorf("Metric family %s has no metrics", *mf.Name)
		}
	}
}

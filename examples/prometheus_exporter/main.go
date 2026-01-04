package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/nmcd/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// This example demonstrates how to use the Prometheus exporter to expose
// nmcd metrics for monitoring and alerting.
func main() {
	// Get the global metrics instance
	m := metrics.Get()

	// Simulate some activity by updating metrics
	fmt.Println("Simulating nmcd activity...")
	m.RecordBlockProcessed("hash1", 100, true, false, 50*time.Millisecond)
	m.RecordBlockProcessed("hash2", 101, true, false, 45*time.Millisecond)
	m.RecordNameOperation("NAME_NEW")
	m.RecordNameOperation("NAME_FIRSTUPDATE")
	m.RecordNameOperation("NAME_UPDATE")
	m.UpdatePeerCount(5, 2, 3)
	m.UpdateMempoolSize(10)

	// Create Prometheus collector
	collector := metrics.NewPrometheusCollector(m)

	// Create registry and register collector
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		log.Fatalf("Failed to register Prometheus collector: %v", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:    "127.0.0.1:9100",
		Handler: mux,
	}

	// Start server
	fmt.Printf("Starting Prometheus metrics server on http://%s/metrics\n", server.Addr)
	fmt.Println("Open http://127.0.0.1:9100/metrics in your browser to see the metrics")
	fmt.Println("Press Ctrl+C to stop")

	// For this example, we'll make a request to show the output
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Make a sample request to show the metrics
	resp, err := http.Get("http://127.0.0.1:9100/metrics")
	if err != nil {
		log.Fatalf("Failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	fmt.Println("\n--- Sample Prometheus Metrics Output ---")
	fmt.Println(string(body))
	fmt.Println("--- End of Metrics ---\n")

	// Keep server running for a few seconds
	fmt.Println("Server will remain running for 5 seconds...")
	time.Sleep(5 * time.Second)

	// Shutdown server
	if err := server.Close(); err != nil {
		log.Printf("Error closing server: %v", err)
	}

	fmt.Println("Server stopped")
}

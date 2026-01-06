package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestDatabaseMetrics tests database-related metrics
func TestDatabaseMetrics(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Test database read metrics
	m.RecordDatabaseRead(10 * time.Millisecond)
	m.RecordDatabaseRead(20 * time.Millisecond)

	snapshot := m.Snapshot()
	expectedReadLatency := 15 * time.Millisecond
	if snapshot.NameDBReadLatency != expectedReadLatency {
		t.Errorf("Expected read latency %v, got %v", expectedReadLatency, snapshot.NameDBReadLatency)
	}

	// Test database write metrics
	m.RecordDatabaseWrite(30 * time.Millisecond)
	m.RecordDatabaseWrite(50 * time.Millisecond)

	snapshot = m.Snapshot()
	expectedWriteLatency := 40 * time.Millisecond
	if snapshot.NameDBWriteLatency != expectedWriteLatency {
		t.Errorf("Expected write latency %v, got %v", expectedWriteLatency, snapshot.NameDBWriteLatency)
	}

	// Test database size update
	m.UpdateDatabaseSize(1024 * 1024) // 1MB
	snapshot = m.Snapshot()
	if snapshot.NameDBSizeBytes != 1024*1024 {
		t.Errorf("Expected database size 1048576, got %d", snapshot.NameDBSizeBytes)
	}
}

// TestRPCMetrics tests RPC per-method tracking
func TestRPCMetrics(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Record some RPC requests
	m.RecordRPCRequest("getinfo", 5*time.Millisecond)
	m.RecordRPCRequest("getinfo", 15*time.Millisecond)
	m.RecordRPCRequest("name_show", 20*time.Millisecond)
	m.RecordRPCRequest("getblockcount", 3*time.Millisecond)

	snapshot := m.Snapshot()

	// Check request counts
	if count := snapshot.RPCRequests["getinfo"]; count != 2 {
		t.Errorf("Expected 2 getinfo requests, got %d", count)
	}
	if count := snapshot.RPCRequests["name_show"]; count != 1 {
		t.Errorf("Expected 1 name_show request, got %d", count)
	}
	if count := snapshot.RPCRequests["getblockcount"]; count != 1 {
		t.Errorf("Expected 1 getblockcount request, got %d", count)
	}

	// Check average durations
	expectedGetInfoDuration := 10 * time.Millisecond // (5+15)/2
	if dur := snapshot.RPCDurations["getinfo"]; dur != 20*time.Millisecond {
		t.Errorf("Expected total getinfo duration %v, got %v", 20*time.Millisecond, dur)
	}

	// Test GetRPCStats helper
	count, avgDuration := m.GetRPCStats("getinfo")
	if count != 2 {
		t.Errorf("Expected 2 requests, got %d", count)
	}
	if avgDuration != expectedGetInfoDuration {
		t.Errorf("Expected avg duration %v, got %v", expectedGetInfoDuration, avgDuration)
	}
}

// TestErrorCategoryMetrics tests error breakdown by category
func TestErrorCategoryMetrics(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Record different types of errors
	m.RecordNetworkError()
	m.RecordNetworkError()
	m.RecordDatabaseError()
	m.RecordValidationError("subsidy")

	snapshot := m.Snapshot()

	if snapshot.NetworkErrors != 2 {
		t.Errorf("Expected 2 network errors, got %d", snapshot.NetworkErrors)
	}
	if snapshot.DatabaseErrors != 1 {
		t.Errorf("Expected 1 database error, got %d", snapshot.DatabaseErrors)
	}
	if snapshot.ValidationErrors != 1 {
		t.Errorf("Expected 1 validation error, got %d", snapshot.ValidationErrors)
	}
}

// TestPrometheusCollector_NewMetrics tests that new metrics are properly exposed
func TestPrometheusCollector_NewMetrics(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Set some values
	m.UpdateDatabaseSize(5000000) // 5MB
	m.RecordDatabaseRead(10 * time.Millisecond)
	m.RecordDatabaseWrite(20 * time.Millisecond)
	m.RecordRPCRequest("getinfo", 5*time.Millisecond)
	m.RecordRPCRequest("name_show", 15*time.Millisecond)
	m.RecordNetworkError()
	m.RecordDatabaseError()

	collector := NewPrometheusCollector(m)

	// Create a registry and register the collector
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Failed to register collector: %v", err)
	}

	// Gather metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that new metrics are present
	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[*mf.Name] = true
	}

	expectedNewMetrics := []string{
		"nmcd_namedb_size_bytes",
		"nmcd_namedb_read_latency_seconds",
		"nmcd_namedb_write_latency_seconds",
		"nmcd_errors_total",
		"nmcd_rpc_requests_total",
		"nmcd_rpc_duration_seconds",
		"nmcd_go_goroutines",
		"nmcd_go_memstats_alloc_bytes",
		"nmcd_go_memstats_heap_alloc_bytes",
		"nmcd_go_memstats_heap_idle_bytes",
		"nmcd_go_memstats_heap_inuse_bytes",
	}

	for _, metricName := range expectedNewMetrics {
		if !foundMetrics[metricName] {
			t.Errorf("Expected metric %s not found", metricName)
		}
	}
}

// TestPrometheusCollector_RPCLabels tests that RPC metrics have proper method labels
func TestPrometheusCollector_RPCLabels(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Record RPC requests for different methods
	m.RecordRPCRequest("getinfo", 5*time.Millisecond)
	m.RecordRPCRequest("name_show", 10*time.Millisecond)
	m.RecordRPCRequest("getinfo", 15*time.Millisecond)

	collector := NewPrometheusCollector(m)

	// Check that metrics have the correct labels
	expected := `
# HELP nmcd_rpc_requests_total Total number of RPC requests by method
# TYPE nmcd_rpc_requests_total counter
nmcd_rpc_requests_total{method="getinfo"} 2
nmcd_rpc_requests_total{method="name_show"} 1
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_rpc_requests_total"); err != nil {
		t.Errorf("RPC metrics comparison failed: %v", err)
	}
}

// TestPrometheusCollector_RPCDurationAverage tests that RPC duration emits average, not total
func TestPrometheusCollector_RPCDurationAverage(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Record 2 requests: 5ms and 15ms (average should be 10ms = 0.01s)
	m.RecordRPCRequest("getinfo", 5*time.Millisecond)
	m.RecordRPCRequest("getinfo", 15*time.Millisecond)

	collector := NewPrometheusCollector(m)

	// Verify that Prometheus emits the average (0.01s), not the total (0.02s)
	expected := `
# HELP nmcd_rpc_duration_seconds Average RPC request duration in seconds by method
# TYPE nmcd_rpc_duration_seconds gauge
nmcd_rpc_duration_seconds{method="getinfo"} 0.01
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_rpc_duration_seconds"); err != nil {
		t.Errorf("RPC duration metrics comparison failed: %v", err)
	}
}

// TestPrometheusCollector_ErrorLabels tests that error metrics have proper category labels
func TestPrometheusCollector_ErrorLabels(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	// Record errors by category
	m.RecordNetworkError()
	m.RecordNetworkError()
	m.RecordDatabaseError()
	m.RecordValidationError("subsidy")
	m.RecordValidationError("auxpow")

	collector := NewPrometheusCollector(m)

	// Check that error metrics have the correct labels
	expected := `
# HELP nmcd_errors_total Total number of errors by category
# TYPE nmcd_errors_total counter
nmcd_errors_total{type="database"} 1
nmcd_errors_total{type="network"} 2
nmcd_errors_total{type="validation"} 2
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_errors_total"); err != nil {
		t.Errorf("Error metrics comparison failed: %v", err)
	}
}

// TestPrometheusCollector_GoMetrics tests that Go runtime metrics are collected
func TestPrometheusCollector_GoMetrics(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	collector := NewPrometheusCollector(m)

	// Create a registry and register the collector
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Failed to register collector: %v", err)
	}

	// Gather metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Find Go metrics
	goMetricsFound := 0
	for _, mf := range metricFamilies {
		if strings.HasPrefix(*mf.Name, "nmcd_go_") {
			goMetricsFound++
			// Verify that the metric has a value > 0
			if len(mf.Metric) > 0 {
				value := mf.Metric[0].GetGauge().GetValue()
				if value <= 0 {
					t.Errorf("Go metric %s should have value > 0, got %f", *mf.Name, value)
				}
			}
		}
	}

	if goMetricsFound != 5 {
		t.Errorf("Expected 5 Go runtime metrics, found %d", goMetricsFound)
	}
}

// TestMetrics_Concurrent tests concurrent access to new metrics
func TestMetrics_Concurrent(t *testing.T) {
	m := &Metrics{
		StartTime:    time.Now(),
		rpcRequests:  make(map[string]uint64),
		rpcDurations: make(map[string]time.Duration),
	}

	done := make(chan bool)

	// Concurrent database operations
	go func() {
		for i := 0; i < 100; i++ {
			m.RecordDatabaseRead(time.Millisecond)
			m.RecordDatabaseWrite(time.Millisecond)
		}
		done <- true
	}()

	// Concurrent RPC operations
	go func() {
		for i := 0; i < 100; i++ {
			m.RecordRPCRequest("getinfo", time.Millisecond)
			m.RecordRPCRequest("name_show", time.Millisecond)
		}
		done <- true
	}()

	// Concurrent error recording
	go func() {
		for i := 0; i < 100; i++ {
			m.RecordNetworkError()
			m.RecordDatabaseError()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify counts
	snapshot := m.Snapshot()
	if snapshot.NetworkErrors != 100 {
		t.Errorf("Expected 100 network errors, got %d", snapshot.NetworkErrors)
	}
	if snapshot.DatabaseErrors != 100 {
		t.Errorf("Expected 100 database errors, got %d", snapshot.DatabaseErrors)
	}
	if snapshot.RPCRequests["getinfo"] != 100 {
		t.Errorf("Expected 100 getinfo requests, got %d", snapshot.RPCRequests["getinfo"])
	}
}

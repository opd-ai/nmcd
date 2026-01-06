package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusCollector_Creation(t *testing.T) {
	m := &Metrics{
		StartTime: time.Now(),
	}
	collector := NewPrometheusCollector(m)

	if collector == nil {
		t.Fatal("NewPrometheusCollector returned nil")
	}

	if collector.metrics != m {
		t.Error("Collector metrics reference not set correctly")
	}
}

func TestPrometheusCollector_Describe(t *testing.T) {
	m := &Metrics{
		StartTime: time.Now(),
	}
	collector := NewPrometheusCollector(m)

	// Create a channel to receive descriptors
	ch := make(chan *prometheus.Desc, 50)
	collector.Describe(ch)
	close(ch)

	// Count the number of descriptors
	count := 0
	for range ch {
		count++
	}

	// We expect 43 metrics (32 original + 11 new metrics)
	// Original: 32 (blocks, names, peers, txs, validation, performance)
	// New: 11 (3 database, 1 error breakdown, 2 RPC, 5 Go runtime)
	expectedCount := 43
	if count != expectedCount {
		t.Errorf("Expected %d metric descriptors, got %d", expectedCount, count)
	}
}

func TestPrometheusCollector_Collect(t *testing.T) {
	m := &Metrics{
		StartTime:       time.Now().Add(-1 * time.Hour),
		BlocksProcessed: 100,
		BlocksAccepted:  95,
		BlocksOrphaned:  3,
		BlocksRejected:  2,
		LastBlockHeight: 1000,
		NameNew:         10,
		NameFirstUpdate: 8,
		NameUpdate:      20,
		PeersConnected:  5,
		TxsInMempool:    15,
	}

	collector := NewPrometheusCollector(m)

	// Register the collector
	reg := prometheus.NewRegistry()
	err := reg.Register(collector)
	if err != nil {
		t.Fatalf("Failed to register collector: %v", err)
	}

	// Gather metrics
	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Fatal("No metrics were gathered")
	}

	// Verify a few key metrics
	found := make(map[string]bool)
	for _, mf := range metricFamilies {
		found[*mf.Name] = true
	}

	expectedMetrics := []string{
		"nmcd_blocks_processed_total",
		"nmcd_blocks_accepted_total",
		"nmcd_last_block_height",
		"nmcd_peers_connected",
		"nmcd_txs_in_mempool",
		"nmcd_uptime_seconds",
	}

	for _, name := range expectedMetrics {
		if !found[name] {
			t.Errorf("Expected metric %q not found in gathered metrics", name)
		}
	}
}

func TestPrometheusCollector_MetricValues(t *testing.T) {
	m := &Metrics{
		StartTime:       time.Now().Add(-2 * time.Hour),
		BlocksProcessed: 250,
		BlocksAccepted:  240,
		LastBlockHeight: 5000,
		PeersConnected:  10,
	}

	collector := NewPrometheusCollector(m)
	reg := prometheus.NewRegistry()
	reg.Register(collector)

	// Test blocks processed counter
	expected := `
# HELP nmcd_blocks_processed_total Total number of blocks processed
# TYPE nmcd_blocks_processed_total counter
nmcd_blocks_processed_total 250
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_blocks_processed_total"); err != nil {
		t.Errorf("Unexpected metric value for blocks_processed: %v", err)
	}

	// Test last block height gauge
	expected = `
# HELP nmcd_last_block_height Height of the last processed block
# TYPE nmcd_last_block_height gauge
nmcd_last_block_height 5000
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_last_block_height"); err != nil {
		t.Errorf("Unexpected metric value for last_block_height: %v", err)
	}

	// Test peers connected gauge
	expected = `
# HELP nmcd_peers_connected Number of currently connected peers
# TYPE nmcd_peers_connected gauge
nmcd_peers_connected 10
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_peers_connected"); err != nil {
		t.Errorf("Unexpected metric value for peers_connected: %v", err)
	}
}

func TestPrometheusCollector_NameOperations(t *testing.T) {
	m := &Metrics{
		StartTime:           time.Now(),
		NameOperationsTotal: 50,
		NameNew:             15,
		NameFirstUpdate:     10,
		NameUpdate:          25,
	}

	collector := NewPrometheusCollector(m)
	reg := prometheus.NewRegistry()
	reg.Register(collector)

	expected := `
# HELP nmcd_name_operations_total Total number of name operations processed
# TYPE nmcd_name_operations_total counter
nmcd_name_operations_total 50
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_name_operations_total"); err != nil {
		t.Errorf("Unexpected metric value for name_operations_total: %v", err)
	}

	expected = `
# HELP nmcd_name_new_total Total number of NAME_NEW operations
# TYPE nmcd_name_new_total counter
nmcd_name_new_total 15
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_name_new_total"); err != nil {
		t.Errorf("Unexpected metric value for name_new_total: %v", err)
	}
}

func TestPrometheusCollector_ValidationErrors(t *testing.T) {
	m := &Metrics{
		StartTime:           time.Now(),
		ValidationErrors:    10,
		AuxPoWErrors:        3,
		SubsidyErrors:       2,
		NameTheftAttempts:   1,
		DoubleSpendAttempts: 1,
	}

	collector := NewPrometheusCollector(m)
	reg := prometheus.NewRegistry()
	reg.Register(collector)

	expected := `
# HELP nmcd_validation_errors_total Total number of validation errors
# TYPE nmcd_validation_errors_total counter
nmcd_validation_errors_total 10
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_validation_errors_total"); err != nil {
		t.Errorf("Unexpected metric value for validation_errors_total: %v", err)
	}

	expected = `
# HELP nmcd_auxpow_errors_total Total number of AuxPoW validation errors
# TYPE nmcd_auxpow_errors_total counter
nmcd_auxpow_errors_total 3
`
	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected), "nmcd_auxpow_errors_total"); err != nil {
		t.Errorf("Unexpected metric value for auxpow_errors_total: %v", err)
	}
}

func TestPrometheusCollector_Uptime(t *testing.T) {
	// Set start time to 1 hour ago
	m := &Metrics{
		StartTime: time.Now().Add(-1 * time.Hour),
	}

	_ = NewPrometheusCollector(m) // Create collector to test it doesn't panic
	snapshot := m.Snapshot()

	// Uptime should be approximately 3600 seconds (1 hour)
	// Allow for some variance due to test execution time
	if snapshot.Uptime.Seconds() < 3599 || snapshot.Uptime.Seconds() > 3601 {
		t.Errorf("Expected uptime around 3600 seconds, got %.2f", snapshot.Uptime.Seconds())
	}
}

func TestPrometheusCollector_ConcurrentAccess(t *testing.T) {
	m := Get() // Use global metrics instance
	collector := NewPrometheusCollector(m)

	// Simulate concurrent metric updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.RecordBlockProcessed("hash", 1, true, false, time.Millisecond)
				m.RecordNameOperation("NAME_NEW")
				m.UpdatePeerCount(5, 2, 3)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Collect metrics - should not panic
	reg := prometheus.NewRegistry()
	reg.Register(collector)
	_, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics after concurrent updates: %v", err)
	}
}

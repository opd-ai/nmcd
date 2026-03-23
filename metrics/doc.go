// Package metrics provides runtime instrumentation for nmcd nodes.
//
// The metrics package implements comprehensive monitoring for Namecoin node
// operations without requiring external dependencies. It tracks block processing,
// name operations, peer connections, transaction handling, and various error
// categories to support operational monitoring and debugging.
//
// # Metrics Categories
//
// The package tracks several categories of metrics:
//
// Block Processing:
//   - BlocksProcessed: Total blocks processed
//   - BlocksAccepted: Blocks accepted on main chain
//   - BlocksOrphaned: Orphaned blocks received
//   - BlocksRejected: Blocks rejected due to validation errors
//
// Name Operations:
//   - NameOperationsTotal: Total name operations processed
//   - NameNew/NameFirstUpdate/NameUpdate: Operations by type
//   - NamesExpired: Names expired during processing
//   - NameErrors: Name operation validation errors
//
// Peer Management:
//   - PeersConnected: Currently connected peers
//   - InboundPeers/OutboundPeers: Peer counts by direction
//   - PeerDisconnects: Total peer disconnections
//
// Validation Errors:
//   - SubsidyErrors: Block subsidy validation errors
//   - ProofOfWorkErrors: PoW validation errors
//   - AuxPoWErrors: AuxPow validation errors
//   - NameTheftAttempts: Detected name theft attempts
//
// # Prometheus Integration
//
// The package provides optional Prometheus metrics export:
//
//	// Create metrics instance
//	m := metrics.NewMetrics()
//
//	// Register Prometheus collectors
//	prometheus.MustRegister(metrics.NewPrometheusCollector(m))
//
//	// Expose metrics endpoint
//	http.Handle("/metrics", promhttp.Handler())
//
// # Thread Safety
//
// All Metrics methods are safe for concurrent use. The struct uses sync.RWMutex
// internally to protect all counters and gauges.
//
// # Example Usage
//
// Recording metrics:
//
//	m := metrics.NewMetrics()
//
//	// Record block processing
//	m.RecordBlockProcessed(hash, height)
//	m.RecordBlockAccepted()
//
//	// Record name operations
//	m.RecordNameOperation(namedb.NameFirstUpdate)
//	m.RecordNameExpired()
//
//	// Record validation errors
//	m.RecordValidationError("auxpow")
//	m.RecordNameTheftAttempt()
//
// Getting a metrics snapshot:
//
//	snapshot := m.Snapshot()
//	fmt.Printf("Blocks processed: %d\n", snapshot.BlocksProcessed)
//	fmt.Printf("Names registered: %d\n", snapshot.NameFirstUpdate)
//	fmt.Printf("Peers connected: %d\n", snapshot.PeersConnected)
//
// # Global Instance
//
// A global metrics instance is available for convenience:
//
//	metrics.Global().RecordBlockProcessed(hash, height)
//
// However, creating explicit instances is preferred for better testability
// and explicit dependency injection.
package metrics

package metrics

import (
	"sync"
	"time"
)

// Metrics holds runtime metrics for nmcd
// This provides basic instrumentation for monitoring node health and protocol compliance
// without requiring external dependencies.
type Metrics struct {
	mu sync.RWMutex

	// Block processing metrics
	BlocksProcessed       uint64    // Total blocks processed
	BlocksAccepted        uint64    // Blocks accepted on main chain
	BlocksOrphaned        uint64    // Orphaned blocks received
	BlocksRejected        uint64    // Blocks rejected due to validation errors
	LastBlockTime         time.Time // Timestamp of last processed block
	LastBlockHash         string    // Hash of last processed block
	LastBlockHeight       int32     // Height of last processed block
	BlockProcessingErrors uint64    // Count of block processing errors

	// Name operation metrics
	NameOperationsTotal uint64 // Total name operations processed
	NameNew             uint64 // NAME_NEW operations
	NameFirstUpdate     uint64 // NAME_FIRSTUPDATE operations
	NameUpdate          uint64 // NAME_UPDATE operations
	NamesExpired        uint64 // Names expired during processing
	NameErrors          uint64 // Name operation validation errors

	// Reorganization metrics
	Reorgs      uint64    // Number of blockchain reorganizations
	LastReorg   time.Time // Time of last reorganization
	ReorgBlocks uint64    // Total blocks rolled back during reorgs

	// Peer metrics
	PeersConnected    uint32    // Currently connected peers
	PeersMax          uint32    // Maximum peers reached
	InboundPeers      uint32    // Current inbound peer count
	OutboundPeers     uint32    // Current outbound peer count
	PeerDisconnects   uint64    // Total peer disconnections
	LastPeerConnected time.Time // Last peer connection time

	// Transaction metrics
	TxsProcessed uint64 // Total transactions processed
	TxsInMempool uint64 // Current transactions in mempool

	// Validation metrics
	ValidationErrors    uint64 // Total validation errors
	SubsidyErrors       uint64 // Block subsidy validation errors
	ProofOfWorkErrors   uint64 // Proof of work validation errors
	AuxPoWErrors        uint64 // AuxPow validation errors
	VersionErrors       uint64 // Block version validation errors
	DustLimitErrors     uint64 // Dust limit validation errors
	TimingWindowErrors  uint64 // Name timing window errors
	NameTheftAttempts   uint64 // Name theft attempts detected
	DoubleSpendAttempts uint64 // Double-spend attempts detected

	// Performance metrics
	StartTime           time.Time     // Node start time
	AvgBlockProcessTime time.Duration // Average block processing time
	totalBlockTime      time.Duration // Total time spent processing blocks (internal)

	// Database metrics
	NameDBSizeBytes    uint64        // Size of name database in bytes
	NameDBReadLatency  time.Duration // Average namedb read latency
	NameDBWriteLatency time.Duration // Average namedb write latency
	totalDBReadTime    time.Duration // Total time spent on DB reads (internal)
	totalDBWriteTime   time.Duration // Total time spent on DB writes (internal)
	dbReadCount        uint64        // Number of DB reads (internal)
	dbWriteCount       uint64        // Number of DB writes (internal)

	// RPC metrics (per-method tracking)
	rpcRequests  map[string]uint64        // RPC requests by method
	rpcDurations map[string]time.Duration // Total RPC duration by method

	// Error metrics by category
	NetworkErrors  uint64 // Network-related errors
	DatabaseErrors uint64 // Database-related errors
	// ValidationErrors already exists above
}

// Global metrics instance
var global = &Metrics{
	StartTime:    time.Now(),
	rpcRequests:  make(map[string]uint64),
	rpcDurations: make(map[string]time.Duration),
}

// Get returns the global metrics instance
func Get() *Metrics {
	return global
}

// RecordBlockProcessed records a successfully processed block
func (m *Metrics) RecordBlockProcessed(hash string, height int32, isMainChain bool, isOrphan bool, processingTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.BlocksProcessed++
	if isMainChain {
		m.BlocksAccepted++
	}
	if isOrphan {
		m.BlocksOrphaned++
	}
	m.LastBlockTime = time.Now()
	m.LastBlockHash = hash
	m.LastBlockHeight = height

	// Update average processing time
	m.totalBlockTime += processingTime
	if m.BlocksProcessed > 0 {
		m.AvgBlockProcessTime = m.totalBlockTime / time.Duration(m.BlocksProcessed)
	}
}

// RecordBlockRejected records a block that was rejected during validation
func (m *Metrics) RecordBlockRejected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlocksRejected++
	m.BlockProcessingErrors++
}

// RecordNameOperation records a name operation
func (m *Metrics) RecordNameOperation(opType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.NameOperationsTotal++
	switch opType {
	case "NAME_NEW":
		m.NameNew++
	case "NAME_FIRSTUPDATE":
		m.NameFirstUpdate++
	case "NAME_UPDATE":
		m.NameUpdate++
	}
}

// RecordNameExpired records an expired name
func (m *Metrics) RecordNameExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NamesExpired++
}

// RecordNameError records a name validation error
func (m *Metrics) RecordNameError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NameErrors++
	m.ValidationErrors++
}

// RecordReorg records a blockchain reorganization
func (m *Metrics) RecordReorg(blocksRolledBack uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Reorgs++
	m.LastReorg = time.Now()
	m.ReorgBlocks += blocksRolledBack
}

// UpdatePeerCount updates the current peer count
func (m *Metrics) UpdatePeerCount(total, inbound, outbound uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PeersConnected = total
	m.InboundPeers = inbound
	m.OutboundPeers = outbound
	if total > m.PeersMax {
		m.PeersMax = total
	}
	if total > 0 {
		m.LastPeerConnected = time.Now()
	}
}

// RecordPeerDisconnect records a peer disconnection
func (m *Metrics) RecordPeerDisconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PeerDisconnects++
}

// RecordTransaction records a processed transaction
func (m *Metrics) RecordTransaction() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TxsProcessed++
}

// UpdateMempoolSize updates the mempool transaction count
func (m *Metrics) UpdateMempoolSize(count uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TxsInMempool = count
}

// RecordValidationError records a validation error by type
func (m *Metrics) RecordValidationError(errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ValidationErrors++

	switch errorType {
	case "subsidy":
		m.SubsidyErrors++
	case "proof_of_work":
		m.ProofOfWorkErrors++
	case "auxpow":
		m.AuxPoWErrors++
	case "version":
		m.VersionErrors++
	case "dust_limit":
		m.DustLimitErrors++
	case "timing_window":
		m.TimingWindowErrors++
	case "name_theft":
		m.NameTheftAttempts++
	case "double_spend":
		m.DoubleSpendAttempts++
	}
}

// RecordDatabaseRead records a database read operation
func (m *Metrics) RecordDatabaseRead(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalDBReadTime += duration
	m.dbReadCount++
	if m.dbReadCount > 0 {
		m.NameDBReadLatency = m.totalDBReadTime / time.Duration(m.dbReadCount)
	}
}

// RecordDatabaseWrite records a database write operation
func (m *Metrics) RecordDatabaseWrite(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalDBWriteTime += duration
	m.dbWriteCount++
	if m.dbWriteCount > 0 {
		m.NameDBWriteLatency = m.totalDBWriteTime / time.Duration(m.dbWriteCount)
	}
}

// UpdateDatabaseSize updates the database size metric
func (m *Metrics) UpdateDatabaseSize(sizeBytes uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NameDBSizeBytes = sizeBytes
}

// RecordRPCRequest records an RPC request with its method and duration
func (m *Metrics) RecordRPCRequest(method string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rpcRequests[method]++
	m.rpcDurations[method] += duration
}

// GetRPCStats returns RPC statistics for a specific method
func (m *Metrics) GetRPCStats(method string) (count uint64, avgDuration time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count = m.rpcRequests[method]
	if count > 0 {
		avgDuration = m.rpcDurations[method] / time.Duration(count)
	}
	return
}

// RecordNetworkError records a network-related error
func (m *Metrics) RecordNetworkError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NetworkErrors++
}

// RecordDatabaseError records a database-related error
func (m *Metrics) RecordDatabaseError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DatabaseErrors++
}

// Snapshot returns a snapshot of current metrics
// This creates a copy to avoid holding the lock during serialization
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		// Block metrics
		BlocksProcessed:       m.BlocksProcessed,
		BlocksAccepted:        m.BlocksAccepted,
		BlocksOrphaned:        m.BlocksOrphaned,
		BlocksRejected:        m.BlocksRejected,
		LastBlockTime:         m.LastBlockTime,
		LastBlockHash:         m.LastBlockHash,
		LastBlockHeight:       m.LastBlockHeight,
		BlockProcessingErrors: m.BlockProcessingErrors,

		// Name operation metrics
		NameOperationsTotal: m.NameOperationsTotal,
		NameNew:             m.NameNew,
		NameFirstUpdate:     m.NameFirstUpdate,
		NameUpdate:          m.NameUpdate,
		NamesExpired:        m.NamesExpired,
		NameErrors:          m.NameErrors,

		// Reorganization metrics
		Reorgs:      m.Reorgs,
		LastReorg:   m.LastReorg,
		ReorgBlocks: m.ReorgBlocks,

		// Peer metrics
		PeersConnected:    m.PeersConnected,
		PeersMax:          m.PeersMax,
		InboundPeers:      m.InboundPeers,
		OutboundPeers:     m.OutboundPeers,
		PeerDisconnects:   m.PeerDisconnects,
		LastPeerConnected: m.LastPeerConnected,

		// Transaction metrics
		TxsProcessed: m.TxsProcessed,
		TxsInMempool: m.TxsInMempool,

		// Validation metrics
		ValidationErrors:    m.ValidationErrors,
		SubsidyErrors:       m.SubsidyErrors,
		ProofOfWorkErrors:   m.ProofOfWorkErrors,
		AuxPoWErrors:        m.AuxPoWErrors,
		VersionErrors:       m.VersionErrors,
		DustLimitErrors:     m.DustLimitErrors,
		TimingWindowErrors:  m.TimingWindowErrors,
		NameTheftAttempts:   m.NameTheftAttempts,
		DoubleSpendAttempts: m.DoubleSpendAttempts,

		// Performance metrics
		StartTime:           m.StartTime,
		Uptime:              time.Since(m.StartTime),
		AvgBlockProcessTime: m.AvgBlockProcessTime,

		// Database metrics
		NameDBSizeBytes:    m.NameDBSizeBytes,
		NameDBReadLatency:  m.NameDBReadLatency,
		NameDBWriteLatency: m.NameDBWriteLatency,

		// Error metrics by category
		NetworkErrors:  m.NetworkErrors,
		DatabaseErrors: m.DatabaseErrors,

		// RPC metrics - create a copy of the maps
		RPCRequests:  copyUint64Map(m.rpcRequests),
		RPCDurations: copyDurationMap(m.rpcDurations),
	}
}

// copyUint64Map creates a copy of a uint64 map
func copyUint64Map(src map[string]uint64) map[string]uint64 {
	dst := make(map[string]uint64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// copyDurationMap creates a copy of a duration map
func copyDurationMap(src map[string]time.Duration) map[string]time.Duration {
	dst := make(map[string]time.Duration, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// MetricsSnapshot is an immutable copy of metrics at a point in time
type MetricsSnapshot struct {
	// Block metrics
	BlocksProcessed       uint64    `json:"blocks_processed"`
	BlocksAccepted        uint64    `json:"blocks_accepted"`
	BlocksOrphaned        uint64    `json:"blocks_orphaned"`
	BlocksRejected        uint64    `json:"blocks_rejected"`
	LastBlockTime         time.Time `json:"last_block_time"`
	LastBlockHash         string    `json:"last_block_hash"`
	LastBlockHeight       int32     `json:"last_block_height"`
	BlockProcessingErrors uint64    `json:"block_processing_errors"`

	// Name operation metrics
	NameOperationsTotal uint64 `json:"name_operations_total"`
	NameNew             uint64 `json:"name_new"`
	NameFirstUpdate     uint64 `json:"name_firstupdate"`
	NameUpdate          uint64 `json:"name_update"`
	NamesExpired        uint64 `json:"names_expired"`
	NameErrors          uint64 `json:"name_errors"`

	// Reorganization metrics
	Reorgs      uint64    `json:"reorgs"`
	LastReorg   time.Time `json:"last_reorg"`
	ReorgBlocks uint64    `json:"reorg_blocks"`

	// Peer metrics
	PeersConnected    uint32    `json:"peers_connected"`
	PeersMax          uint32    `json:"peers_max"`
	InboundPeers      uint32    `json:"inbound_peers"`
	OutboundPeers     uint32    `json:"outbound_peers"`
	PeerDisconnects   uint64    `json:"peer_disconnects"`
	LastPeerConnected time.Time `json:"last_peer_connected"`

	// Transaction metrics
	TxsProcessed uint64 `json:"txs_processed"`
	TxsInMempool uint64 `json:"txs_in_mempool"`

	// Validation metrics
	ValidationErrors    uint64 `json:"validation_errors"`
	SubsidyErrors       uint64 `json:"subsidy_errors"`
	ProofOfWorkErrors   uint64 `json:"proof_of_work_errors"`
	AuxPoWErrors        uint64 `json:"auxpow_errors"`
	VersionErrors       uint64 `json:"version_errors"`
	DustLimitErrors     uint64 `json:"dust_limit_errors"`
	TimingWindowErrors  uint64 `json:"timing_window_errors"`
	NameTheftAttempts   uint64 `json:"name_theft_attempts"`
	DoubleSpendAttempts uint64 `json:"double_spend_attempts"`

	// Performance metrics
	StartTime           time.Time     `json:"start_time"`
	Uptime              time.Duration `json:"uptime"`
	AvgBlockProcessTime time.Duration `json:"avg_block_process_time"`

	// Database metrics
	NameDBSizeBytes    uint64        `json:"namedb_size_bytes"`
	NameDBReadLatency  time.Duration `json:"namedb_read_latency"`
	NameDBWriteLatency time.Duration `json:"namedb_write_latency"`

	// Error metrics by category
	NetworkErrors  uint64 `json:"network_errors"`
	DatabaseErrors uint64 `json:"database_errors"`

	// RPC metrics
	RPCRequests  map[string]uint64        `json:"rpc_requests"`
	RPCDurations map[string]time.Duration `json:"rpc_durations"`
}

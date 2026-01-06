package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// goRuntimeStats holds cached Go runtime statistics
type goRuntimeStats struct {
	mu               sync.RWMutex
	goroutines       int
	allocBytes       uint64
	heapAllocBytes   uint64
	heapIdleBytes    uint64
	heapInuseBytes   uint64
	lastUpdate       time.Time
}

var cachedGoStats = &goRuntimeStats{}

func init() {
	// Update stats immediately on startup
	updateGoRuntimeStats()
	
	// Start background goroutine to update Go runtime stats every 30 seconds
	// This avoids stop-the-world pauses during Prometheus scrapes
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			updateGoRuntimeStats()
		}
	}()
}

func updateGoRuntimeStats() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	cachedGoStats.mu.Lock()
	defer cachedGoStats.mu.Unlock()
	
	cachedGoStats.goroutines = runtime.NumGoroutine()
	cachedGoStats.allocBytes = memStats.Alloc
	cachedGoStats.heapAllocBytes = memStats.HeapAlloc
	cachedGoStats.heapIdleBytes = memStats.HeapIdle
	cachedGoStats.heapInuseBytes = memStats.HeapInuse
	cachedGoStats.lastUpdate = time.Now()
}

// PrometheusCollector implements prometheus.Collector to expose nmcd metrics
// It reads directly from the global metrics instance on each scrape
type PrometheusCollector struct {
	metrics *Metrics

	// Metric descriptors
	blocksProcessedDesc       *prometheus.Desc
	blocksAcceptedDesc        *prometheus.Desc
	blocksOrphanedDesc        *prometheus.Desc
	blocksRejectedDesc        *prometheus.Desc
	lastBlockHeightDesc       *prometheus.Desc
	blockProcessingErrorsDesc *prometheus.Desc
	avgBlockProcessTimeDesc   *prometheus.Desc

	nameOperationsTotalDesc *prometheus.Desc
	nameNewDesc             *prometheus.Desc
	nameFirstUpdateDesc     *prometheus.Desc
	nameUpdateDesc          *prometheus.Desc
	namesExpiredDesc        *prometheus.Desc
	nameErrorsDesc          *prometheus.Desc

	reorgsDesc      *prometheus.Desc
	reorgBlocksDesc *prometheus.Desc

	peersConnectedDesc  *prometheus.Desc
	peersMaxDesc        *prometheus.Desc
	inboundPeersDesc    *prometheus.Desc
	outboundPeersDesc   *prometheus.Desc
	peerDisconnectsDesc *prometheus.Desc

	txsProcessedDesc *prometheus.Desc
	txsInMempoolDesc *prometheus.Desc

	validationErrorsDesc    *prometheus.Desc
	subsidyErrorsDesc       *prometheus.Desc
	proofOfWorkErrorsDesc   *prometheus.Desc
	auxpowErrorsDesc        *prometheus.Desc
	versionErrorsDesc       *prometheus.Desc
	dustLimitErrorsDesc     *prometheus.Desc
	timingWindowErrorsDesc  *prometheus.Desc
	nameTheftAttemptsDesc   *prometheus.Desc
	doubleSpendAttemptsDesc *prometheus.Desc

	uptimeDesc *prometheus.Desc

	// New metrics
	namedbSizeBytesDesc       *prometheus.Desc
	namedbReadLatencyDesc     *prometheus.Desc
	namedbWriteLatencyDesc    *prometheus.Desc
	errorsTotalDesc           *prometheus.Desc
	rpcRequestsTotalDesc      *prometheus.Desc
	rpcDurationSecondsDesc    *prometheus.Desc
	goGoroutinesDesc          *prometheus.Desc
	goMemstatAllocBytesDesc   *prometheus.Desc
	goMemstatHeapAllocDesc    *prometheus.Desc
	goMemstatHeapIdleDesc     *prometheus.Desc
	goMemstatHeapInuseDesc    *prometheus.Desc
}


// NewPrometheusCollector creates a new Prometheus collector for nmcd metrics
func NewPrometheusCollector(m *Metrics) *PrometheusCollector {
	return &PrometheusCollector{
		metrics: m,

		// Block processing metrics
		blocksProcessedDesc: prometheus.NewDesc(
			"nmcd_blocks_processed_total",
			"Total number of blocks processed",
			nil, nil,
		),
		blocksAcceptedDesc: prometheus.NewDesc(
			"nmcd_blocks_accepted_total",
			"Total number of blocks accepted on main chain",
			nil, nil,
		),
		blocksOrphanedDesc: prometheus.NewDesc(
			"nmcd_blocks_orphaned_total",
			"Total number of orphaned blocks received",
			nil, nil,
		),
		blocksRejectedDesc: prometheus.NewDesc(
			"nmcd_blocks_rejected_total",
			"Total number of blocks rejected due to validation errors",
			nil, nil,
		),
		lastBlockHeightDesc: prometheus.NewDesc(
			"nmcd_last_block_height",
			"Height of the last processed block",
			nil, nil,
		),
		blockProcessingErrorsDesc: prometheus.NewDesc(
			"nmcd_block_processing_errors_total",
			"Total number of block processing errors",
			nil, nil,
		),
		avgBlockProcessTimeDesc: prometheus.NewDesc(
			"nmcd_avg_block_process_time_seconds",
			"Average block processing time in seconds",
			nil, nil,
		),

		// Name operation metrics
		nameOperationsTotalDesc: prometheus.NewDesc(
			"nmcd_name_operations_total",
			"Total number of name operations processed",
			nil, nil,
		),
		nameNewDesc: prometheus.NewDesc(
			"nmcd_name_new_total",
			"Total number of NAME_NEW operations",
			nil, nil,
		),
		nameFirstUpdateDesc: prometheus.NewDesc(
			"nmcd_name_firstupdate_total",
			"Total number of NAME_FIRSTUPDATE operations",
			nil, nil,
		),
		nameUpdateDesc: prometheus.NewDesc(
			"nmcd_name_update_total",
			"Total number of NAME_UPDATE operations",
			nil, nil,
		),
		namesExpiredDesc: prometheus.NewDesc(
			"nmcd_names_expired_total",
			"Total number of names expired during processing",
			nil, nil,
		),
		nameErrorsDesc: prometheus.NewDesc(
			"nmcd_name_errors_total",
			"Total number of name operation validation errors",
			nil, nil,
		),

		// Reorganization metrics
		reorgsDesc: prometheus.NewDesc(
			"nmcd_reorgs_total",
			"Total number of blockchain reorganizations",
			nil, nil,
		),
		reorgBlocksDesc: prometheus.NewDesc(
			"nmcd_reorg_blocks_total",
			"Total number of blocks rolled back during reorganizations",
			nil, nil,
		),

		// Peer metrics
		peersConnectedDesc: prometheus.NewDesc(
			"nmcd_peers_connected",
			"Number of currently connected peers",
			nil, nil,
		),
		peersMaxDesc: prometheus.NewDesc(
			"nmcd_peers_max",
			"Maximum number of peers reached",
			nil, nil,
		),
		inboundPeersDesc: prometheus.NewDesc(
			"nmcd_inbound_peers",
			"Number of current inbound peers",
			nil, nil,
		),
		outboundPeersDesc: prometheus.NewDesc(
			"nmcd_outbound_peers",
			"Number of current outbound peers",
			nil, nil,
		),
		peerDisconnectsDesc: prometheus.NewDesc(
			"nmcd_peer_disconnects_total",
			"Total number of peer disconnections",
			nil, nil,
		),

		// Transaction metrics
		txsProcessedDesc: prometheus.NewDesc(
			"nmcd_txs_processed_total",
			"Total number of transactions processed",
			nil, nil,
		),
		txsInMempoolDesc: prometheus.NewDesc(
			"nmcd_txs_in_mempool",
			"Current number of transactions in mempool",
			nil, nil,
		),

		// Validation metrics
		validationErrorsDesc: prometheus.NewDesc(
			"nmcd_validation_errors_total",
			"Total number of validation errors",
			nil, nil,
		),
		subsidyErrorsDesc: prometheus.NewDesc(
			"nmcd_subsidy_errors_total",
			"Total number of block subsidy validation errors",
			nil, nil,
		),
		proofOfWorkErrorsDesc: prometheus.NewDesc(
			"nmcd_proof_of_work_errors_total",
			"Total number of proof of work validation errors",
			nil, nil,
		),
		auxpowErrorsDesc: prometheus.NewDesc(
			"nmcd_auxpow_errors_total",
			"Total number of AuxPoW validation errors",
			nil, nil,
		),
		versionErrorsDesc: prometheus.NewDesc(
			"nmcd_version_errors_total",
			"Total number of block version validation errors",
			nil, nil,
		),
		dustLimitErrorsDesc: prometheus.NewDesc(
			"nmcd_dust_limit_errors_total",
			"Total number of dust limit validation errors",
			nil, nil,
		),
		timingWindowErrorsDesc: prometheus.NewDesc(
			"nmcd_timing_window_errors_total",
			"Total number of name timing window errors",
			nil, nil,
		),
		nameTheftAttemptsDesc: prometheus.NewDesc(
			"nmcd_name_theft_attempts_total",
			"Total number of name theft attempts detected",
			nil, nil,
		),
		doubleSpendAttemptsDesc: prometheus.NewDesc(
			"nmcd_double_spend_attempts_total",
			"Total number of double-spend attempts detected",
			nil, nil,
		),

		// Performance metrics
		uptimeDesc: prometheus.NewDesc(
			"nmcd_uptime_seconds",
			"Node uptime in seconds",
			nil, nil,
		),

		// Database metrics
		namedbSizeBytesDesc: prometheus.NewDesc(
			"nmcd_namedb_size_bytes",
			"Size of name database in bytes",
			nil, nil,
		),
		namedbReadLatencyDesc: prometheus.NewDesc(
			"nmcd_namedb_read_latency_seconds",
			"Average name database read latency in seconds",
			nil, nil,
		),
		namedbWriteLatencyDesc: prometheus.NewDesc(
			"nmcd_namedb_write_latency_seconds",
			"Average name database write latency in seconds",
			nil, nil,
		),

		// Error breakdown with labels
		errorsTotalDesc: prometheus.NewDesc(
			"nmcd_errors_total",
			"Total number of errors by category",
			[]string{"type"}, // label: type (validation, network, database)
			nil,
		),

		// RPC metrics with labels
		rpcRequestsTotalDesc: prometheus.NewDesc(
			"nmcd_rpc_requests_total",
			"Total number of RPC requests by method",
			[]string{"method"}, // label: method name
			nil,
		),
		rpcDurationSecondsDesc: prometheus.NewDesc(
			"nmcd_rpc_duration_seconds",
			"Average RPC request duration in seconds by method",
			[]string{"method"}, // label: method name
			nil,
		),

		// Go runtime metrics
		goGoroutinesDesc: prometheus.NewDesc(
			"nmcd_go_goroutines",
			"Number of goroutines currently running",
			nil, nil,
		),
		goMemstatAllocBytesDesc: prometheus.NewDesc(
			"nmcd_go_memstats_alloc_bytes",
			"Number of bytes allocated and currently in use",
			nil, nil,
		),
		goMemstatHeapAllocDesc: prometheus.NewDesc(
			"nmcd_go_memstats_heap_alloc_bytes",
			"Number of heap bytes allocated and currently in use",
			nil, nil,
		),
		goMemstatHeapIdleDesc: prometheus.NewDesc(
			"nmcd_go_memstats_heap_idle_bytes",
			"Number of heap bytes waiting to be used",
			nil, nil,
		),
		goMemstatHeapInuseDesc: prometheus.NewDesc(
			"nmcd_go_memstats_heap_inuse_bytes",
			"Number of heap bytes that are in use",
			nil, nil,
		),
	}
}


// Describe implements prometheus.Collector
func (c *PrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.blocksProcessedDesc
	ch <- c.blocksAcceptedDesc
	ch <- c.blocksOrphanedDesc
	ch <- c.blocksRejectedDesc
	ch <- c.lastBlockHeightDesc
	ch <- c.blockProcessingErrorsDesc
	ch <- c.avgBlockProcessTimeDesc

	ch <- c.nameOperationsTotalDesc
	ch <- c.nameNewDesc
	ch <- c.nameFirstUpdateDesc
	ch <- c.nameUpdateDesc
	ch <- c.namesExpiredDesc
	ch <- c.nameErrorsDesc

	ch <- c.reorgsDesc
	ch <- c.reorgBlocksDesc

	ch <- c.peersConnectedDesc
	ch <- c.peersMaxDesc
	ch <- c.inboundPeersDesc
	ch <- c.outboundPeersDesc
	ch <- c.peerDisconnectsDesc

	ch <- c.txsProcessedDesc
	ch <- c.txsInMempoolDesc

	ch <- c.validationErrorsDesc
	ch <- c.subsidyErrorsDesc
	ch <- c.proofOfWorkErrorsDesc
	ch <- c.auxpowErrorsDesc
	ch <- c.versionErrorsDesc
	ch <- c.dustLimitErrorsDesc
	ch <- c.timingWindowErrorsDesc
	ch <- c.nameTheftAttemptsDesc
	ch <- c.doubleSpendAttemptsDesc

	ch <- c.uptimeDesc

	// New metrics
	ch <- c.namedbSizeBytesDesc
	ch <- c.namedbReadLatencyDesc
	ch <- c.namedbWriteLatencyDesc
	ch <- c.errorsTotalDesc
	ch <- c.rpcRequestsTotalDesc
	ch <- c.rpcDurationSecondsDesc
	ch <- c.goGoroutinesDesc
	ch <- c.goMemstatAllocBytesDesc
	ch <- c.goMemstatHeapAllocDesc
	ch <- c.goMemstatHeapIdleDesc
	ch <- c.goMemstatHeapInuseDesc
}

// Collect implements prometheus.Collector
func (c *PrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	snapshot := c.metrics.Snapshot()

	// Block processing metrics
	ch <- prometheus.MustNewConstMetric(c.blocksProcessedDesc, prometheus.CounterValue, float64(snapshot.BlocksProcessed))
	ch <- prometheus.MustNewConstMetric(c.blocksAcceptedDesc, prometheus.CounterValue, float64(snapshot.BlocksAccepted))
	ch <- prometheus.MustNewConstMetric(c.blocksOrphanedDesc, prometheus.CounterValue, float64(snapshot.BlocksOrphaned))
	ch <- prometheus.MustNewConstMetric(c.blocksRejectedDesc, prometheus.CounterValue, float64(snapshot.BlocksRejected))
	ch <- prometheus.MustNewConstMetric(c.lastBlockHeightDesc, prometheus.GaugeValue, float64(snapshot.LastBlockHeight))
	ch <- prometheus.MustNewConstMetric(c.blockProcessingErrorsDesc, prometheus.CounterValue, float64(snapshot.BlockProcessingErrors))
	ch <- prometheus.MustNewConstMetric(c.avgBlockProcessTimeDesc, prometheus.GaugeValue, snapshot.AvgBlockProcessTime.Seconds())

	// Name operation metrics
	ch <- prometheus.MustNewConstMetric(c.nameOperationsTotalDesc, prometheus.CounterValue, float64(snapshot.NameOperationsTotal))
	ch <- prometheus.MustNewConstMetric(c.nameNewDesc, prometheus.CounterValue, float64(snapshot.NameNew))
	ch <- prometheus.MustNewConstMetric(c.nameFirstUpdateDesc, prometheus.CounterValue, float64(snapshot.NameFirstUpdate))
	ch <- prometheus.MustNewConstMetric(c.nameUpdateDesc, prometheus.CounterValue, float64(snapshot.NameUpdate))
	ch <- prometheus.MustNewConstMetric(c.namesExpiredDesc, prometheus.CounterValue, float64(snapshot.NamesExpired))
	ch <- prometheus.MustNewConstMetric(c.nameErrorsDesc, prometheus.CounterValue, float64(snapshot.NameErrors))

	// Reorganization metrics
	ch <- prometheus.MustNewConstMetric(c.reorgsDesc, prometheus.CounterValue, float64(snapshot.Reorgs))
	ch <- prometheus.MustNewConstMetric(c.reorgBlocksDesc, prometheus.CounterValue, float64(snapshot.ReorgBlocks))

	// Peer metrics
	ch <- prometheus.MustNewConstMetric(c.peersConnectedDesc, prometheus.GaugeValue, float64(snapshot.PeersConnected))
	ch <- prometheus.MustNewConstMetric(c.peersMaxDesc, prometheus.GaugeValue, float64(snapshot.PeersMax))
	ch <- prometheus.MustNewConstMetric(c.inboundPeersDesc, prometheus.GaugeValue, float64(snapshot.InboundPeers))
	ch <- prometheus.MustNewConstMetric(c.outboundPeersDesc, prometheus.GaugeValue, float64(snapshot.OutboundPeers))
	ch <- prometheus.MustNewConstMetric(c.peerDisconnectsDesc, prometheus.CounterValue, float64(snapshot.PeerDisconnects))

	// Transaction metrics
	ch <- prometheus.MustNewConstMetric(c.txsProcessedDesc, prometheus.CounterValue, float64(snapshot.TxsProcessed))
	ch <- prometheus.MustNewConstMetric(c.txsInMempoolDesc, prometheus.GaugeValue, float64(snapshot.TxsInMempool))

	// Validation metrics
	ch <- prometheus.MustNewConstMetric(c.validationErrorsDesc, prometheus.CounterValue, float64(snapshot.ValidationErrors))
	ch <- prometheus.MustNewConstMetric(c.subsidyErrorsDesc, prometheus.CounterValue, float64(snapshot.SubsidyErrors))
	ch <- prometheus.MustNewConstMetric(c.proofOfWorkErrorsDesc, prometheus.CounterValue, float64(snapshot.ProofOfWorkErrors))
	ch <- prometheus.MustNewConstMetric(c.auxpowErrorsDesc, prometheus.CounterValue, float64(snapshot.AuxPoWErrors))
	ch <- prometheus.MustNewConstMetric(c.versionErrorsDesc, prometheus.CounterValue, float64(snapshot.VersionErrors))
	ch <- prometheus.MustNewConstMetric(c.dustLimitErrorsDesc, prometheus.CounterValue, float64(snapshot.DustLimitErrors))
	ch <- prometheus.MustNewConstMetric(c.timingWindowErrorsDesc, prometheus.CounterValue, float64(snapshot.TimingWindowErrors))
	ch <- prometheus.MustNewConstMetric(c.nameTheftAttemptsDesc, prometheus.CounterValue, float64(snapshot.NameTheftAttempts))
	ch <- prometheus.MustNewConstMetric(c.doubleSpendAttemptsDesc, prometheus.CounterValue, float64(snapshot.DoubleSpendAttempts))

	// Performance metrics
	ch <- prometheus.MustNewConstMetric(c.uptimeDesc, prometheus.GaugeValue, snapshot.Uptime.Seconds())

	// Database metrics
	ch <- prometheus.MustNewConstMetric(c.namedbSizeBytesDesc, prometheus.GaugeValue, float64(snapshot.NameDBSizeBytes))
	ch <- prometheus.MustNewConstMetric(c.namedbReadLatencyDesc, prometheus.GaugeValue, snapshot.NameDBReadLatency.Seconds())
	ch <- prometheus.MustNewConstMetric(c.namedbWriteLatencyDesc, prometheus.GaugeValue, snapshot.NameDBWriteLatency.Seconds())

	// Error breakdown by category (with labels)
	ch <- prometheus.MustNewConstMetric(c.errorsTotalDesc, prometheus.CounterValue, float64(snapshot.ValidationErrors), "validation")
	ch <- prometheus.MustNewConstMetric(c.errorsTotalDesc, prometheus.CounterValue, float64(snapshot.NetworkErrors), "network")
	ch <- prometheus.MustNewConstMetric(c.errorsTotalDesc, prometheus.CounterValue, float64(snapshot.DatabaseErrors), "database")

	// RPC metrics (per-method with labels)
	for method, count := range snapshot.RPCRequests {
		ch <- prometheus.MustNewConstMetric(c.rpcRequestsTotalDesc, prometheus.CounterValue, float64(count), method)
	}
	for method, duration := range snapshot.RPCDurations {
		count := snapshot.RPCRequests[method]
		if count == 0 {
			continue
		}
		avgDuration := duration.Seconds() / float64(count)
		ch <- prometheus.MustNewConstMetric(c.rpcDurationSecondsDesc, prometheus.GaugeValue, avgDuration, method)
	}

	// Go runtime metrics (from cached stats to avoid stop-the-world pauses)
	cachedGoStats.mu.RLock()
	ch <- prometheus.MustNewConstMetric(c.goGoroutinesDesc, prometheus.GaugeValue, float64(cachedGoStats.goroutines))
	ch <- prometheus.MustNewConstMetric(c.goMemstatAllocBytesDesc, prometheus.GaugeValue, float64(cachedGoStats.allocBytes))
	ch <- prometheus.MustNewConstMetric(c.goMemstatHeapAllocDesc, prometheus.GaugeValue, float64(cachedGoStats.heapAllocBytes))
	ch <- prometheus.MustNewConstMetric(c.goMemstatHeapIdleDesc, prometheus.GaugeValue, float64(cachedGoStats.heapIdleBytes))
	ch <- prometheus.MustNewConstMetric(c.goMemstatHeapInuseDesc, prometheus.GaugeValue, float64(cachedGoStats.heapInuseBytes))
	cachedGoStats.mu.RUnlock()
}


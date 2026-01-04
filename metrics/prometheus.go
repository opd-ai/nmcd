package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

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
}

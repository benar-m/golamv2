package metrics

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/benar-m/golamv2/internal/domain"
)

// MetricsCollector collects and manages crawler metrics
type MetricsCollector struct {
	metrics          *domain.CrawlMetrics
	lastResetTime    time.Time
	startTime        time.Time
	lastProcessCount int64
	// Component memory trackers
	bloomFilter BloomFilterMemory
	storage     StorageMemory
	queue       QueueMemory
}

// BloomFilterMemory interface for tracking bloom filter memory
type BloomFilterMemory interface {
	GetMemoryUsageMB() float64
}

// StorageMemory interface for tracking storage memory
type StorageMemory interface {
	GetMemoryUsageMB() float64
}

// QueueMemory interface for tracking queue memory
type QueueMemory interface {
	GetMemoryUsageMB() float64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	now := time.Now()
	return &MetricsCollector{
		metrics: &domain.CrawlMetrics{
			StartTime:       now,
			LastUpdateTime:  now,
			MemoryBreakdown: domain.MemoryBreakdown{},
		},
		lastResetTime:    now,
		startTime:        now,
		lastProcessCount: 0,
	}
}

// SetComponentMemoryTrackers sets the memory tracking components
func (m *MetricsCollector) SetComponentMemoryTrackers(bloom BloomFilterMemory, storage StorageMemory, queue QueueMemory) {
	m.bloomFilter = bloom
	m.storage = storage
	m.queue = queue
}

// UpdateURLsProcessed increments the processed URLs counter
func (m *MetricsCollector) UpdateURLsProcessed(delta int64) {
	atomic.AddInt64(&m.metrics.URLsProcessed, delta)
}

// UpdateURLsInQueue updates the URLs in queue counter
func (m *MetricsCollector) UpdateURLsInQueue(count int64) {
	atomic.StoreInt64(&m.metrics.URLsInQueue, count)
}

// UpdateURLsInDB updates the URLs in database counter
func (m *MetricsCollector) UpdateURLsInDB(count int64) {
	atomic.StoreInt64(&m.metrics.URLsInDB, count)
}

// UpdateEmailsFound increments the emails found counter
func (m *MetricsCollector) UpdateEmailsFound(delta int64) {
	atomic.AddInt64(&m.metrics.EmailsFound, delta)
}

// UpdateKeywordsFound increments the keywords found counter
func (m *MetricsCollector) UpdateKeywordsFound(delta int64) {
	atomic.AddInt64(&m.metrics.KeywordsFound, delta)
}

// UpdateLinksChecked increments the links checked counter
func (m *MetricsCollector) UpdateLinksChecked(delta int64) {
	atomic.AddInt64(&m.metrics.LinksChecked, delta)
}

// UpdateDeadLinksFound increments the dead links found counter
func (m *MetricsCollector) UpdateDeadLinksFound(delta int64) {
	atomic.AddInt64(&m.metrics.DeadLinksFound, delta)
}

// UpdateDeadDomainsFound increments the dead domains found counter
func (m *MetricsCollector) UpdateDeadDomainsFound(delta int64) {
	atomic.AddInt64(&m.metrics.DeadDomainsFound, delta)
}

// UpdateActiveWorkers updates the active workers counter
func (m *MetricsCollector) UpdateActiveWorkers(count int) {
	m.metrics.ActiveWorkers = count
}

// UpdateErrors increments the errors counter
func (m *MetricsCollector) UpdateErrors(delta int64) {
	atomic.AddInt64(&m.metrics.Errors, delta)
}

// GetMetrics returns current metrics with calculated values
func (m *MetricsCollector) GetMetrics() *domain.CrawlMetrics {
	now := time.Now()

	// Update calculated fields
	m.metrics.LastUpdateTime = now
	m.metrics.MemoryUsageMB = m.getMemoryUsageMB()
	m.metrics.URLsPerSecond = m.calculateURLsPerSecond()
	m.metrics.MemoryBreakdown = m.calculateMemoryBreakdown()

	// Return a copy to avoid race conditions
	metricsCopy := *m.metrics
	return &metricsCopy
}

// getMemoryUsageMB returns current memory usage in MB
func (m *MetricsCollector) getMemoryUsageMB() float64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Return allocated memory in MB
	return float64(memStats.Alloc) / 1024 / 1024
}

// calculateURLsPerSecond calculates the current URLs per second rate
func (m *MetricsCollector) calculateURLsPerSecond() float64 {
	currentCount := atomic.LoadInt64(&m.metrics.URLsProcessed)
	now := time.Now()

	elapsed := now.Sub(m.lastResetTime).Seconds()
	if elapsed < 1.0 {
		return m.metrics.URLsPerSecond // Return last calculated value
	}

	processed := currentCount - m.lastProcessCount
	rate := float64(processed) / elapsed

	// Update for next calculation
	m.lastResetTime = now
	m.lastProcessCount = currentCount

	return rate
}

// calculateMemoryBreakdown calculates memory usage by component
func (m *MetricsCollector) calculateMemoryBreakdown() domain.MemoryBreakdown {
	var breakdown domain.MemoryBreakdown

	// Get total memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	totalMB := float64(memStats.Alloc) / 1024 / 1024
	breakdown.TotalMB = totalMB

	// Get component-specific memory usage
	if m.bloomFilter != nil {
		breakdown.BloomFilterMB = m.bloomFilter.GetMemoryUsageMB()
	}

	if m.storage != nil {
		breakdown.DatabaseMB = m.storage.GetMemoryUsageMB()
	}

	if m.queue != nil {
		breakdown.QueueMB = m.queue.GetMemoryUsageMB()
	}

	// Estimate other components based on worker count and typical usage
	activeWorkers := float64(m.metrics.ActiveWorkers)

	// HTTP buffers: approximately 2MB per active worker (our optimization)
	breakdown.HTTPBuffersMB = activeWorkers * 2.0

	// Parsing: approximately 0.5MB per active worker for HTML parsing
	breakdown.ParsingMB = activeWorkers * 0.5

	// Crawlers: approximately 1MB per active worker for goroutine overhead
	breakdown.CrawlersMB = activeWorkers * 1.0

	// Calculate remaining memory as "other"
	accountedMemory := breakdown.BloomFilterMB + breakdown.DatabaseMB +
		breakdown.QueueMB + breakdown.HTTPBuffersMB +
		breakdown.ParsingMB + breakdown.CrawlersMB

	breakdown.OtherMB = totalMB - accountedMemory
	if breakdown.OtherMB < 0 {
		breakdown.OtherMB = 0
	}

	return breakdown
}

// Reset resets counters (useful for testing or restarting)
func (m *MetricsCollector) Reset() {
	now := time.Now()

	m.metrics = &domain.CrawlMetrics{
		StartTime:      now,
		LastUpdateTime: now,
	}

	m.lastResetTime = now
	m.lastProcessCount = 0
}

// GetUptimeSeconds returns the uptime in seconds
func (m *MetricsCollector) GetUptimeSeconds() float64 {
	return time.Since(m.startTime).Seconds()
}

// GetProcessingRate returns URLs processed per minute
func (m *MetricsCollector) GetProcessingRate() float64 {
	elapsed := time.Since(m.startTime).Minutes()
	if elapsed == 0 {
		return 0
	}

	return float64(atomic.LoadInt64(&m.metrics.URLsProcessed)) / elapsed
}

// GetTotalFinds returns total items found across all categories
func (m *MetricsCollector) GetTotalFinds() int64 {
	return atomic.LoadInt64(&m.metrics.EmailsFound) +
		atomic.LoadInt64(&m.metrics.KeywordsFound) +
		atomic.LoadInt64(&m.metrics.DeadLinksFound) +
		atomic.LoadInt64(&m.metrics.DeadDomainsFound)
}

// GetSuccessRate returns the success rate (processed without errors)
func (m *MetricsCollector) GetSuccessRate() float64 {
	processed := atomic.LoadInt64(&m.metrics.URLsProcessed)
	errors := atomic.LoadInt64(&m.metrics.Errors)

	if processed == 0 {
		return 100.0
	}

	return float64(processed-errors) / float64(processed) * 100.0
}

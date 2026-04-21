package infrastructure

import (
	"fmt"
	"path/filepath"

	"github.com/benar-m/golamv2/internal/domain"
	"github.com/benar-m/golamv2/pkg/bloom"
	"github.com/benar-m/golamv2/pkg/metrics"
	"github.com/benar-m/golamv2/pkg/queue"
	"github.com/benar-m/golamv2/pkg/storage"
)

// Infrastructure holds all infrastructure components
type Infrastructure struct {
	URLQueue         domain.URLQueue
	BloomFilter      domain.BloomFilter
	Storage          domain.Storage
	RobotsChecker    domain.RobotsChecker
	ContentExtractor domain.ContentExtractor
	Metrics          *metrics.MetricsCollector
}

// NewInfrastructure creates a new infrastructure instance
func NewInfrastructure(maxMemoryMB int) (*Infrastructure, error) {
	// Create metrics collector
	metricsCollector := metrics.NewMetricsCollector()

	// Create Bloom filter for URL deduplication
	bloomFilter := bloom.NewURLBloomFilter()

	// Create storage (default path in current directory)
	dbPath := filepath.Join(".", "golamv2_data")
	storage, err := storage.NewBadgerStorage(dbPath, domain.ModeAll, maxMemoryMB)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %v", err)
	}

	// Create URL queue
	urlQueue := queue.NewPriorityURLQueue(storage)

	// Create robots checker
	robotsChecker := NewRobotsChecker("GolamV2-Crawler/1.0")

	// Create content extractor
	contentExtractor := NewContentExtractor()

	// Set storage reference for async dead link processing
	contentExtractor.SetStorage(storage)

	// Set metrics reference for updating dead link counters
	contentExtractor.SetMetrics(metricsCollector)

	// Set up memory tracking components
	metricsCollector.SetComponentMemoryTrackers(bloomFilter, storage, urlQueue)

	return &Infrastructure{
		URLQueue:         urlQueue,
		BloomFilter:      bloomFilter,
		Storage:          storage,
		RobotsChecker:    robotsChecker,
		ContentExtractor: contentExtractor,
		Metrics:          metricsCollector,
	}, nil
}

// GetMetrics returns the metrics collector
func (i *Infrastructure) GetMetrics() *metrics.MetricsCollector {
	return i.Metrics
}

// Close closes all infrastructure components
func (i *Infrastructure) Close() error {
	var errors []error

	if err := i.URLQueue.Close(); err != nil {
		errors = append(errors, fmt.Errorf("failed to close URL queue: %v", err))
	}

	if err := i.Storage.Close(); err != nil {
		errors = append(errors, fmt.Errorf("failed to close storage: %v", err))
	}

	// Close the content extractor to shut down async workers
	if extractor, ok := i.ContentExtractor.(*ContentExtractor); ok {
		extractor.Close()
	}

	if len(errors) > 0 {
		return fmt.Errorf("infrastructure close errors: %v", errors)
	}

	return nil
}

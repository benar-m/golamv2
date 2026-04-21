package storage

// Took Up Badger After A chatgpt pros and cons. Ha!. In the Previous Version I used a sqlite but would suffer from write lock and bottlenecks due to its single item write nature.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/benar-m/golamv2/internal/domain"

	"github.com/dgraph-io/badger/v4"
)

const (
	URLPrefix    = "url:"
	ResultPrefix = "result:"
	MetricsKey   = "metrics"
	BatchSize    = 1000
)

// BadgerStorage implements domain.Storage using BadgerDB
type BadgerStorage struct {
	urlDB     *badger.DB
	resultsDB *badger.DB
	mode      domain.CrawlMode
	dbPath    string
	metrics   *domain.CrawlMetrics
	// Memory tracking
	allocatedMemoryMB float64
}

// NewBadgerStorage creates a new BadgerDB storage instance
func NewBadgerStorage(dbPath string, mode domain.CrawlMode, maxMemoryMB int) (*BadgerStorage, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %v", err)
	}

	// Ensure total memory usage stays within limits
	totalMemoryBytes := int64(maxMemoryMB) * 1024 * 1024
	urlMemory := totalMemoryBytes * 40 / 100    // 40% for URLs
	resultMemory := totalMemoryBytes * 30 / 100 // 30% for results
	// Reserve 30% for HTTP buffers, Bloom filter, and other overhead

	// Track allocated memory for monitoring (70% of total limit)
	allocatedMemoryMB := float64(maxMemoryMB) * 0.7

	// Open URL database
	urlOpts := badger.DefaultOptions(filepath.Join(dbPath, "urls"))
	urlOpts.Logger = nil // Disable logging for performance
	urlOpts.ValueLogMaxEntries = 1000000
	urlOpts.MemTableSize = urlMemory
	urlOpts.ValueLogFileSize = 64 << 20 // 64MB
	urlOpts.NumMemtables = 2
	urlOpts.NumLevelZeroTables = 2
	urlOpts.NumLevelZeroTablesStall = 4
	urlOpts.NumCompactors = 2
	urlOpts.CompactL0OnClose = true

	urlDB, err := badger.Open(urlOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open URL database: %v", err)
	}

	// Open results database with specific name based on mode
	resultsDBName := "finds"
	if mode != domain.ModeAll {
		resultsDBName = fmt.Sprintf("finds_%s", mode)
	}

	resultOpts := badger.DefaultOptions(filepath.Join(dbPath, resultsDBName))
	resultOpts.Logger = nil
	resultOpts.ValueLogMaxEntries = 1000000
	resultOpts.MemTableSize = resultMemory
	resultOpts.ValueLogFileSize = 64 << 20
	resultOpts.NumMemtables = 2
	resultOpts.NumLevelZeroTables = 2
	resultOpts.NumLevelZeroTablesStall = 4
	resultOpts.NumCompactors = 2
	resultOpts.CompactL0OnClose = true

	resultsDB, err := badger.Open(resultOpts)
	if err != nil {
		urlDB.Close()
		return nil, fmt.Errorf("failed to open results database: %v", err)
	}

	storage := &BadgerStorage{
		urlDB:     urlDB,
		resultsDB: resultsDB,
		mode:      mode,
		dbPath:    dbPath,
		metrics: &domain.CrawlMetrics{
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
		},
		allocatedMemoryMB: allocatedMemoryMB,
	}

	// Load existing metrics
	storage.loadMetrics()

	// Start background garbage collection
	go storage.startGC()

	return storage, nil
}

// StoreURL stores a URL task in the database
func (s *BadgerStorage) StoreURL(task domain.URLTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal URL task: %v", err)
	}

	key := fmt.Sprintf("%s%s", URLPrefix, task.URL)

	return s.urlDB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// GetURLs retrieves URL tasks from the database
func (s *BadgerStorage) GetURLs(limit int) ([]domain.URLTask, error) {
	var tasks []domain.URLTask

	err := s.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = BatchSize
		iterator := txn.NewIterator(opts)
		defer iterator.Close()

		prefix := []byte(URLPrefix)
		count := 0

		for iterator.Seek(prefix); iterator.ValidForPrefix(prefix) && count < limit; iterator.Next() {
			item := iterator.Item()

			err := item.Value(func(val []byte) error {
				var task domain.URLTask
				if err := json.Unmarshal(val, &task); err != nil {
					return err
				}
				tasks = append(tasks, task)
				return nil
			})

			if err != nil {
				return err
			}

			count++
		}

		return nil
	})

	// batch delet
	if err == nil && len(tasks) > 0 {
		s.deleteURLsBatch(tasks)
	}

	return tasks, err
}

func (s *BadgerStorage) deleteURLsBatch(tasks []domain.URLTask) {
	batch := s.urlDB.NewWriteBatch()
	defer batch.Cancel()

	for _, task := range tasks {
		key := fmt.Sprintf("%s%s", URLPrefix, task.URL)
		batch.Delete([]byte(key))
	}

	batch.Flush()
}

func (s *BadgerStorage) StoreResult(result domain.CrawlResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %v", err)
	}

	key := fmt.Sprintf("%s%s_%d", ResultPrefix, result.URL, result.ProcessedAt.Unix())

	err = s.resultsDB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	if err == nil {
		// Update metrics
		atomic.AddInt64(&s.metrics.URLsProcessed, 1)

		if len(result.Emails) > 0 {
			atomic.AddInt64(&s.metrics.EmailsFound, int64(len(result.Emails)))
		}
		if len(result.Keywords) > 0 {
			atomic.AddInt64(&s.metrics.KeywordsFound, int64(len(result.Keywords)))
		}
		if len(result.DeadLinks) > 0 {
			atomic.AddInt64(&s.metrics.DeadLinksFound, int64(len(result.DeadLinks)))
		}
		if len(result.DeadDomains) > 0 {
			atomic.AddInt64(&s.metrics.DeadDomainsFound, int64(len(result.DeadDomains)))
		}
		if result.Error != "" {
			atomic.AddInt64(&s.metrics.Errors, 1)
		}
	}

	return err
}

// Retrrieve Result from the database--CrawlResult
func (s *BadgerStorage) GetResults(mode domain.CrawlMode, limit int) ([]domain.CrawlResult, error) {
	var results []domain.CrawlResult

	err := s.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = BatchSize
		iterator := txn.NewIterator(opts)
		defer iterator.Close()

		prefix := []byte(ResultPrefix)
		count := 0

		for iterator.Seek(prefix); iterator.ValidForPrefix(prefix) && count < limit; iterator.Next() {
			item := iterator.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err != nil {
					return err
				}
				results = append(results, result)
				return nil
			})

			if err != nil {
				return err
			}

			count++
		}

		return nil
	})

	return results, err
}

// GetMetrics returns current crawler metrics
func (s *BadgerStorage) GetMetrics() (*domain.CrawlMetrics, error) {
	// Update URLs in DB count
	s.metrics.URLsInDB = s.countURLsInDB()
	s.metrics.LastUpdateTime = time.Now()

	// Calculate URLs per second
	elapsed := time.Since(s.metrics.StartTime).Seconds()
	if elapsed > 0 {
		s.metrics.URLsPerSecond = float64(s.metrics.URLsProcessed) / elapsed
	}

	return s.metrics, nil
}

// UpdateMetrics updates the metrics
func (s *BadgerStorage) UpdateMetrics(metrics *domain.CrawlMetrics) error {
	s.metrics = metrics
	return s.saveMetrics()
}

// countURLsInDB counts URLs in the database
func (s *BadgerStorage) countURLsInDB() int64 {
	var count int64

	s.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only count keys
		iterator := txn.NewIterator(opts)
		defer iterator.Close()

		prefix := []byte(URLPrefix)

		for iterator.Seek(prefix); iterator.ValidForPrefix(prefix); iterator.Next() {
			count++
		}

		return nil
	})

	return count
}

// loadMetrics loads metrics from database
func (s *BadgerStorage) loadMetrics() {
	s.urlDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(MetricsKey))
		if err != nil {
			return err // Metrics don't exist yet
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, s.metrics)
		})
	})
}

// saveMetrics saves metrics to database
func (s *BadgerStorage) saveMetrics() error {
	data, err := json.Marshal(s.metrics)
	if err != nil {
		return err
	}

	return s.urlDB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(MetricsKey), data)
	})
}

// startGC starts background garbage collection
func (s *BadgerStorage) startGC() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Run garbage collection
		s.urlDB.RunValueLogGC(0.5)
		s.resultsDB.RunValueLogGC(0.5)
	}
}

// Close closes the storage
func (s *BadgerStorage) Close() error {
	s.saveMetrics()

	if err := s.urlDB.Close(); err != nil {
		return err
	}

	return s.resultsDB.Close()
}

// GetMemoryUsageMB returns the estimated memory usage in MB
func (s *BadgerStorage) GetMemoryUsageMB() float64 {
	// Return the allocated memory limit as the databases will use up to this amount
	return s.allocatedMemoryMB
}

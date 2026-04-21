package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/benar-m/golamv2/internal/domain"
)

// FastFileStorage implements high-performance file-based storage
type FastFileStorage struct {
	resultsFile *os.File
	urlsFile    *os.File
	writer      *bufio.Writer
	urlWriter   *bufio.Writer
	mutex       sync.Mutex
	metrics     *domain.CrawlMetrics
}

// NewFastFileStorage creates a new file-based storage
func NewFastFileStorage(dataDir string) (*FastFileStorage, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	// Open results file in append mode
	resultsPath := filepath.Join(dataDir, "crawl_results.jsonl")
	resultsFile, err := os.OpenFile(resultsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open results file: %v", err)
	}

	// Open URLs file in append mode
	urlsPath := filepath.Join(dataDir, "crawl_urls.jsonl")
	urlsFile, err := os.OpenFile(urlsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		resultsFile.Close()
		return nil, fmt.Errorf("failed to open URLs file: %v", err)
	}

	return &FastFileStorage{
		resultsFile: resultsFile,
		urlsFile:    urlsFile,
		writer:      bufio.NewWriterSize(resultsFile, 64*1024), // 64KB buffer
		urlWriter:   bufio.NewWriterSize(urlsFile, 64*1024),    // 64KB buffer
		metrics: &domain.CrawlMetrics{
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
		},
	}, nil
}

// StoreResult stores a crawl result to file (FAST)
func (s *FastFileStorage) StoreResult(result domain.CrawlResult) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Write as JSON Lines format (one JSON object per line)
	if err := json.NewEncoder(s.writer).Encode(result); err != nil {
		return fmt.Errorf("failed to encode result: %v", err)
	}

	// Update metrics
	s.metrics.URLsProcessed++
	if len(result.Emails) > 0 {
		s.metrics.EmailsFound += int64(len(result.Emails))
	}
	if len(result.Keywords) > 0 {
		s.metrics.KeywordsFound += int64(len(result.Keywords))
	}

	return nil
}

// StoreURL stores a URL task to file (FAST)
func (s *FastFileStorage) StoreURL(task domain.URLTask) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Write as JSON Lines format
	return json.NewEncoder(s.urlWriter).Encode(task)
}

// Flush ensures all data is written to disk
func (s *FastFileStorage) Flush() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.writer.Flush(); err != nil {
		return err
	}
	if err := s.urlWriter.Flush(); err != nil {
		return err
	}

	// Force OS to write to disk
	if err := s.resultsFile.Sync(); err != nil {
		return err
	}
	return s.urlsFile.Sync()
}

// Close closes the storage
func (s *FastFileStorage) Close() error {
	s.Flush()

	if err := s.resultsFile.Close(); err != nil {
		return err
	}
	return s.urlsFile.Close()
}

// GetMetrics returns current metrics
func (s *FastFileStorage) GetMetrics() *domain.CrawlMetrics {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.metrics.LastUpdateTime = time.Now()
	return s.metrics
}

// Stub methods to satisfy interface (not needed for file storage)
func (s *FastFileStorage) GetURLs(limit int) ([]domain.URLTask, error) {
	return []domain.URLTask{}, nil
}

func (s *FastFileStorage) GetResults(limit int) ([]domain.CrawlResult, error) {
	return []domain.CrawlResult{}, nil
}

func (s *FastFileStorage) GetEmails(limit int) ([]string, error) {
	return []string{}, nil
}

func (s *FastFileStorage) GetKeywords(limit int) (map[string]int, error) {
	return map[string]int{}, nil
}

func (s *FastFileStorage) GetDeadLinks(limit int) ([]string, error) {
	return []string{}, nil
}

func (s *FastFileStorage) SearchResults(query string, limit int) ([]domain.CrawlResult, error) {
	return []domain.CrawlResult{}, nil
}

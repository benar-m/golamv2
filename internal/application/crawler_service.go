package application

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benar-m/golamv2/internal/domain"
	"github.com/benar-m/golamv2/internal/infrastructure"

	"golang.org/x/time/rate"
)

type CrawlerService struct {
	infra            *infrastructure.Infrastructure
	mode             domain.CrawlMode
	keywords         []string
	activeWorkers    int64
	httpClient       *http.Client
	rateLimiter      *rate.Limiter
	checkDeadDomains bool
}

func NewCrawlerService(infra *infrastructure.Infrastructure, mode domain.CrawlMode, keywords []string, checkDeadDomains bool) *CrawlerService {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 25,
		MaxConnsPerHost:     50,
		MaxIdleConns:        100,

		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       90 * time.Second,

		DisableCompression: false,
	}

	return &CrawlerService{
		infra:            infra,
		mode:             mode,
		keywords:         keywords,
		checkDeadDomains: checkDeadDomains,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
		rateLimiter: rate.NewLimiter(rate.Limit(200), 200),
	}
}

func (c *CrawlerService) StartCrawling(ctx context.Context, startURL string, maxWorkers, maxDepth int) error {
	startTask := domain.URLTask{
		URL:       startURL,
		Depth:     0,
		Timestamp: time.Now(),
		Retries:   0,
	}

	if err := c.infra.URLQueue.Push(startTask); err != nil {
		return fmt.Errorf("failed to add start URL to queue: %v", err)
	}

	// Add to Bloom filter
	c.infra.BloomFilter.Add(startURL)

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.worker(ctx, workerID, maxDepth)
		}(i)
	}

	// Start metrics updater
	go c.updateMetrics(ctx)

	// Wait for all workers to finish
	wg.Wait()

	return nil
}

func (c *CrawlerService) worker(ctx context.Context, workerID, maxDepth int) {
	defer atomic.AddInt64(&c.activeWorkers, -1)
	atomic.AddInt64(&c.activeWorkers, 1)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Try to get a URL from the queue
			task, err := c.infra.URLQueue.Pop()
			if err != nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			// Process the URL
			c.processURL(ctx, task, maxDepth)
		}
	}
}

func (c *CrawlerService) processURL(ctx context.Context, task domain.URLTask, maxDepth int) {
	startTime := time.Now()

	result := domain.CrawlResult{
		URL:         task.URL,
		ProcessedAt: startTime,
	}

	defer func() {
		result.ProcessTime = time.Since(startTime)
		c.infra.Storage.StoreResult(result)
		c.infra.Metrics.UpdateURLsProcessed(1)
	}()

	// Check robots.txt compliance incase we got ourselves explicitly blocked or rather forbidden
	if !c.infra.RobotsChecker.CanFetch("GolamV2-Crawler/1.0", task.URL) {
		result.Error = "blocked by robots.txt"
		return
	}

	// Respect crawl delay - DISABLED FOR PERFORMANCE
	// domain := domain.GetDomain(task.URL)
	// crawlDelay := c.infra.RobotsChecker.GetCrawlDelay("GolamV2-Crawler/1.0", domain)
	// if crawlDelay > 0 {
	//     time.Sleep(crawlDelay)
	// }

	// Rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		result.Error = "rate limit context cancelled"
		return
	}

	// Fetch the URL
	content, statusCode, err := c.fetchURL(task.URL)
	result.StatusCode = statusCode

	if err != nil {
		result.Error = err.Error()
		c.infra.Metrics.UpdateErrors(1)
		return
	}

	// Extract title
	result.Title = c.infra.ContentExtractor.ExtractTitle(content)

	// Extract data based on mode
	switch c.mode {
	case "email":
		result.Emails = c.infra.ContentExtractor.ExtractEmails(content)
		c.infra.Metrics.UpdateEmailsFound(int64(len(result.Emails)))

	case "keywords":
		result.Keywords = c.infra.ContentExtractor.ExtractKeywords(content, c.keywords)
		keywordCount := int64(0)
		for _, count := range result.Keywords {
			keywordCount += int64(count)
		}
		c.infra.Metrics.UpdateKeywordsFound(keywordCount)

	case "domains":
		links := c.infra.ContentExtractor.ExtractLinks(content, task.URL)
		result.DeadLinks, result.DeadDomains = c.infra.ContentExtractor.CheckDeadLinks(links, task.URL)
		c.infra.Metrics.UpdateLinksChecked(int64(len(links)))
		c.infra.Metrics.UpdateDeadLinksFound(int64(len(result.DeadLinks)))
		c.infra.Metrics.UpdateDeadDomainsFound(int64(len(result.DeadDomains)))

	case "all":
		// Extract everything - enable dead link checking if domains mode was requested
		result.Emails = c.infra.ContentExtractor.ExtractEmails(content)
		result.Keywords = c.infra.ContentExtractor.ExtractKeywords(content, c.keywords)

		// Check if domains mode was explicitly requested
		if c.shouldCheckDeadLinks() {
			links := c.infra.ContentExtractor.ExtractLinks(content, task.URL)
			result.DeadLinks, result.DeadDomains = c.infra.ContentExtractor.CheckDeadLinks(links, task.URL)
			c.infra.Metrics.UpdateLinksChecked(int64(len(links)))
			c.infra.Metrics.UpdateDeadLinksFound(int64(len(result.DeadLinks)))
			c.infra.Metrics.UpdateDeadDomainsFound(int64(len(result.DeadDomains)))
		} else {
			// Skip dead link checking for performance when not explicitly requested
			result.DeadLinks = []string{}   // Empty - no dead link checking
			result.DeadDomains = []string{} // Empty - no dead link checking
		}

		c.infra.Metrics.UpdateEmailsFound(int64(len(result.Emails)))
		keywordCount := int64(0)
		for _, count := range result.Keywords {
			keywordCount += int64(count)
		}
		c.infra.Metrics.UpdateKeywordsFound(keywordCount)
	}

	if task.Depth < maxDepth {
		newURLs := c.infra.ContentExtractor.ExtractLinks(content, task.URL)
		result.NewURLs = c.addNewURLs(newURLs, task.Depth+1)
	}
}

func (c *CrawlerService) fetchURL(url string) (string, int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", 0, err
	}

	req.Header.Set("User-Agent", "GolamV2-Crawler/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	// Check Content-Type header - only process HTML content for performance
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "text/html") &&
		!strings.Contains(strings.ToLower(contentType), "application/xhtml") {
		return "", resp.StatusCode, fmt.Errorf("skipped non-HTML content: %s", contentType)
	}

	limitedReader := io.LimitReader(resp.Body, 2*1024*1024)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(content), resp.StatusCode, nil
}

func (c *CrawlerService) addNewURLs(urls []string, depth int) []string {
	var newURLs []string

	for _, url := range urls {
		// Check if URL is valid
		if !domain.IsValidURL(url) {
			continue
		}

		// Check Bloom filter for duplicates
		if c.infra.BloomFilter.Test(url) {
			continue // Likely already seen by bloom
		}

		// Add to Bloom filter
		c.infra.BloomFilter.Add(url)

		// Create URL task
		task := domain.URLTask{
			URL:       url,
			Depth:     depth,
			Timestamp: time.Now(),
			Retries:   0,
		}

		// Try to add to queue, if full, store in database
		if err := c.infra.URLQueue.Push(task); err != nil {
			c.infra.Storage.StoreURL(task)
		}

		newURLs = append(newURLs, url)
	}

	return newURLs
}

func (c *CrawlerService) updateMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update active workers count
			c.infra.Metrics.UpdateActiveWorkers(int(atomic.LoadInt64(&c.activeWorkers)))

			// Update queue size
			c.infra.Metrics.UpdateURLsInQueue(int64(c.infra.URLQueue.Size()))

			if storageMetrics, err := c.infra.Storage.GetMetrics(); err == nil {
				c.infra.Metrics.UpdateURLsInDB(storageMetrics.URLsInDB)
			}
		}
	}
}

func (c *CrawlerService) shouldCheckDeadLinks() bool {
	return c.checkDeadDomains || c.mode == "domains"
}

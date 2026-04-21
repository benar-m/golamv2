package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benar-m/golamv2/internal/application"
	"github.com/benar-m/golamv2/internal/domain"
	"github.com/benar-m/golamv2/internal/infrastructure"
	"github.com/benar-m/golamv2/internal/interfaces"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "golamv2",
		Short: "GolamV2 - Super efficient web crawler",
		Long:  `GolamV2 is a high-performance, low-memory web crawler with multiple hunting modes.`,
		Run:   runCrawler,
	}

	// Flags
	emailMode     bool
	domainMode    bool
	keywords      []string
	maxWorkers    int
	maxMemoryMB   int
	startURL      string
	maxDepth      int
	dashboardPort int
)

func init() {
	rootCmd.Flags().BoolVar(&emailMode, "email", false, "Hunt for email addresses")
	rootCmd.Flags().BoolVar(&domainMode, "domains", false, "Hunt for dead URLs and domains")
	rootCmd.Flags().StringSliceVar(&keywords, "keywords", []string{}, "Hunt for specific keywords (comma-separated)")
	rootCmd.Flags().IntVar(&maxWorkers, "workers", 50, "Maximum number of concurrent workers")
	rootCmd.Flags().IntVar(&maxMemoryMB, "memory", 500, "Maximum memory usage in MB")
	rootCmd.Flags().StringVar(&startURL, "url", "", "Starting URL to crawl (required)")
	rootCmd.Flags().IntVar(&maxDepth, "depth", 5, "Maximum crawling depth")
	rootCmd.Flags().IntVar(&dashboardPort, "dashboard", 8080, "Dashboard port")

	rootCmd.MarkFlagRequired("url")
}

func Execute() error {
	return rootCmd.Execute()
}

func runCrawler(cmd *cobra.Command, args []string) {
	// Validate flags
	if !emailMode && !domainMode && len(keywords) == 0 {
		log.Fatal("At least one hunting mode must be specified: --email, --domains, or --keywords")
	}

	// Determine crawl mode
	mode := determineCrawlMode()

	// Initialize infrastructure
	infra, err := infrastructure.NewInfrastructure(maxMemoryMB)
	if err != nil {
		log.Fatalf("Failed to initialize infrastructure: %v", err)
	}
	defer infra.Close()

	// Create application service
	app := application.NewCrawlerService(infra, domain.CrawlMode(mode), keywords, domainMode)

	// Start dashboard with storage and URL queue access
	dashboard := interfaces.NewDashboard(infra.GetMetrics(), infra.Storage, infra.URLQueue, dashboardPort)
	go dashboard.Start()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		cancel()
	}()

	// Start crawler
	fmt.Printf("Starting GolamV2 crawler...\n")
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Start URL: %s\n", startURL)
	fmt.Printf("Max Workers: %d\n", maxWorkers)
	fmt.Printf("Max Memory: %dMB\n", maxMemoryMB)
	fmt.Printf("Dashboard: http://localhost:%d\n", dashboardPort)

	err = app.StartCrawling(ctx, startURL, maxWorkers, maxDepth)
	if err != nil {
		log.Fatalf("Crawling failed: %v", err)
	}

	// Wait a lil before cleanup
	time.Sleep(2 * time.Second)
	fmt.Println("Crawling completed!")
}

func determineCrawlMode() string {
	// Multi-mode support: collect all enabled modes
	var modes []string

	if emailMode {
		modes = append(modes, "email")
	}
	if len(keywords) > 0 {
		modes = append(modes, "keywords")
	}
	if domainMode {
		modes = append(modes, "domains")
	}

	if len(modes) == 0 {
		log.Fatal("At least one hunting mode must be specified: --email, --domains, or --keywords")
	}

	// If multiple modes, use "all" but i've configured the "all" mode to avoid dead link checking, to enable dead link checking, explicitly use --domains
	if len(modes) > 1 {
		return "all"
	}

	return modes[0]
}

package cmd

//BADGERDB LACKS IN EXPLORER TOOLS,THIS WAS A CUSTOM IMPLEMENTATION FOR GOLAMV2 THAT WORKED FOR MY USECASE. BY "FOR GOLAMV2" I MEAN IT WAS DESIGNED TO WORK WITH GOLAMV2'S DATA STRUCTURES AND SCHEMA, NOT A GENERIC EXPLORER TOOL.
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/benar-m/golamv2/internal/domain"

	"github.com/dgraph-io/badger/v4"
	"github.com/spf13/cobra"
)

const (
	URLPrefix    = "url:"
	ResultPrefix = "result:"
	MetricsKey   = "metrics"
)

var (
	dataPath   string
	outputFile string
)

// exploreCmd - the explore command
var exploreCmd = &cobra.Command{
	Use:   "explore",
	Short: "Interactive data explorer for GolamV2 databases",
	Long: `An interactive CLI tool to explore GolamV2 crawl data.
	
Browse URLs, results, search data, view statistics, and export findings.
The tool provides an interactive shell for data exploration.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runExplore(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(exploreCmd)
	exploreCmd.Flags().StringVarP(&dataPath, "data", "d", "golamv2_data", "Path to GolamV2 data directory")
	exploreCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file for exports (optional)")
}

type Explorer struct {
	urlDB     *badger.DB
	resultsDB *badger.DB
	dataPath  string
	scanner   *bufio.Scanner
}

func runExplore() error {
	explorer, err := NewExplorer(dataPath)
	if err != nil {
		return fmt.Errorf("failed to initialize explorer: %v", err)
	}
	defer explorer.Close()

	explorer.printBanner()
	explorer.runInteractiveShell()
	return nil
}

func NewExplorer(dbPath string) (*Explorer, error) {
	// Check if data directory exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("data directory not found: %s", dbPath)
	}

	// Open URLs database
	urlOpts := badger.DefaultOptions(filepath.Join(dbPath, "urls"))
	urlOpts.Logger = nil // Disable logging
	urlDB, err := badger.Open(urlOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open URLs database: %v", err)
	}

	// Open results database
	resultsOpts := badger.DefaultOptions(filepath.Join(dbPath, "finds"))
	resultsOpts.Logger = nil // Disable logging
	resultsDB, err := badger.Open(resultsOpts)
	if err != nil {
		urlDB.Close()
		return nil, fmt.Errorf("failed to open results database: %v", err)
	}

	return &Explorer{
		urlDB:     urlDB,
		resultsDB: resultsDB,
		dataPath:  dbPath,
		scanner:   bufio.NewScanner(os.Stdin),
	}, nil
}

func (e *Explorer) Close() {
	if e.urlDB != nil {
		e.urlDB.Close()
	}
	if e.resultsDB != nil {
		e.resultsDB.Close()
	}
}

func (e *Explorer) printBanner() {
	fmt.Println("🕸️  GolamV2 Data Explorer")
	fmt.Println("========================")
	fmt.Println("Interactive tool to explore crawl data")
	fmt.Printf("Data path: %s\n", e.dataPath)
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  help          - Show this help")
	fmt.Println("  stats         - Show database statistics")
	fmt.Println("  urls [limit]  - List URLs (default: 10)")
	fmt.Println("  results [limit] - List results (default: 10)")
	fmt.Println("  search <term> - Search in results")
	fmt.Println("  emails [limit] - Show found emails")
	fmt.Println("  keywords [limit] - Show found keywords")
	fmt.Println("  deadlinks [limit] - Show dead links")
	fmt.Println("  export <type> - Export data (urls|results|emails|keywords)")
	fmt.Println("  raw <key>     - Show raw data for specific key")
	fmt.Println("  analyze       - Detailed analysis of crawl data")
	fmt.Println("  timeline      - Show crawling timeline")
	fmt.Println("  domains       - Show domain statistics")
	fmt.Println("  clear         - Clear screen")
	fmt.Println("  quit/exit     - Exit explorer")
	fmt.Println()
}

func (e *Explorer) runInteractiveShell() {
	for {
		fmt.Print("golamv2> ")
		if !e.scanner.Scan() {
			break
		}

		input := strings.TrimSpace(e.scanner.Text())
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := strings.ToLower(parts[0])

		switch command {
		case "help", "h":
			e.printBanner()
		case "stats":
			e.showStats()
		case "urls":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			e.listURLs(limit)
		case "results":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			e.listResults(limit)
		case "search":
			if len(parts) < 2 {
				fmt.Println("Usage: search <term>")
				continue
			}
			term := strings.Join(parts[1:], " ")
			e.searchResults(term)
		case "emails":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			e.showEmails(limit)
		case "keywords":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			e.showKeywords(limit)
		case "deadlinks":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			e.showDeadLinks(limit)
		case "export":
			if len(parts) < 2 {
				fmt.Println("Usage: export <type> (urls|results|emails|keywords)")
				continue
			}
			e.exportData(parts[1])
		case "raw":
			if len(parts) < 2 {
				fmt.Println("Usage: raw <key>")
				continue
			}
			key := strings.Join(parts[1:], " ")
			e.showRawData(key)
		case "analyze":
			e.analyzeData()
		case "timeline":
			e.showTimeline()
		case "domains":
			e.showDomainStats()
		case "clear":
			fmt.Print("\033[2J\033[H")
		case "quit", "exit", "q":
			fmt.Println("Goodbye! ")
			return
		default:
			fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", command)
		}
	}
}

func (e *Explorer) showStats() {
	fmt.Println("\n Database Statistics")
	fmt.Println("=====================")

	// URLs database stats
	urlCount := 0
	e.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(URLPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			urlCount++
		}
		return nil
	})

	// Results database stats
	resultCount := 0
	emailCount := 0
	keywordCount := 0
	deadLinkCount := 0
	errorCount := 0

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					resultCount++
					emailCount += len(result.Emails)
					keywordCount += len(result.Keywords)
					deadLinkCount += len(result.DeadLinks)
					if result.Error != "" {
						errorCount++
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	fmt.Printf("URLs in database:      %d\n", urlCount)
	fmt.Printf("Results in database:   %d\n", resultCount)
	fmt.Printf("Emails found:          %d\n", emailCount)
	fmt.Printf("Keywords found:        %d\n", keywordCount)
	fmt.Printf("Dead links found:      %d\n", deadLinkCount)
	fmt.Printf("Errors encountered:    %d\n", errorCount)

	if resultCount > 0 {
		fmt.Printf("Success rate:          %.1f%%\n", float64(resultCount-errorCount)/float64(resultCount)*100)
	}
	fmt.Println()
}

func (e *Explorer) listURLs(limit int) {
	fmt.Printf("\n URLs (showing %d):\n", limit)
	fmt.Println("====================")

	count := 0
	e.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(URLPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix) && count < limit; it.Next() {
			item := it.Item()
			key := item.Key()
			url := string(key[len(URLPrefix):])

			err := item.Value(func(val []byte) error {
				var task domain.URLTask
				if err := json.Unmarshal(val, &task); err == nil {
					fmt.Printf("%d. %s\n", count+1, url)
					fmt.Printf("   Depth: %d, Retries: %d\n", task.Depth, task.Retries)
					fmt.Printf("   Added: %s\n", task.Timestamp.Format("2006-01-02 15:04:05"))
				}
				return nil
			})
			if err != nil {
				return err
			}
			count++
			fmt.Println()
		}
		return nil
	})

	if count == 0 {
		fmt.Println("No URLs found in database.")
	}
	fmt.Println()
}

func (e *Explorer) listResults(limit int) {
	fmt.Printf("\nResults (showing %d):\n", limit)
	fmt.Println("========================")

	count := 0
	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix) && count < limit; it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					fmt.Printf("%d. %s\n", count+1, result.URL)
					fmt.Printf("   Status: %d, Title: %s\n", result.StatusCode, truncateString(result.Title, 50))
					fmt.Printf("   Processed: %s\n", result.ProcessedAt.Format("2006-01-02 15:04:05"))
					fmt.Printf("   Process Time: %v\n", result.ProcessTime)

					if len(result.Emails) > 0 {
						fmt.Printf("   Emails: %d found\n", len(result.Emails))
					}
					if len(result.Keywords) > 0 {
						fmt.Printf("   Keywords: %d found\n", len(result.Keywords))
					}
					if len(result.DeadLinks) > 0 {
						fmt.Printf("   Dead Links: %d found\n", len(result.DeadLinks))
					}
					if result.Error != "" {
						fmt.Printf("   Error: %s\n", truncateString(result.Error, 100))
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			count++
			fmt.Println()
		}
		return nil
	})

	if count == 0 {
		fmt.Println("No results found in database.")
	}
	fmt.Println()
}

func (e *Explorer) searchResults(term string) {
	fmt.Printf("\n Search results for '%s':\n", term)
	fmt.Println("============================")

	lowerTerm := strings.ToLower(term)
	count := 0

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					// Search in URL, title, emails, and keywords
					found := false
					if strings.Contains(strings.ToLower(result.URL), lowerTerm) ||
						strings.Contains(strings.ToLower(result.Title), lowerTerm) {
						found = true
					}

					// Search in emails
					for _, email := range result.Emails {
						if strings.Contains(strings.ToLower(email), lowerTerm) {
							found = true
							break
						}
					}

					// Search in keywords
					for keyword := range result.Keywords {
						if strings.Contains(strings.ToLower(keyword), lowerTerm) {
							found = true
							break
						}
					}

					if found {
						count++
						fmt.Printf("%d. %s\n", count, result.URL)
						fmt.Printf("   Title: %s\n", truncateString(result.Title, 60))
						fmt.Printf("   Processed: %s\n", result.ProcessedAt.Format("2006-01-02 15:04:05"))

						// Show matching emails
						for _, email := range result.Emails {
							if strings.Contains(strings.ToLower(email), lowerTerm) {
								fmt.Printf("    Email: %s\n", email)
							}
						}

						// Show matching keywords
						for keyword, freq := range result.Keywords {
							if strings.Contains(strings.ToLower(keyword), lowerTerm) {
								fmt.Printf("    Keyword: %s (%d times)\n", keyword, freq)
							}
						}
						fmt.Println()
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if count == 0 {
		fmt.Printf("No results found for '%s'.\n", term)
	} else {
		fmt.Printf("Found %d matching results.\n", count)
	}
	fmt.Println()
}

func (e *Explorer) showEmails(limit int) {
	fmt.Printf("\n Found Emails (showing %d):\n", limit)
	fmt.Println("=============================")

	emailMap := make(map[string][]string) // email -> list of URLs where found
	count := 0

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					for _, email := range result.Emails {
						emailMap[email] = append(emailMap[email], result.URL)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	for email, urls := range emailMap {
		if count >= limit {
			break
		}
		count++
		fmt.Printf("%d. %s\n", count, email)
		fmt.Printf("   Found on %d page(s):\n", len(urls))
		for i, url := range urls {
			if i < 3 { // Show first 3 URLs
				fmt.Printf("   - %s\n", url)
			} else if i == 3 {
				fmt.Printf("   - ... and %d more\n", len(urls)-3)
				break
			}
		}
		fmt.Println()
	}

	if count == 0 {
		fmt.Println("No emails found in database.")
	}
	fmt.Println()
}

func (e *Explorer) showKeywords(limit int) {
	fmt.Printf("\nFound Keywords (showing %d):\n", limit)
	fmt.Println("==============================")

	keywordMap := make(map[string]int)       // keyword -> total frequency
	keywordURLs := make(map[string][]string) // keyword -> URLs where found

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					for keyword, freq := range result.Keywords {
						keywordMap[keyword] += freq
						keywordURLs[keyword] = append(keywordURLs[keyword], result.URL)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Sort keywords by frequency (simple approach)
	count := 0
	for keyword, totalFreq := range keywordMap {
		if count >= limit {
			break
		}
		count++
		urls := keywordURLs[keyword]
		fmt.Printf("%d. %s (found %d times on %d pages)\n", count, keyword, totalFreq, len(urls))
		for i, url := range urls {
			if i < 2 { // Show first 2 URLs
				fmt.Printf("   - %s\n", url)
			} else if i == 2 {
				fmt.Printf("   - ... and %d more\n", len(urls)-2)
				break
			}
		}
		fmt.Println()
	}

	if count == 0 {
		fmt.Println("No keywords found in database.")
	}
	fmt.Println()
}

func (e *Explorer) showDeadLinks(limit int) {
	fmt.Printf("\n Dead Links (showing %d):\n", limit)
	fmt.Println("===========================")

	deadLinkMap := make(map[string][]string) // dead link -> list of URLs where found
	count := 0

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					for _, deadLink := range result.DeadLinks {
						deadLinkMap[deadLink] = append(deadLinkMap[deadLink], result.URL)
					}
					for _, deadDomain := range result.DeadDomains {
						deadLinkMap[deadDomain] = append(deadLinkMap[deadDomain], result.URL)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	for deadLink, urls := range deadLinkMap {
		if count >= limit {
			break
		}
		count++
		fmt.Printf("%d. %s\n", count, deadLink)
		fmt.Printf("   Found on %d page(s):\n", len(urls))
		for i, url := range urls {
			if i < 3 { // Show first 3 URLs
				fmt.Printf("   - %s\n", url)
			} else if i == 3 {
				fmt.Printf("   - ... and %d more\n", len(urls)-3)
				break
			}
		}
		fmt.Println()
	}

	if count == 0 {
		fmt.Println("No dead links found in database.")
	}
	fmt.Println()
}

func (e *Explorer) exportData(dataType string) {
	filename := fmt.Sprintf("golamv2_%s_export_%s.json", dataType, time.Now().Format("20060102_150405"))
	if outputFile != "" {
		filename = outputFile
	}

	fmt.Printf("Exporting %s data to %s...\n", dataType, filename)

	var data interface{}
	var err error

	switch strings.ToLower(dataType) {
	case "urls":
		data, err = e.exportURLs()
	case "results":
		data, err = e.exportResults()
	case "emails":
		data, err = e.exportEmails()
	case "keywords":
		data, err = e.exportKeywords()
	default:
		fmt.Printf("Unknown export type: %s. Available: urls, results, emails, keywords\n", dataType)
		return
	}

	if err != nil {
		fmt.Printf("Error exporting data: %v\n", err)
		return
	}

	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Printf("Error writing data: %v\n", err)
		return
	}

	fmt.Printf("Successfully exported to %s\n", filename)
}

func (e *Explorer) exportURLs() ([]domain.URLTask, error) {
	var urls []domain.URLTask

	err := e.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(URLPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var task domain.URLTask
				if err := json.Unmarshal(val, &task); err == nil {
					urls = append(urls, task)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return urls, err
}

func (e *Explorer) exportResults() ([]domain.CrawlResult, error) {
	var results []domain.CrawlResult

	err := e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					results = append(results, result)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return results, err
}

func (e *Explorer) exportEmails() (map[string][]string, error) {
	emailMap := make(map[string][]string)

	err := e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					for _, email := range result.Emails {
						emailMap[email] = append(emailMap[email], result.URL)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return emailMap, err
}

func (e *Explorer) exportKeywords() (map[string]interface{}, error) {
	keywordData := make(map[string]interface{})
	keywordFreq := make(map[string]int)
	keywordURLs := make(map[string][]string)

	err := e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					for keyword, freq := range result.Keywords {
						keywordFreq[keyword] += freq
						keywordURLs[keyword] = append(keywordURLs[keyword], result.URL)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Combine frequency and URL data
	for keyword, freq := range keywordFreq {
		keywordData[keyword] = map[string]interface{}{
			"frequency": freq,
			"urls":      keywordURLs[keyword],
		}
	}

	return keywordData, err
}

func (e *Explorer) showRawData(key string) {
	fmt.Printf("\n Raw Data for Key: %s\n", key)
	fmt.Println("============================")

	found := false

	// Try URLs database first
	e.urlDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == nil {
			found = true
			item.Value(func(val []byte) error {
				fmt.Println("Database: URLs")
				fmt.Printf("Raw Value: %s\n", string(val))

				// Try to parse as URLTask
				var task domain.URLTask
				if err := json.Unmarshal(val, &task); err == nil {
					fmt.Println("\nParsed as URLTask:")
					prettyJSON, _ := json.MarshalIndent(task, "", "  ")
					fmt.Println(string(prettyJSON))
				}
				return nil
			})
		}
		return nil
	})

	// Try results database if not found in URLs
	if !found {
		e.resultsDB.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(key))
			if err == nil {
				found = true
				item.Value(func(val []byte) error {
					fmt.Println("Database: Results")
					fmt.Printf("Raw Value: %s\n", string(val))

					// Try to parse as CrawlResult
					var result domain.CrawlResult
					if err := json.Unmarshal(val, &result); err == nil {
						fmt.Println("\nParsed as CrawlResult:")
						prettyJSON, _ := json.MarshalIndent(result, "", "  ")
						fmt.Println(string(prettyJSON))
					}
					return nil
				})
			}
			return nil
		})
	}

	if !found {
		fmt.Printf("Key '%s' not found in any database.\n", key)
		fmt.Println("\nTip: Use 'urls' or 'results' commands to see available keys.")
	}
	fmt.Println()
}

func (e *Explorer) analyzeData() {
	fmt.Println("\n Detailed Data Analysis")
	fmt.Println("=========================")

	// Collect comprehensive statistics
	stats := struct {
		TotalURLs       int
		TotalResults    int
		UniqueEmails    map[string]bool
		UniqueKeywords  map[string]int
		DomainStats     map[string]int
		StatusCodes     map[int]int
		ErrorAnalysis   map[string]int
		ProcessingTimes []time.Duration
		CrawlDepths     map[int]int
	}{
		UniqueEmails:   make(map[string]bool),
		UniqueKeywords: make(map[string]int),
		DomainStats:    make(map[string]int),
		StatusCodes:    make(map[int]int),
		ErrorAnalysis:  make(map[string]int),
		CrawlDepths:    make(map[int]int),
	}

	// Analyze URLs
	e.urlDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(URLPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var task domain.URLTask
				if err := json.Unmarshal(val, &task); err == nil {
					stats.TotalURLs++
					stats.CrawlDepths[task.Depth]++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Analyze results
	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					stats.TotalResults++
					stats.StatusCodes[result.StatusCode]++
					stats.ProcessingTimes = append(stats.ProcessingTimes, result.ProcessTime)

					// Extract domain
					if domain := extractDomain(result.URL); domain != "" {
						stats.DomainStats[domain]++
					}

					// Collect emails
					for _, email := range result.Emails {
						stats.UniqueEmails[email] = true
					}

					// Collect keywords
					for keyword, freq := range result.Keywords {
						stats.UniqueKeywords[keyword] += freq
					}

					// Analyze errors
					if result.Error != "" {
						errorType := categorizeError(result.Error)
						stats.ErrorAnalysis[errorType]++
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Display analysis
	fmt.Printf("Total URLs in queue: %d\n", stats.TotalURLs)
	fmt.Printf("Total processed results: %d\n", stats.TotalResults)
	fmt.Printf("Unique emails found: %d\n", len(stats.UniqueEmails))
	fmt.Printf("Unique keywords found: %d\n", len(stats.UniqueKeywords))
	fmt.Printf("Unique domains crawled: %d\n", len(stats.DomainStats))

	// Show top domains
	fmt.Println("\nTop 10 Domains:")
	topDomains := getTopEntries(stats.DomainStats, 10)
	for i, entry := range topDomains {
		fmt.Printf("%d. %s (%d pages)\n", i+1, entry.Key, entry.Value)
	}

	// Show status codes
	fmt.Println("\nHTTP Status Codes:")
	for code, count := range stats.StatusCodes {
		fmt.Printf("%d: %d responses\n", code, count)
	}

	// Show error analysis
	if len(stats.ErrorAnalysis) > 0 {
		fmt.Println("\nError Analysis:")
		for errorType, count := range stats.ErrorAnalysis {
			fmt.Printf("%s: %d occurrences\n", errorType, count)
		}
	}

	// Show processing time statistics
	if len(stats.ProcessingTimes) > 0 {
		var totalTime time.Duration
		minTime := stats.ProcessingTimes[0]
		maxTime := stats.ProcessingTimes[0]

		for _, t := range stats.ProcessingTimes {
			totalTime += t
			if t < minTime {
				minTime = t
			}
			if t > maxTime {
				maxTime = t
			}
		}

		avgTime := totalTime / time.Duration(len(stats.ProcessingTimes))
		fmt.Println("\nProcessing Times:")
		fmt.Printf("Average: %v\n", avgTime)
		fmt.Printf("Minimum: %v\n", minTime)
		fmt.Printf("Maximum: %v\n", maxTime)
	}

	// Show crawl depth distribution
	fmt.Println("\nCrawl Depth Distribution:")
	for depth, count := range stats.CrawlDepths {
		fmt.Printf("Depth %d: %d URLs\n", depth, count)
	}

	fmt.Println()
}

func (e *Explorer) showTimeline() {
	fmt.Println("\n Crawling Timeline")
	fmt.Println("===================")

	timeMap := make(map[string]int) // hour -> count
	var firstTime, lastTime time.Time
	resultCount := 0

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					resultCount++

					if resultCount == 1 {
						firstTime = result.ProcessedAt
						lastTime = result.ProcessedAt
					} else {
						if result.ProcessedAt.Before(firstTime) {
							firstTime = result.ProcessedAt
						}
						if result.ProcessedAt.After(lastTime) {
							lastTime = result.ProcessedAt
						}
					}

					hourKey := result.ProcessedAt.Format("2006-01-02 15:00")
					timeMap[hourKey]++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if resultCount == 0 {
		fmt.Println("No results found for timeline analysis.")
		return
	}

	fmt.Printf("Crawling Period: %s to %s\n", firstTime.Format("2006-01-02 15:04:05"), lastTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total Duration: %v\n", lastTime.Sub(firstTime))
	fmt.Printf("Total Results: %d\n", resultCount)

	if len(timeMap) > 0 {
		fmt.Println("\nActivity by Hour:")
		// Sort and display timeline (simplified)
		count := 0
		for timeKey, pages := range timeMap {
			if count >= 20 { // Show first 20 entries
				fmt.Printf("... and %d more time periods\n", len(timeMap)-20)
				break
			}
			fmt.Printf("%s: %d pages processed\n", timeKey, pages)
			count++
		}
	}

	fmt.Println()
}

func (e *Explorer) showDomainStats() {
	fmt.Println("\n Domain Statistics")
	fmt.Println("===================")

	domainStats := make(map[string]struct {
		PageCount   int
		EmailCount  int
		ErrorCount  int
		AvgRespTime time.Duration
		TotalTime   time.Duration
	})

	e.resultsDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(ResultPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var result domain.CrawlResult
				if err := json.Unmarshal(val, &result); err == nil {
					domain := extractDomain(result.URL)
					if domain == "" {
						return nil
					}

					stats := domainStats[domain]
					stats.PageCount++
					stats.EmailCount += len(result.Emails)
					stats.TotalTime += result.ProcessTime
					if result.Error != "" {
						stats.ErrorCount++
					}
					domainStats[domain] = stats
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	// Calculate averages and display
	fmt.Printf("Total Domains: %d\n\n", len(domainStats))

	count := 0
	for domain, stats := range domainStats {
		if count >= 15 { // Show top 15 domains
			fmt.Printf("... and %d more domains\n", len(domainStats)-15)
			break
		}
		count++

		avgTime := time.Duration(0)
		if stats.PageCount > 0 {
			avgTime = stats.TotalTime / time.Duration(stats.PageCount)
		}

		successRate := float64(stats.PageCount-stats.ErrorCount) / float64(stats.PageCount) * 100

		fmt.Printf("%d. %s\n", count, domain)
		fmt.Printf("   Pages: %d, Emails: %d, Errors: %d\n", stats.PageCount, stats.EmailCount, stats.ErrorCount)
		fmt.Printf("   Success Rate: %.1f%%, Avg Response Time: %v\n", successRate, avgTime)
		fmt.Println()
	}
}

// Helper functions

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

func extractDomain(url string) string {
	// Simple domain extraction
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func categorizeError(errorMsg string) string {
	errorMsg = strings.ToLower(errorMsg)

	if strings.Contains(errorMsg, "timeout") {
		return "Timeout"
	} else if strings.Contains(errorMsg, "connection") {
		return "Connection Error"
	} else if strings.Contains(errorMsg, "404") || strings.Contains(errorMsg, "not found") {
		return "Not Found (404)"
	} else if strings.Contains(errorMsg, "403") || strings.Contains(errorMsg, "forbidden") {
		return "Forbidden (403)"
	} else if strings.Contains(errorMsg, "500") || strings.Contains(errorMsg, "internal server") {
		return "Server Error (5xx)"
	} else if strings.Contains(errorMsg, "dns") {
		return "DNS Error"
	} else {
		return "Other"
	}
}

type KeyValuePair struct {
	Key   string
	Value int
}

func getTopEntries(m map[string]int, limit int) []KeyValuePair {
	var pairs []KeyValuePair
	for k, v := range m {
		pairs = append(pairs, KeyValuePair{k, v})
	}

	// Simple bubble sort for top entries (good enough for small datasets)
	for i := 0; i < len(pairs)-1; i++ {
		for j := 0; j < len(pairs)-i-1; j++ {
			if pairs[j].Value < pairs[j+1].Value {
				pairs[j], pairs[j+1] = pairs[j+1], pairs[j]
			}
		}
	}

	if len(pairs) > limit {
		pairs = pairs[:limit]
	}

	return pairs
}

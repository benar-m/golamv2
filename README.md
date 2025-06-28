# GolamV2 - A LightWeight Web Crawler for Emails/Keywords/Dead Backlinks/Dead Domains

GolamV2 is a low-memory web crawler designed for maximum throughput in resource-constrained environments. It supports multiple hunting modes including email extraction, keyword searching, and dead link detection. It is a rewrite of the Python Version Gollum Spyder [here](https://github.com/nobrainghost/Keyword-Web-Crawler). Includes a Custom Interactive CLI Explore for its BadgerDB database

![Screenshot From 2025-06-10 15-25-08](https://github.com/user-attachments/assets/6725d0d5-1fc4-4713-8aed-8ddc9c530e70)

## Features

- **Multi-Purpose Crawling**: Email hunting, keyword searching, dead link detection
- **Memory Efficiency**: Can run with decent through put on low resource environments
- **Robots.txt Compliant**: Respects robots.txt and crawl delays
- **Real-time Dashboard**: Web-based monitoring interface
- **Interactive CLI Explorer**: Comprehensive data exploration and analysis tool
- **Clean Architecture**: Modular, maintainable codebase
- **Efficient Storage**: BadgerDB for persistent storage
- **Bloom Filter**: Memory-efficient duplicate URL detection
- **Priority Queue**: Smart URL queuing with database fallback

## Architecture

### Core Components

1. **URL Queue**: Priority-based queue (100k URLs limit) with automatic database refilling and spilling
2. **Bloom Filter**: To dedupe
3. **Storage Layer**: BadgerDB for persistent URL and result storage
4. **Worker Pool**: Configurable concurrent workers
5. **Content Extractor**:
6. **Robots Checker**: Compliant robots.txt parsing and enforcement. Also parses sitemaps
7. **Dashboard**: Real-time web interface for monitoring

## Installation

```bash
# Clone the repository
git clone https://github.com/nobrainghost/golamv2
cd GolamV2

# Install dependencies
go mod tidy

# Build the application
go build -o golamv2 main.go
```

## Usage

### Basic Email Hunting
```bash
./golamv2 --email --url https://example.com --workers 25
```

### Keyword Searching
```bash
./golamv2 --keywords "password,login,admin" --url https://example.com --workers 30
```

### Dead Link Detection
```bash
./golamv2 --domains --url https://example.com --workers 20
```

### All-in-One Mode
```bash
./golamv2 --email --domains --keywords "smeagol,ring" --url https://example.com --workers 40
```

### Data Exploration
```bash
# Explore crawl data interactively
./golamv2 explore

# Explore with custom data directory
./golamv2 explore --data /path/to/data
```

### Advanced Options
```bash
./golamv2 \
  --email \
  --url https://example.com \
  --workers 50 \
  --memory 400 \
  --depth 5 \
  --dashboard 8080
```

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--email` | Hunt for email addresses | false |
| `--domains` | Hunt for dead URLs and domains | false |
| `--keywords` | Hunt for specific keywords (comma-separated) | [] |
| `--url` | Starting URL to crawl (required) | - |
| `--workers` | Maximum number of concurrent workers | 50 |
| `--memory` | Maximum memory usage in MB | 500 |
| `--depth` | Maximum crawling depth | 5 |
| `--dashboard` | Dashboard port | 8080 |

## Dashboard

Access the real-time dashboard at `http://localhost:8080` (or your specified port). The paths /db currently dont work

### Dashboard Features

- **Real-time Metrics**: Live updates via a WebSocket
- **Performance Monitoring**: URLs/second, memory usage, uptime
- **Queue Status**: URLs in queue, database, active workers
- **Findings Summary**: Emails, keywords, dead links found
- **Success Rate**: Error tracking and success percentage


## CLI Data Explorer

GolamV2 includes an interactive CLI tool for exploring and analyzing crawl data stored in its BadgerDB databases.

### Starting the Explorer

```bash
# Use default data directory (golamv2_data)
./golamv2 explore

# Specify custom data directory
./golamv2 explore --data /path/to/data

# With output file for exports
./golamv2 explore --output results.json
```

### Available Commands

| Command | Description | Example |
|---------|-------------|---------|
| `help` | Show all available commands | `help` |
| `stats` | Display database statistics | `stats` |
| `urls [limit]` | List URLs (default: 10) | `urls 20` |
| `results [limit]` | List crawl results (default: 10) | `results 50` |
| `search <term>` | Search in results content | `search "admin panel"` |
| `emails [limit]` | Show found emails | `emails 25` |
| `keywords [limit]` | Show found keywords | `keywords 15` |
| `deadlinks [limit]` | Show dead links found | `deadlinks 30` |
| `export <type>` | Export data to JSON | `export emails` |
| `raw <key>` | Show raw data for specific key | `raw url:example.com` |
| `analyze` | Detailed data analysis | `analyze` |
| `timeline` | Show crawling timeline | `timeline` |
| `domains` | Show domain statistics | `domains` |
| `clear` | Clear terminal screen | `clear` |
| `quit/exit` | Exit explorer | `quit` |

### Explorer Features

#### Data Search and Filtering
- Full-text search across all results
- Search in titles, content, emails, and keywords
- Filter by status, domain, or content type

#### Export Capabilities
- Export URLs, results, emails, or keywords to JSON
- Configurable output files
- Data formatting for further analysis
##NOTE : NOT FULLY TESTED

#### Advanced Analysis
- Domain-based statistics and analysis
- Timeline visualization of crawling activity
- Success rate analysis by domain
- Error categorization and reporting
- Performance metrics and trends

### Example Explorer Session

```bash
$ ./golamv2 explore
🕸️  GolamV2 Data Explorer
========================
Interactive tool to explore crawl data
Data path: golamv2_data

golamv2> stats
 Database Statistics
=====================
URLs in database:      2,767
Results in database:   37,635
Emails found:          118,613
Keywords found:        1,258
Dead links found:      20,422
Errors encountered:    226
Success rate:          99.4%

golamv2> search "login"
 Search Results for "login":
=============================
Found 45 results containing "login"
- example.com/admin - Admin Login Portal
- test.org/user - User Login Page
...

golamv2> export emails
 Exporting emails...
Exported 118,613 emails to emails_export.json

golamv2> quit
Goodbye! [waveEmoji]
```

### Command Line Options

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--data` | `-d` | Path to GolamV2 data directory | `golamv2_data` |
| `--output` | `-o` | Output file for exports | (none) |

## Database Storage

### URL Database (`urls/`)
- Stores pending URLs for crawling
- Automatic queue refilling when memory queue is <40% full
- Optimized for fast retrieval and batch operations

### Results Database (`finds_*`)
- Stores crawling results based on mode:
  - `finds_email`: Email hunting results
  - `finds_keywords`: Keyword search results  
  - `finds_domains`: Dead link detection results
  - `finds`: All-mode results

## Performance Optimization

### Memory Management
- **Bloom Filter**: 10M URL capacity, 1% false positive rate
- **Priority Queue**: 100k URL limit with smart refilling
- **BadgerDB**: Tuned for low memory - can increase to suit your environment
- **HTTP Responses**: 10MB size limit to prevent memory exhaustion

### Throughput Optimization
- **Worker Pool**: Configurable concurrent processing
- **Rate Limiting**: Respectful crawling (10 req/sec default)
- **Batch Operations**: Efficient database operations
- **Connection Pooling**: Reused HTTP connections

### Robots.txt Compliance
- **Automatic Parsing**: Fetches and caches robots.txt
- **Crawl Delays**: Respects specified delays
- **Sitemap Discovery**: Extracts sitemap URLs for better crawling
- **User-Agent Specific**: Follows rules for GolamV2-Crawler/1.0

## Configuration

### Environment Variables
```bash
export GOLAMV2_DB_PATH="./golamv2_data"
export GOLAMV2_USER_AGENT="GolamV2-Crawler/1.0"
export GOLAMV2_RATE_LIMIT="10"
```

### Memory Allocation
- **70%**: URL storage and processing
- **30%**: Results storage and caching
## Troubleshooting

### Common Issues

1. **Memory Usage Too High**
   - Reduce `--workers` count
   - Lower `--memory` limit
   - Reduce crawling `--depth`

2. **Slow Performance**
   - Increase `--workers` count
   - Check network connectivity
   - Monitor robots.txt delays

3. **Database Issues**
   - Ensure sufficient disk space
   - Check file permissions
   - Restart application for corruption

### Performance Tuning

1. **For High-Memory Systems**
   ```bash
   ./golamv2 --workers 100 --memory 800 --url https://example.com
   ```

2. **For Low-Memory Systems**
   ```bash
   ./golamv2 --workers 20 --memory 200 --url https://example.com
   ```

3. **For Maximum Throughput**
   ```bash
   ./golamv2 --workers 200 --memory 400 --depth 3 --url https://example.com
   ```

## License

MIT License - see LICENSE file for details.

## Support

For issues, questions, or contributions, please use the GitHub issue tracker or mailto:golam@benar.me 

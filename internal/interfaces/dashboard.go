package interfaces

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/benar-m/golamv2/internal/domain"
	"github.com/benar-m/golamv2/pkg/metrics"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Dashboard implements the web interface for monitoring
type Dashboard struct {
	metrics  *metrics.MetricsCollector
	storage  domain.Storage
	urlQueue domain.URLQueue
	port     int
	upgrader websocket.Upgrader
	clients  map[*websocket.Conn]bool
}

// NewDashboard creates a new dashboard
func NewDashboard(metrics *metrics.MetricsCollector, storage domain.Storage, urlQueue domain.URLQueue, port int) *Dashboard {
	return &Dashboard{
		metrics:  metrics,
		storage:  storage,
		urlQueue: urlQueue,
		port:     port,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		clients: make(map[*websocket.Conn]bool),
	}
}

// Start starts the dashboard web server //Works but not the display---problem with JS
func (d *Dashboard) Start() {
	r := mux.NewRouter()

	// Serve static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))

	// API routes
	r.HandleFunc("/api/metrics", d.handleMetrics).Methods("GET")
	r.HandleFunc("/api/ws", d.handleWebSocket)
	r.HandleFunc("/api/results", d.handleResults).Methods("GET")
	r.HandleFunc("/api/add-urls", d.handleAddURLs).Methods("POST")
	r.HandleFunc("/api/db-view", d.handleDBView).Methods("GET") // New route for database view

	r.HandleFunc("/", d.handleDashboard).Methods("GET")
	r.HandleFunc("/db", d.handleDBDashboard).Methods("GET") // New route for database dashboard

	// Start broadcasting metrics to WebSocket clients
	go d.broadcastMetrics()

	addr := fmt.Sprintf(":%d", d.port)
	log.Printf("Dashboard server starting on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Printf("Dashboard server error: %v", err)
	}
}

// handleDashboard serves the main dashboard page
func (d *Dashboard) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GolamV2 Crawler Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg,rgb(1, 11, 155) 0%, #764ba2 100%);
            min-height: 100vh;
            color: #333;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        
        header {
            text-align: center;
            color: white;
            margin-bottom: 30px;
        }
        
        h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        .subtitle {
            font-size: 1.1rem;
            opacity: 0.9;
        }
        
        /* Tab Navigation */
        .tab-nav {
            display: flex;
            justify-content: center;
            margin-bottom: 30px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            padding: 5px;
            backdrop-filter: blur(10px);
        }
        
        .tab-button {
            flex: 1;
            max-width: 200px;
            padding: 15px 20px;
            background: transparent;
            border: none;
            color: white;
            cursor: pointer;
            border-radius: 10px;
            font-size: 1rem;
            font-weight: 500;
            transition: all 0.3s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
        }
        
        .tab-button:hover {
            background: rgba(255, 255, 255, 0.1);
        }
        
        .tab-button.active {
            background: white;
            color: #667eea;
            box-shadow: 0 4px 15px rgba(0,0,0,0.2);
        }
        
        .tab-content {
            display: none;
        }
        
        .tab-content.active {
            display: block;
        }
        
        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
        }
        
        .card {
            background: white;
            border-radius: 15px;
            padding: 25px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            transition: transform 0.3s ease;
        }
        
        .card:hover {
            transform: translateY(-5px);
        }
        
        .card h3 {
            color: #667eea;
            margin-bottom: 15px;
            font-size: 1.3rem;
            border-bottom: 2px solid #f0f0f0;
            padding-bottom: 10px;
        }
        
        .metric {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
            padding: 8px 0;
        }
        
        .metric-label {
            font-weight: 500;
            color: #666;
        }
        
        .metric-value {
            font-weight: bold;
            color: #333;
            font-size: 1.1rem;
        }
        
        .status-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-right: 8px;
        }
        
        .status-active {
            background-color: #4CAF50;
            animation: pulse 2s infinite;
        }
        
        .status-idle {
            background-color: #FFC107;
        }
        
        @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.5; }
            100% { opacity: 1; }
        }
        
        .progress-bar {
            width: 100%;
            height: 20px;
            background-color: #e0e0e0;
            border-radius: 10px;
            overflow: hidden;
            margin-top: 5px;
        }
        
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #667eea, #764ba2);
            transition: width 0.3s ease;
        }
        
        .large-number {
            font-size: 2rem;
            font-weight: bold;
            color: #667eea;
        }
        
        .update-time {
            text-align: center;
            color: #888;
            margin-top: 20px;
            font-size: 0.9rem;
        }
        
        .error {
            color: #f44336;
        }
        
        .success {
            color: #4CAF50;
        }
        
        /* URL Management Styles */
        .url-form {
            background: white;
            border-radius: 15px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            margin-bottom: 20px;
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        .form-group label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
            color: #333;
        }
        
        .form-group textarea {
            width: 100%;
            min-height: 120px;
            padding: 12px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-family: inherit;
            font-size: 14px;
            resize: vertical;
            transition: border-color 0.3s ease;
        }
        
        .form-group textarea:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .form-actions {
            display: flex;
            gap: 15px;
            align-items: center;
        }
        
        .btn {
            padding: 12px 24px;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
            display: inline-flex;
            align-items: center;
            gap: 8px;
        }
        
        .btn-primary {
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
        }
        
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(102, 126, 234, 0.4);
        }
        
        .btn-secondary {
            background: #f5f5f5;
            color: #666;
        }
        
        .btn-secondary:hover {
            background: #e0e0e0;
        }
        
        .form-help {
            font-size: 14px;
            color: #666;
            margin-top: 5px;
        }
        
        .message {
            padding: 12px 16px;
            border-radius: 8px;
            margin-bottom: 20px;
            font-weight: 500;
        }
        
        .message-success {
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
        }
        
        .message-error {
            background: #f8d7da;
            color: #721c24;
            border: 1px solid #f5c6cb;
        }
        
        /* Results Styles */
        .results-controls {
            background: white;
            border-radius: 15px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            display: flex;
            gap: 15px;
            align-items: center;
            flex-wrap: wrap;
        }
        
        .filter-group {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        
        .filter-group label {
            font-weight: 500;
            color: #333;
        }
        
        .filter-group select,
        .filter-group input {
            padding: 8px 12px;
            border: 2px solid #e0e0e0;
            border-radius: 6px;
            font-size: 14px;
            transition: border-color 0.3s ease;
        }
        
        .filter-group select:focus,
        .filter-group input:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .results-table {
            background: white;
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            overflow: hidden;
        }
        
        .table {
            width: 100%;
            border-collapse: collapse;
        }
        
        .table th,
        .table td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        
        .table th {
            background: #f8f9fa;
            font-weight: 600;
            color: #333;
        }
        
        .table tr:hover {
            background: #f8f9fa;
        }
        
        .table .url-cell {
            max-width: 300px;
            word-break: break-all;
        }
        
        .status-badge {
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
        }
        
        .status-success {
            background: #d4edda;
            color: #155724;
        }
        
        .status-error {
            background: #f8d7da;
            color: #721c24;
        }
        
        .loading {
            text-align: center;
            padding: 40px;
            color: #666;
        }
        
        .no-results {
            text-align: center;
            padding: 40px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🕸️ GolamV2 Crawler Dashboard</h1>
            <p class="subtitle">Real-time monitoring of your web crawling operation</p>
        </header>
        
        <!-- Tab Navigation -->
        <div class="tab-nav">
            <button class="tab-button active" onclick="switchTab('monitoring')">
                 Monitoring
            </button>
            <button class="tab-button" onclick="switchTab('add-urls')">
                 Add URLs
            </button>
            <button class="tab-button" onclick="switchTab('results')">
                 Results
            </button>
            <a href="/db" style="text-decoration: none;" class="tab-button">
                🗄️ Database Viewer
            </a>
        </div>
        
        <!-- Monitoring Tab -->
        <div id="monitoring" class="tab-content active">
        <div class="dashboard-grid">
            <!-- Overview Card -->
            <div class="card">
                <h3> Overview</h3>
                <div class="metric">
                    <span class="metric-label">Status</span>
                    <span class="metric-value">
                        <span class="status-indicator status-active"></span>
                        <span id="status">Active</span>
                    </span>
                </div>
                <div class="metric">
                    <span class="metric-label">URLs Processed</span>
                    <span class="metric-value large-number" id="urls-processed">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">URLs per Second</span>
                    <span class="metric-value" id="urls-per-second">0.0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Uptime</span>
                    <span class="metric-value" id="uptime">00:00:00</span>
                </div>
            </div>
            
            <!-- Queue Status Card -->
            <div class="card">
                <h3> Queue Status</h3>
                <div class="metric">
                    <span class="metric-label">URLs in Queue</span>
                    <span class="metric-value" id="urls-in-queue">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">URLs in Database</span>
                    <span class="metric-value" id="urls-in-db">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Active Workers</span>
                    <span class="metric-value" id="active-workers">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Memory Usage</span>
                    <span class="metric-value" id="memory-usage">0.0 MB</span>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" id="memory-progress" style="width: 0%"></div>
                </div>
            </div>
            
            <!-- Findings Card -->
            <div class="card">
                <h3> Findings</h3>
                <div class="metric">
                    <span class="metric-label">📧 Emails Found</span>
                    <span class="metric-value success" id="emails-found">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Keywords Found</span>
                    <span class="metric-value success" id="keywords-found">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Dead Links</span>
                    <span class="metric-value error" id="dead-links">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Dead Domains</span>
                    <span class="metric-value error" id="dead-domains">0</span>
                </div>
            </div>
            
            <!-- Performance Card -->
            <div class="card">
                <h3>⚡ Performance</h3>
                <div class="metric">
                    <span class="metric-label">Success Rate</span>
                    <span class="metric-value" id="success-rate">100%</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Total Errors</span>
                    <span class="metric-value error" id="total-errors">0</span>
                </div>
                <div class="metric">
                    <span class="metric-label">Avg Processing Time</span>
                    <span class="metric-value" id="avg-processing-time">0ms</span>
                </div>
            </div>
            
            <!-- Memory Breakdown Card -->
            <div class="card">
                <h3> Memory Breakdown</h3>
                <div class="metric">
                    <span class="metric-label"> Bloom Filter</span>
                    <span class="metric-value" id="memory-bloom">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Database</span>
                    <span class="metric-value" id="memory-database">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Queue</span>
                    <span class="metric-value" id="memory-queue">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> HTTP Buffers</span>
                    <span class="metric-value" id="memory-http">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Parsing</span>
                    <span class="metric-value" id="memory-parsing">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Crawlers</span>
                    <span class="metric-value" id="memory-crawlers">0.0 MB</span>
                </div>
                <div class="metric">
                    <span class="metric-label"> Other</span>
                    <span class="metric-value" id="memory-other">0.0 MB</span>
                </div>
                <div class="metric" style="border-top: 2px solid #667eea; margin-top: 10px; padding-top: 10px;">
                    <span class="metric-label" style="font-weight: bold;">📊 Total</span>
                    <span class="metric-value" style="font-weight: bold; color: #667eea;" id="memory-total">0.0 MB</span>
                </div>
            </div>
        </div>
        
        <!-- Add URLs Tab -->
        <div id="add-urls" class="tab-content">
            <div class="url-form">
                <h3> Add URLs to Crawl</h3>
                <div id="url-message"></div>
                <form id="url-form">
                    <div class="form-group">
                        <label for="urls">URLs (one per line):</label>
                        <textarea id="urls" name="urls" placeholder="https://example.com&#10;https://another-site.com&#10;https://third-site.com/page"></textarea>
                        <div class="form-help">
                            Enter one URL per line. URLs should include the protocol (http:// or https://)
                        </div>
                    </div>
                    <div class="form-actions">
                        <button type="submit" class="btn btn-primary">
                             Add URLs
                        </button>
                        <button type="button" class="btn btn-secondary" onclick="clearURLForm()">
                             Clear
                        </button>
                        <span id="url-count">0 URLs ready</span>
                    </div>
                </form>
            </div>
        </div>
        
        <!-- Results Tab -->
        <div id="results" class="tab-content">
            <div class="results-controls">
                <div class="filter-group">
                    <label for="result-type">Type:</label>
                    <select id="result-type" onchange="loadResults()">
                        <option value="all">All Results</option>
                        <option value="emails">Emails</option>
                        <option value="keywords">Keywords</option>
                        <option value="dead_links">Dead Links</option>
                    </select>
                </div>
                <div class="filter-group">
                    <label for="result-limit">Limit:</label>
                    <select id="result-limit" onchange="loadResults()">
                        <option value="100">100</option>
                        <option value="500">500</option>
                        <option value="1000">1000</option>
                    </select>
                </div>
                <button class="btn btn-primary" onclick="loadResults()">
                     Refresh
                </button>
                <button class="btn btn-secondary" onclick="exportResults()">
                     Export
                </button>
            </div>
            
            <div class="results-table">
                <div id="results-loading" class="loading">
                    Loading results...
                </div>
                <div id="results-content" style="display: none;">
                    <table class="table">
                        <thead>
                            <tr>
                                <th>Type</th>
                                <th>URL</th>
                                <th>Data</th>
                                <th>Found At</th>
                            </tr>
                        </thead>
                        <tbody id="results-tbody">
                        </tbody>
                    </table>
                </div>
                <div id="results-empty" class="no-results" style="display: none;">
                    No results found matching your criteria.
                </div>
            </div>
        </div>
        
        <!-- Database Tab -->
        <div id="db" class="tab-content">
            <h3>🗄️ Database Information</h3>
            <div id="db-message"></div>
            <div class="results-table">
                <div id="db-loading" class="loading">
                    Loading database information...
                </div>
                <div id="db-content" style="display: none;">
                    <table class="table">
                        <thead>
                            <tr>
                                <th>Collection</th>
                                <th>Document Count</th>
                                <th>Size (MB)</th>
                            </tr>
                        </thead>
                        <tbody id="db-tbody">
                        </tbody>
                    </table>
                </div>
                <div id="db-empty" class="no-results" style="display: none;">
                    No database information available.
                </div>
            </div>
        </div>
        
        <div class="update-time">
            Last updated: <span id="last-update">Never</span>
        </div>
    </div>
    
    <script>
        const ws = new WebSocket('ws://localhost:' + window.location.port + '/api/ws');
        
        ws.onmessage = function(event) {
            const metrics = JSON.parse(event.data);
            updateMetrics(metrics);
        };
        
        ws.onerror = function(error) {
            console.error('WebSocket error:', error);
            document.getElementById('status').textContent = 'Disconnected';
            document.querySelector('.status-indicator').className = 'status-indicator status-idle';
        };
        
        // Tab Management - MOVED TO GLOBAL SCOPE
        function switchTab(tabName) {
            // Hide all tabs
            const tabs = document.querySelectorAll('.tab-content');
            tabs.forEach(tab => tab.classList.remove('active'));

            // Remove active from all buttons
            const buttons = document.querySelectorAll('.tab-button');
            buttons.forEach(button => button.classList.remove('active'));

            // Show selected tab
            const targetTab = document.getElementById(tabName);
            if (targetTab) {
                targetTab.classList.add('active');
            }

            // Activate the clicked button (find by data-tab attribute or fallback to partial match)
            let clickedButton = document.querySelector('.tab-button[onclick*="' + tabName + '"]');
            if (!clickedButton) {
                // fallback: find by text content
                clickedButton = Array.from(document.querySelectorAll('.tab-button')).find(btn => btn.textContent.includes(tabName));
            }
            if (clickedButton) {
                clickedButton.classList.add('active');
            }

            // Load data for specific tabs
            if (tabName === 'results') {
                loadResults();
            } else if (tabName === 'db') {
                loadDBInfo();
            }
        }
        
        // Initialize when DOM is ready
        document.addEventListener('DOMContentLoaded', function() {
            // URL Management
            const urlForm = document.getElementById('url-form');
            if (urlForm) {
                urlForm.addEventListener('submit', function(e) {
                    e.preventDefault();
                    addURLs();
                });
            }
            const urlsTextarea = document.getElementById('urls');
            if (urlsTextarea) {
                urlsTextarea.addEventListener('input', function() {
                    updateURLCount();
                });
            }
            // Initialize URL count
            updateURLCount();
            // Always show monitoring tab by default
            switchTab('monitoring');
        });
        
        function updateURLCount() {
            const urlsElement = document.getElementById('urls');
            const countElement = document.getElementById('url-count');
            
            if (!urlsElement || !countElement) return;
            
            const urls = urlsElement.value.split('\n').filter(url => url.trim() !== '');
            countElement.textContent = urls.length + ' URLs ready';
        }
        
        function clearURLForm() {
            const urlsElement = document.getElementById('urls');
            const messageElement = document.getElementById('url-message');
            
            if (urlsElement) {
                urlsElement.value = '';
            }
            
            updateURLCount();
            
            if (messageElement) {
                messageElement.innerHTML = '';
            }
        }
        
        async function addURLs() {
            const urlsElement = document.getElementById('urls');
            if (!urlsElement) {
                console.error('URLs textarea not found');
                return;
            }
            
            const urlsText = urlsElement.value;
            const urls = urlsText.split('\n').filter(url => url.trim() !== '').map(url => url.trim());
            
            if (urls.length === 0) {
                showMessage('Please enter at least one URL', 'error');
                return;
            }
            
            try {
                const response = await fetch('/api/add-urls', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ urls: urls })
                });
                
                if (response.ok) {
                    const result = await response.json();
                    let message = 'Successfully added ' + result.added + ' URLs to the queue!';
                    
                    if (result.invalid_urls && result.invalid_urls.length > 0) {
                        message += ' (' + result.invalid_urls.length + ' invalid URLs skipped)';
                    }
                    
                    showMessage(message, 'success');
                    clearURLForm();
                } else {
                    const errorText = await response.text();
                    showMessage('Error: ' + errorText, 'error');
                }
            } catch (error) {
                showMessage('Network error: ' + error.message, 'error');
            }
        }
        
        function showMessage(message, type) {
            const messageDiv = document.getElementById('url-message');
            if (!messageDiv) {
                console.error('Message div not found');
                return;
            }
            
            messageDiv.innerHTML = '<div class="message message-' + type + '">' + message + '</div>';
            setTimeout(() => {
                if (messageDiv) {
                    messageDiv.innerHTML = '';
                }
            }, 5000);
        }
        
        // Results Management
        async function loadResults() {
            const type = document.getElementById('result-type').value;
            const limit = document.getElementById('result-limit').value;
            
            document.getElementById('results-loading').style.display = 'block';
            document.getElementById('results-content').style.display = 'none';
            document.getElementById('results-empty').style.display = 'none';
            
            try {
                const response = await fetch('/api/results?type=' + type + '&limit=' + limit);
                const results = await response.json();
                
                document.getElementById('results-loading').style.display = 'none';
                
                if (results.length === 0) {
                    document.getElementById('results-empty').style.display = 'block';
                } else {
                    displayResults(results);
                    document.getElementById('results-content').style.display = 'block';
                }
            } catch (error) {
                console.error('Error loading results:', error);
                document.getElementById('results-loading').style.display = 'none';
                document.getElementById('results-empty').style.display = 'block';
            }
        }
        
        function displayResults(results) {
            const tbody = document.getElementById('results-tbody');
            tbody.innerHTML = '';
            
            results.forEach(result => {
                const row = document.createElement('tr');
                row.innerHTML = 
                    '<td><span class="status-badge status-success">' + result.type + '</span></td>' +
                    '<td class="url-cell"><a href="' + result.source_url + '" target="_blank">' + result.source_url + '</a></td>' +
                    '<td>' + result.data + '</td>' +
                    '<td>' + new Date(result.found_at).toLocaleString() + '</td>';
                tbody.appendChild(row);
            });
        }
        
        function exportResults() {
            const type = document.getElementById('result-type').value;
            const limit = document.getElementById('result-limit').value;
            
            // Create download link
            const url = '/api/results?type=' + type + '&limit=' + limit + '&format=csv';
            const a = document.createElement('a');
            a.href = url;
            a.download = 'crawler-results-' + type + '-' + new Date().toISOString().split('T')[0] + '.csv';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
        }
        
        // New function to load database information
        async function loadDBInfo() {
            document.getElementById('db-loading').style.display = 'block';
            document.getElementById('db-content').style.display = 'none';
            document.getElementById('db-empty').style.display = 'none';
            
            try {
                const response = await fetch('/api/db-view');
                const dbInfo = await response.json();
                
                document.getElementById('db-loading').style.display = 'none';
                
                if (dbInfo.length === 0) {
                    document.getElementById('db-empty').style.display = 'block';
                } else {
                    displayDBInfo(dbInfo);
                    document.getElementById('db-content').style.display = 'block';
                }
            } catch (error) {
                console.error('Error loading db info:', error);
                document.getElementById('db-loading').style.display = 'none';
                document.getElementById('db-empty').style.display = 'block';
            }
        }
        
        // New function to display database information
        function displayDBInfo(dbInfo) {
            const tbody = document.getElementById('db-tbody');
            tbody.innerHTML = '';
            
            dbInfo.forEach(info => {
                const row = document.createElement('tr');
                row.innerHTML = 
                    '<td>' + info.collection + '</td>' +
                    '<td>' + info.document_count.toLocaleString() + '</td>' +
                    '<td>' + info.size_mb.toFixed(1) + ' MB</td>';
                tbody.appendChild(row);
            });
        }
        
        function updateMetrics(metrics) {
            // Overview
            document.getElementById('urls-processed').textContent = metrics.urls_processed.toLocaleString();
            document.getElementById('urls-per-second').textContent = metrics.urls_per_second.toFixed(1);
            
            // Calculate uptime
            const startTime = new Date(metrics.start_time);
            const now = new Date();
            const uptime = Math.floor((now - startTime) / 1000);
            document.getElementById('uptime').textContent = formatUptime(uptime);
            
            // Queue Status
            document.getElementById('urls-in-queue').textContent = metrics.urls_in_queue.toLocaleString();
            document.getElementById('urls-in-db').textContent = metrics.urls_in_db.toLocaleString();
            document.getElementById('active-workers').textContent = metrics.active_workers;
            document.getElementById('memory-usage').textContent = metrics.memory_usage_mb.toFixed(1) + ' MB';
            
            // Memory progress bar (assuming 500MB limit)
            const memoryPercent = Math.min((metrics.memory_usage_mb / 500) * 100, 100);
            document.getElementById('memory-progress').style.width = memoryPercent + '%';
            
            // Findings
            document.getElementById('emails-found').textContent = metrics.emails_found.toLocaleString();
            document.getElementById('keywords-found').textContent = metrics.keywords_found.toLocaleString();
            document.getElementById('dead-links').textContent = metrics.dead_links_found.toLocaleString();
            document.getElementById('dead-domains').textContent = metrics.dead_domains_found.toLocaleString();
            
            // Performance
            const successRate = metrics.urls_processed > 0 ? 
                ((metrics.urls_processed - metrics.errors) / metrics.urls_processed * 100).toFixed(1) : 100;
            document.getElementById('success-rate').textContent = successRate + '%';
            document.getElementById('total-errors').textContent = metrics.errors.toLocaleString();
            
            // Memory Breakdown
            if (metrics.memory_breakdown) {
                document.getElementById('memory-bloom').textContent = metrics.memory_breakdown.bloom_filter_mb.toFixed(1) + ' MB';
                document.getElementById('memory-database').textContent = metrics.memory_breakdown.database_mb.toFixed(1) + ' MB';
                document.getElementById('memory-queue').textContent = metrics.memory_breakdown.queue_mb.toFixed(1) + ' MB';
                document.getElementById('memory-http').textContent = metrics.memory_breakdown.http_buffers_mb.toFixed(1) + ' MB';
                document.getElementById('memory-parsing').textContent = metrics.memory_breakdown.parsing_mb.toFixed(1) + ' MB';
                document.getElementById('memory-crawlers').textContent = metrics.memory_breakdown.crawlers_mb.toFixed(1) + ' MB';
                document.getElementById('memory-other').textContent = metrics.memory_breakdown.other_mb.toFixed(1) + ' MB';
                document.getElementById('memory-total').textContent = metrics.memory_breakdown.total_mb.toFixed(1) + ' MB';
            }
            
            // Update timestamp
            document.getElementById('last-update').textContent = new Date().toLocaleTimeString();
        }
        
        function formatUptime(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            const secs = seconds % 60;
            return String(hours).padStart(2, '0') + ':' + 
                   String(minutes).padStart(2, '0') + ':' + 
                   String(secs).padStart(2, '0');
        }
        
        // Fetch initial metrics
        fetch('/api/metrics')
            .then(response => response.json())
            .then(metrics => updateMetrics(metrics))
            .catch(error => console.error('Error fetching initial metrics:', error));
    </script>
</body>
</html>
`

	t, err := template.New("dashboard").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, nil)
}

// handleMetrics serves current metrics as JSON
func (d *Dashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := d.metrics.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (d *Dashboard) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := d.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Register client
	d.clients[conn] = true

	// Remove client when connection closes
	defer func() {
		delete(d.clients, conn)
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastMetrics sends metrics to all connected WebSocket clients
func (d *Dashboard) broadcastMetrics() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics := d.metrics.GetMetrics()
		data, err := json.Marshal(metrics)
		if err != nil {
			continue
		}

		// Send to all connected clients
		for client := range d.clients {
			err := client.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				// Remove disconnected client
				delete(d.clients, client)
				client.Close()
			}
		}
	}
}

// handleResults serves the results API endpoint
func (d *Dashboard) handleResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get query parameters
	resultType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	// Default values
	if resultType == "" {
		resultType = "all"
	}
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Get results from storage
	var results []domain.CrawlResult
	var err error

	switch resultType {
	case "emails":
		results, err = d.storage.GetResults(domain.ModeEmail, limit)
	case "keywords":
		results, err = d.storage.GetResults(domain.ModeKeywords, limit)
	case "dead_links":
		results, err = d.storage.GetResults(domain.ModeDomains, limit)
	case "all":
		results, err = d.storage.GetResults(domain.ModeAll, limit)
	default:
		results, err = d.storage.GetResults(domain.ModeAll, limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching results: %v", err), http.StatusInternalServerError)
		return
	}

	// Transform results for frontend
	var responseResults []map[string]interface{}
	for _, result := range results {
		// Create entries based on what was found in this result
		if len(result.Emails) > 0 {
			for _, email := range result.Emails {
				responseResults = append(responseResults, map[string]interface{}{
					"type":       "email",
					"source_url": result.URL,
					"data":       email,
					"found_at":   result.ProcessedAt,
				})
			}
		}

		if len(result.Keywords) > 0 {
			for keyword, count := range result.Keywords {
				responseResults = append(responseResults, map[string]interface{}{
					"type":       "keyword",
					"source_url": result.URL,
					"data":       fmt.Sprintf("%s (found %d times)", keyword, count),
					"found_at":   result.ProcessedAt,
				})
			}
		}

		if len(result.DeadLinks) > 0 {
			for _, deadLink := range result.DeadLinks {
				responseResults = append(responseResults, map[string]interface{}{
					"type":       "dead_link",
					"source_url": result.URL,
					"data":       deadLink,
					"found_at":   result.ProcessedAt,
				})
			}
		}

		if len(result.DeadDomains) > 0 {
			for _, deadDomain := range result.DeadDomains {
				responseResults = append(responseResults, map[string]interface{}{
					"type":       "dead_domain",
					"source_url": result.URL,
					"data":       deadDomain,
					"found_at":   result.ProcessedAt,
				})
			}
		}

		// If no specific findings, show the crawl result itself
		if len(result.Emails) == 0 && len(result.Keywords) == 0 &&
			len(result.DeadLinks) == 0 && len(result.DeadDomains) == 0 {
			status := "success"
			if result.Error != "" {
				status = "error"
			}
			responseResults = append(responseResults, map[string]interface{}{
				"type":       status,
				"source_url": result.URL,
				"data":       fmt.Sprintf("Status: %d, Title: %s", result.StatusCode, result.Title),
				"found_at":   result.ProcessedAt,
			})
		}
	}

	json.NewEncoder(w).Encode(responseResults)
}

// handleAddURLs handles adding new URLs to the crawl queue
func (d *Dashboard) handleAddURLs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse JSON request body
	var request struct {
		URLs []string `json:"urls"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	if len(request.URLs) == 0 {
		http.Error(w, "No URLs provided", http.StatusBadRequest)
		return
	}

	// Validate and add URLs to queue
	var validURLs []string
	var invalidURLs []string

	for _, rawURL := range request.URLs {
		cleanURL := strings.TrimSpace(rawURL)
		if cleanURL == "" {
			continue
		}

		// Validate URL
		if parsedURL, err := url.Parse(cleanURL); err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			validURLs = append(validURLs, cleanURL)
		} else {
			invalidURLs = append(invalidURLs, cleanURL)
		}
	}

	// Add valid URLs to queue
	var addedCount int
	var errors []string

	for _, validURL := range validURLs {
		task := domain.URLTask{
			URL:       validURL,
			Depth:     0,
			Timestamp: time.Now(),
			Retries:   0,
		}

		if err := d.urlQueue.Push(task); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to add %s: %v", validURL, err))
		} else {
			addedCount++
		}
	}

	// Prepare response
	response := map[string]interface{}{
		"success":      true,
		"added":        addedCount,
		"total_valid":  len(validURLs),
		"invalid_urls": invalidURLs,
		"errors":       errors,
		"message":      fmt.Sprintf("Successfully added %d URLs to the crawl queue", addedCount),
	}

	json.NewEncoder(w).Encode(response)
}

// handleDBView serves detailed database information
func (d *Dashboard) handleDBView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get query parameters
	resultType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	// Default values
	if resultType == "" {
		resultType = "all"
	}
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Get results from storage for DB view
	var results []domain.CrawlResult
	var err error

	switch resultType {
	case "emails":
		results, err = d.storage.GetResults(domain.ModeEmail, limit)
	case "keywords":
		results, err = d.storage.GetResults(domain.ModeKeywords, limit)
	case "dead_links":
		results, err = d.storage.GetResults(domain.ModeDomains, limit)
	case "all":
		results, err = d.storage.GetResults(domain.ModeAll, limit)
	default:
		results, err = d.storage.GetResults(domain.ModeAll, limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching database content: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert results to a more database-oriented view
	type DBEntry struct {
		ID           string      `json:"id"`
		URL          string      `json:"url"`
		ProcessedAt  time.Time   `json:"processed_at"`
		DataType     string      `json:"data_type"`
		DataCount    int         `json:"data_count"`
		StatusCode   int         `json:"status_code"`
		ProcessTime  float64     `json:"process_time_ms"`
		HasError     bool        `json:"has_error"`
		ErrorMessage string      `json:"error_message,omitempty"`
		RawData      interface{} `json:"raw_data"`
	}

	var entries []DBEntry

	for i, result := range results {
		// Create a unique ID for each result based on URL and timestamp
		id := fmt.Sprintf("result_%d", i+1)

		// Create the basic entry
		entry := DBEntry{
			ID:           id,
			URL:          result.URL,
			ProcessedAt:  result.ProcessedAt,
			StatusCode:   result.StatusCode,
			ProcessTime:  float64(result.ProcessTime) / float64(time.Millisecond),
			HasError:     result.Error != "",
			ErrorMessage: result.Error,
		}

		// Add email data if any
		if len(result.Emails) > 0 {
			emailEntry := entry
			emailEntry.DataType = "emails"
			emailEntry.DataCount = len(result.Emails)
			emailEntry.RawData = result.Emails
			entries = append(entries, emailEntry)
		}

		// Add keyword data if any
		if len(result.Keywords) > 0 {
			keywordEntry := entry
			keywordEntry.DataType = "keywords"
			keywordEntry.DataCount = len(result.Keywords)
			keywordEntry.RawData = result.Keywords
			entries = append(entries, keywordEntry)
		}

		// Add dead links if any
		if len(result.DeadLinks) > 0 {
			deadLinksEntry := entry
			deadLinksEntry.DataType = "dead_links"
			deadLinksEntry.DataCount = len(result.DeadLinks)
			deadLinksEntry.RawData = result.DeadLinks
			entries = append(entries, deadLinksEntry)
		}

		// Add dead domains if any
		if len(result.DeadDomains) > 0 {
			deadDomainsEntry := entry
			deadDomainsEntry.DataType = "dead_domains"
			deadDomainsEntry.DataCount = len(result.DeadDomains)
			deadDomainsEntry.RawData = result.DeadDomains
			entries = append(entries, deadDomainsEntry)
		}

		// If no specific findings, create a general entry
		if len(result.Emails) == 0 && len(result.Keywords) == 0 &&
			len(result.DeadLinks) == 0 && len(result.DeadDomains) == 0 {
			entry.DataType = "general"
			entry.RawData = map[string]interface{}{
				"title": result.Title,
			}
			entries = append(entries, entry)
		}
	}

	json.NewEncoder(w).Encode(entries)
}

// handleDBDashboard serves the database dashboard page
func (d *Dashboard) handleDBDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GolamV2 Crawler Database Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            color: #333;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        
        header {
            text-align: center;
            color: white;
            margin-bottom: 30px;
        }
        
        h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        .subtitle {
            font-size: 1.1rem;
            opacity: 0.9;
        }
        
        /* Navigation */
        .navigation {
            display: flex;
            justify-content: center;
            margin-bottom: 20px;
        }
        
        .navigation a {
            padding: 10px 20px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 10px;
            color: white;
            text-decoration: none;
            margin: 0 10px;
            transition: all 0.3s ease;
        }
        
        .navigation a:hover {
            background: rgba(255, 255, 255, 0.2);
            transform: translateY(-2px);
        }
        
        .tab-nav {
            display: flex;
            justify-content: center;
            margin-bottom: 30px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 15px;
            padding: 5px;
            backdrop-filter: blur(10px);
        }
        
        .tab-button {
            flex: 1;
            max-width: 200px;
            padding: 15px 20px;
            background: transparent;
            border: none;
            color: white;
            cursor: pointer;
            border-radius: 10px;
            font-size: 1rem;
            font-weight: 500;
            transition: all 0.3s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
        }
        
        .tab-button:hover {
            background: rgba(255, 255, 255, 0.1);
        }
        
        .tab-button.active {
            background: white;
            color: #667eea;
            box-shadow: 0 4px 15px rgba(0,0,0,0.2);
        }
        
        .tab-content {
            display: none;
        }
        
        .tab-content.active {
            display: block;
        }
        
        .results-controls {
            background: white;
            border-radius: 15px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            display: flex;
            gap: 15px;
            align-items: center;
            flex-wrap: wrap;
        }
        
        .filter-group {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        
        .filter-group label {
            font-weight: 500;
            color: #333;
        }
        
        .filter-group select,
        .filter-group input {
            padding: 8px 12px;
            border: 2px solid #e0e0e0;
            border-radius: 6px;
            font-size: 14px;
            transition: border-color 0.3s ease;
        }
        
        .filter-group select:focus,
        .filter-group input:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .results-table {
            background: white;
            border-radius: 15px;
            padding: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            overflow: hidden;
        }
        
        .table {
            width: 100%;
            border-collapse: collapse;
        }
        
        .table th,
        .table td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #e0e0e0;
        }
        
        .table th {
            background: #f8f9fa;
            font-weight: 600;
            color: #333;
            position: sticky;
            top: 0;
            z-index: 10;
        }
        
        .table tr:hover {
            background: #f8f9fa;
        }
        
        .table .url-cell {
            max-width: 300px;
            word-break: break-all;
        }
        
        .status-badge {
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
        }
        
        .status-success {
            background: #d4edda;
            color: #155724;
        }
        
        .status-error {
            background: #f8d7da;
            color: #721c24;
        }
        
        .loading {
            text-align: center;
            padding: 40px;
            color: #666;
        }
        
        .no-results {
            text-align: center;
            padding: 40px;
            color: #666;
        }
        
        pre {
            background: #f8f9fa;
            border: 1px solid #e0e0e0;
            border-radius: 4px;
            padding: 10px;
            overflow: auto;
            max-height: 200px;
        }
        
        .card {
            background: white;
            border-radius: 15px;
            padding: 25px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            margin-bottom: 20px;
        }
        
        .card h3 {
            color: #667eea;
            margin-bottom: 15px;
            font-size: 1.3rem;
            border-bottom: 2px solid #f0f0f0;
            padding-bottom: 10px;
        }
        
        .searchbox {
            width: 100%;
            padding: 10px;
            margin-bottom: 20px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-size: 16px;
        }
        
        .searchbox:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .toggle-json {
            cursor: pointer;
            color: #667eea;
            font-weight: 500;
            margin-top: 5px;
            display: inline-block;
        }
        
        .json-content {
            display: none;
            margin-top: 10px;
        }
        
        .update-time {
            text-align: center;
            color: #888;
            margin-top: 20px;
            font-size: 0.9rem;
        }
        
        .two-column {
            display: grid;
            grid-template-columns: 1fr 2fr;
            gap: 20px;
        }
        
        @media (max-width: 768px) {
            .two-column {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🗄️ GolamV2 Database Explorer</h1>
            <p class="subtitle">Explore the crawler's database contents</p>
        </header>
        
        <div class="navigation">
            <a href="/">← Back to Dashboard</a>
        </div>
        
        <!-- Tab Navigation -->
        <div class="tab-nav">
            <button class="tab-button active" onclick="switchDBTab('db-records')">
                🗃️ Database Records
            </button>
            <button class="tab-button" onclick="switchDBTab('db-stats')">
                📊 Database Stats
            </button>
        </div>
        
        <!-- Database Records Tab -->
        <div id="db-records" class="tab-content active">
            <div class="results-controls">
                <div class="filter-group">
                    <label for="data-type">Data Type:</label>
                    <select id="data-type" onchange="loadDBData()">
                        <option value="all">All Types</option>
                        <option value="emails">Emails</option>
                        <option value="keywords">Keywords</option>
                        <option value="dead_links">Dead Links</option>
                        <option value="dead_domains">Dead Domains</option>
                        <option value="general">General</option>
                    </select>
                </div>
                <div class="filter-group">
                    <label for="data-limit">Limit:</label>
                    <select id="data-limit" onchange="loadDBData()">
                        <option value="50">50</option>
                        <option value="100" selected>100</option>
                        <option value="500">500</option>
                        <option value="1000">1000</option>
                    </select>
                </div>
                <button class="btn btn-primary" onclick="loadDBData()">
                    🔄 Refresh
                </button>
                <button class="btn btn-secondary" onclick="exportDBData()">
                    📥 Export as JSON
                </button>
            </div>

            <div class="card">
                <input type="text" id="search-box" class="searchbox" placeholder="Search in results..." oninput="filterResults()">
                <div id="search-stats"></div>
            </div>
            
            <div class="results-table">
                <div id="db-loading" class="loading">
                    Loading database records...
                </div>
                <div id="db-content" style="display: none;">
                    <table class="table" id="db-table">
                        <thead>
                            <tr>
                                <th width="10%">Type</th>
                                <th width="35%">URL</th>
                                <th width="35%">Data</th>
                                <th width="20%">Details</th>
                            </tr>
                        </thead>
                        <tbody id="db-tbody">
                        </tbody>
                    </table>
                </div>
                <div id="db-empty" class="no-results" style="display: none;">
                    No database records found matching your criteria.
                </div>
            </div>
        </div>
        
        <!-- Database Stats Tab -->
        <div id="db-stats" class="tab-content">
            <div class="two-column">
                <div class="card">
                    <h3>📊 Database Statistics</h3>
                    <div id="db-stats-content">
                        <div class="loading">Loading statistics...</div>
                    </div>
                </div>
                
                <div class="card">
                    <h3>📈 Data Distribution</h3>
                    <div id="db-stats-chart">
                        <canvas id="data-distribution-chart" width="400" height="300"></canvas>
                    </div>
                </div>
            </div>
            
            <div class="card">
                <h3>📆 Timeline</h3>
                <div id="db-timeline">
                    <canvas id="timeline-chart" width="800" height="300"></canvas>
                </div>
            </div>
        </div>
        
        <!-- Record Details Modal -->
        <div id="record-modal" style="display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 100; overflow-y: auto;">
            <div style="background: white; max-width: 800px; margin: 50px auto; border-radius: 15px; padding: 20px; position: relative;">
                <button onclick="closeModal()" style="position: absolute; top: 10px; right: 10px; background: none; border: none; font-size: 20px; cursor: pointer;">✕</button>
                <h3 id="modal-title">Record Details</h3>
                <div id="modal-content"></div>
            </div>
        </div>
        
        <div class="update-time">
            Last updated: <span id="last-update">Never</span>
        </div>
    </div>
    
    <!-- Include Chart.js for visualizations -->
    <script src="https://cdn.jsdelivr.net/npm/chart.js@3.7.0/dist/chart.min.js"></script>

    <script>
        // Global variables to store loaded data
        let dbData = [];
        let filteredData = [];
        
        // Tab Management for Database Dashboard
        function switchDBTab(tabName) {
            // Hide all tabs
            const tabs = document.querySelectorAll('.tab-content');
            tabs.forEach(tab => tab.classList.remove('active'));

            // Remove active from all buttons
            const buttons = document.querySelectorAll('.tab-button');
            buttons.forEach(button => button.classList.remove('active'));

            // Show selected tab
            document.getElementById(tabName).classList.add('active');

            // Activate the clicked button
            const clickedButton = document.querySelector('[onclick="switchDBTab(\\\'' + tabName + '\\\')"');
            if (clickedButton) {
                clickedButton.classList.add('active');
            }

            // Load data for specific tabs
            if (tabName === 'db-records' && dbData.length === 0) {
                loadDBData();
            } else if (tabName === 'db-stats') {
                loadDBStats();
            }
        }
        
        // Load database records
        async function loadDBData() {
            const type = document.getElementById('data-type').value;
            const limit = document.getElementById('data-limit').value;
            
            document.getElementById('db-loading').style.display = 'block';
            document.getElementById('db-content').style.display = 'none';
            document.getElementById('db-empty').style.display = 'none';
            
            try {
                const response = await fetch('/api/db-view?type=' + type + '&limit=' + limit);
                dbData = await response.json();
                filteredData = [...dbData]; // Copy for filtering
                
                document.getElementById('db-loading').style.display = 'none';
                
                if (dbData.length === 0) {
                    document.getElementById('db-empty').style.display = 'block';
                } else {
                    displayDBData(dbData);
                    document.getElementById('db-content').style.display = 'block';
                    document.getElementById('search-stats').innerHTML = dbData.length + ' records found';
                }
            } catch (error) {
                console.error('Error loading database records:', error);
                document.getElementById('db-loading').style.display = 'none';
                document.getElementById('db-empty').style.display = 'block';
                document.getElementById('db-empty').innerHTML = 'Error loading database: ' + error.message;
            }
            
            document.getElementById('last-update').textContent = new Date().toLocaleTimeString();
        }
        
        // Display database records
        function displayDBData(data) {
            const tbody = document.getElementById('db-tbody');
            tbody.innerHTML = '';
            
            data.forEach(record => {
                const row = document.createElement('tr');
                
                // Determine badge class based on data type
                let badgeClass = 'status-success';
                if (record.has_error) {
                    badgeClass = 'status-error';
                }
                
                // Format process time
                const processTime = record.process_time_ms.toFixed(2) + ' ms';
                
                // Format data content for display
                let dataContent = '';
                if (record.data_type === 'emails') {
                    dataContent = Array.isArray(record.raw_data) ? record.raw_data.join(', ') : record.raw_data;
                } else if (record.data_type === 'keywords') {
                    if (typeof record.raw_data === 'object') {
                        const keywords = [];
                        for (const [key, value] of Object.entries(record.raw_data)) {
                            keywords.push(key + ' (' + value + ')');
                        }
                        dataContent = keywords.join(', ');
                    } else {
                        dataContent = String(record.raw_data);
                    }
                } else if (record.data_type === 'dead_links' || record.data_type === 'dead_domains') {
                    dataContent = Array.isArray(record.raw_data) ? record.raw_data.join(', ') : record.raw_data;
                } else {
                    dataContent = JSON.stringify(record.raw_data);
                }
                
                // Truncate content if too long
                const maxLength = 100;
                const displayContent = dataContent.length > maxLength ? 
                    dataContent.substring(0, maxLength) + '...' : dataContent;
                
                row.innerHTML = 
                    '<td><span class="status-badge ' + badgeClass + '">' + record.data_type + '</span></td>' +
                    '<td class="url-cell"><a href="' + record.url + '" target="_blank">' + record.url + '</a></td>' +
                    '<td>' + displayContent + '</td>' +
                    '<td>' + 
                        'Status: ' + record.status_code + '<br>' +
                        'Time: ' + processTime + '<br>' +
                        '<a href="#" class="toggle-json" onclick="viewRecord(\'' + record.id + '\')">View Details</a>' +
                    '</td>';
                tbody.appendChild(row);
            });
        }
        
        // Filter results based on search box
        function filterResults() {
            const searchText = document.getElementById('search-box').value.toLowerCase();
            
            if (!searchText) {
                filteredData = [...dbData]; // Reset to full data
                document.getElementById('search-stats').innerHTML = dbData.length + ' records found';
                displayDBData(filteredData);
                return;
            }
            
            filteredData = dbData.filter(record => {
                // Search in URL
                if (record.url.toLowerCase().includes(searchText)) return true;
                
                // Search in data
                let dataContent = '';
                if (record.data_type === 'emails' || record.data_type === 'dead_links' || record.data_type === 'dead_domains') {
                    dataContent = Array.isArray(record.raw_data) ? record.raw_data.join(' ') : String(record.raw_data);
                } else if (record.data_type === 'keywords') {
                    if (typeof record.raw_data === 'object') {
                        dataContent = Object.keys(record.raw_data).join(' ');
                    } else {
                        dataContent = String(record.raw_data);
                    }
                } else {
                    dataContent = JSON.stringify(record.raw_data);
                }
                
                return dataContent.toLowerCase().includes(searchText);
            });
            
            document.getElementById('search-stats').innerHTML = filteredData.length + ' of ' + dbData.length + ' records found';
            displayDBData(filteredData);
        }
        
        // View record details in modal
        function viewRecord(id) {
            const record = dbData.find(r => r.id === id);
            if (!record) return;
            
            const modal = document.getElementById('record-modal');
            const title = document.getElementById('modal-title');
            const content = document.getElementById('modal-content');
            
            title.textContent = 'Record Details: ' + record.data_type;
            
            let html = '<div style="margin-bottom: 15px;">';
            html += '<strong>URL:</strong> <a href="' + record.url + '" target="_blank">' + record.url + '</a><br>';
            html += '<strong>Processed At:</strong> ' + new Date(record.processed_at).toLocaleString() + '<br>';
            html += '<strong>Status Code:</strong> ' + record.status_code + '<br>';
            html += '<strong>Process Time:</strong> ' + record.process_time_ms.toFixed(2) + ' ms<br>';
            
            if (record.has_error) {
                html += '<strong>Error:</strong> <span style="color: #f44336;">' + record.error_message + '</span><br>';
            }
            
            html += '</div>';
            
            html += '<div style="margin-bottom: 15px;">';
            html += '<strong>Data Type:</strong> ' + record.data_type + '<br>';
            html += '<strong>Data Count:</strong> ' + record.data_count + '<br>';
            html += '</div>';
            
            html += '<div>';
            html += '<strong>Raw Data:</strong><br>';
            html += '<pre>' + JSON.stringify(record.raw_data, null, 2) + '</pre>';
            html += '</div>';
            
            content.innerHTML = html;
            modal.style.display = 'block';
        }
        
        // Close modal
        function closeModal() {
            document.getElementById('record-modal').style.display = 'none';
        }
        
        // Load database stats
        async function loadDBStats() {
            const statsContainer = document.getElementById('db-stats-content');
            statsContainer.innerHTML = '<div class="loading">Loading statistics...</div>';
            
            try {
                // Load all data to generate stats
                const response = await fetch('/api/db-view?type=all&limit=1000');
                const data = await response.json();
                
                if (data.length === 0) {
                    statsContainer.innerHTML = 'No data available for statistics';
                    return;
                }
                
                // Generate statistics
                const stats = calculateStats(data);
                
                // Display statistics
                let html = '<table class="table">';
                html += '<tr><td><strong>Total Records</strong></td><td>' + stats.totalRecords + '</td></tr>';
                html += '<tr><td><strong>Unique URLs</strong></td><td>' + stats.uniqueURLs.size + '</td></tr>';
                html += '<tr><td><strong>Email Records</strong></td><td>' + stats.emailRecords + '</td></tr>';
                html += '<tr><td><strong>Unique Emails</strong></td><td>' + stats.uniqueEmails.size + '</td></tr>';
                html += '<tr><td><strong>Keyword Records</strong></td><td>' + stats.keywordRecords + '</td></tr>';
                html += '<tr><td><strong>Unique Keywords</strong></td><td>' + stats.uniqueKeywords.size + '</td></tr>';
                html += '<tr><td><strong>Dead Link Records</strong></td><td>' + stats.deadLinkRecords + '</td></tr>';
                html += '<tr><td><strong>Unique Dead Links</strong></td><td>' + stats.uniqueDeadLinks.size + '</td></tr>';
                html += '<tr><td><strong>Average Process Time</strong></td><td>' + stats.avgProcessTime.toFixed(2) + ' ms</td></tr>';
                html += '<tr><td><strong>Success Rate</strong></td><td>' + stats.successRate.toFixed(2) + '%</td></tr>';
                html += '</table>';
                
                statsContainer.innerHTML = html;
                
                // Draw charts
                drawDistributionChart(stats);
                drawTimelineChart(data);
                
            } catch (error) {
                console.error('Error loading stats:', error);
                statsContainer.innerHTML = 'Error loading statistics: ' + error.message;
            }
        }
        
        // Calculate statistics
        function calculateStats(data) {
            const stats = {
                totalRecords: data.length,
                uniqueURLs: new Set(),
                emailRecords: 0,
                uniqueEmails: new Set(),
                keywordRecords: 0,
                uniqueKeywords: new Set(),
                deadLinkRecords: 0,
                uniqueDeadLinks: new Set(),
                totalProcessTime: 0,
                errorCount: 0,
            };
            
            data.forEach(record => {
                stats.uniqueURLs.add(record.url);
                stats.totalProcessTime += record.process_time_ms;
                
                if (record.has_error) stats.errorCount++;
                
                if (record.data_type === 'emails') {
                    stats.emailRecords++;
                    if (Array.isArray(record.raw_data)) {
                        record.raw_data.forEach(email => stats.uniqueEmails.add(email));
                    }
                } else if (record.data_type === 'keywords') {
                    stats.keywordRecords++;
                    if (typeof record.raw_data === 'object') {
                        Object.keys(record.raw_data).forEach(keyword => stats.uniqueKeywords.add(keyword));
                    }
                } else if (record.data_type === 'dead_links' || record.data_type === 'dead_domains') {
                    stats.deadLinkRecords++;
                    if (Array.isArray(record.raw_data)) {
                        record.raw_data.forEach(link => stats.uniqueDeadLinks.add(link));
                    }
                }
            });
            
            stats.avgProcessTime = stats.totalProcessTime / stats.totalRecords;
            stats.successRate = ((stats.totalRecords - stats.errorCount) / stats.totalRecords) * 100;
            
            return stats;
        }
        
        // Draw distribution chart
        function drawDistributionChart(stats) {
            const ctx = document.getElementById('data-distribution-chart').getContext('2d');
            
            // Prepare data
            const data = {
                labels: ['Emails', 'Keywords', 'Dead Links', 'Others'],
                datasets: [{
                    label: 'Distribution of Data Types',
                    data: [
                       
                        stats.emailRecords, 
                        stats.keywordRecords, 
                        stats.deadLinkRecords,
                        stats.totalRecords - stats.emailRecords - stats.keywordRecords - stats.deadLinkRecords
                    ],
                    backgroundColor: [
                        'rgba(54, 162, 235, 0.6)',
                        'rgba(75, 192, 192, 0.6)',
                        'rgba(255, 99, 132, 0.6)',
                        'rgba(255, 206, 86, 0.6)'
                    ],
                    borderColor: [
                        'rgba(54, 162, 235, 1)',
                        'rgba(75, 192, 192, 1)',
                        'rgba(255, 99, 132, 1)',
                        'rgba(255, 206, 86, 1)'
                    ],
                    borderWidth: 1
                }]
            };
            
            // Create chart
            const distributionChart = new Chart(ctx, {
                type: 'pie',
                data: data,
                options: {
                    responsive: true,
                    plugins: {
                        legend: {
                            position: 'right',
                        },
                        title: {
                            display: true,
                            text: 'Distribution of Data Types'
                        }
                    }
                });
        }
        
        // Draw timeline chart
        function drawTimelineChart(data) {
            const ctx = document.getElementById('timeline-chart').getContext('2d');
            
            // Group data by hour
            const timeData = {};
            data.forEach(record => {
                const date = new Date(record.processed_at);
                const hour = date.toLocaleString('en-US', {
                    year: 'numeric',
                    month: 'short',
                    day: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
                
                if (!timeData[hour]) {
                    timeData[hour] = {
                        emails: 0,
                        keywords: 0,
                        deadLinks: 0
                    };
                }
                
                if (record.data_type === 'emails') {
                    timeData[hour].emails++;
                } else if (record.data_type === 'keywords') {
                    timeData[hour].keywords++;
                } else if (record.data_type === 'dead_links' || record.data_type === 'dead_domains') {
                    timeData[hour].deadLinks++;
                }
            });
            
            // Convert to arrays for ChartJS
            const labels = Object.keys(timeData).sort();
            const emails = labels.map(label => timeData[label].emails);
            const keywords = labels.map(label => timeData[label].keywords);
            const deadLinks = labels.map(label => timeData[label].deadLinks);
            
            // Create chart
            const timelineChart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [
                        {
                            label: 'Emails',
                            data: emails,
                            borderColor: 'rgba(54, 162, 235, 1)',
                            backgroundColor: 'rgba(54, 162, 235, 0.1)',
                            fill: true,
                            tension: 0.3
                        },
                        {
                            label: 'Keywords',
                            data: keywords,
                            borderColor: 'rgba(75, 192, 192, 1)',
                            backgroundColor: 'rgba(75, 192, 192, 0.1)',
                            fill: true,
                            tension: 0.3
                        },
                        {
                            label: 'Dead Links',
                            data: deadLinks,
                            borderColor: 'rgba(255, 99, 132, 1)',
                            backgroundColor: 'rgba(255, 99, 132, 0.1)',
                            fill: true,
                            tension: 0.3
                        }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: {
                        title: {
                            display: true,
                            text: 'Data Collection Timeline'
                        }
                    },
                    scales: {
                        x: {
                            title: {
                                display: true,
                                text: 'Date/Time'
                            },
                            ticks: {
                                maxRotation: 45,
                                minRotation: 45
                            }
                        },
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: 'Count'
                            }
                        }
                    }
                }
            });
        }
        
        // Export database data as JSON
        function exportDBData() {
            const data = JSON.stringify(filteredData, null, 2);
            const blob = new Blob([data], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'golamv2-db-export-' + new Date().toISOString().split('T')[0] + '.json';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        }
        
        // Initialize when DOM is ready
        document.addEventListener('DOMContentLoaded', function() {
            // Load initial data
            loadDBData();
            
            // Close modal on escape key
            document.addEventListener('keydown', function(event) {
                if (event.key === 'Escape') {
                    closeModal();
                }
            });
            
            // Close modal when clicking outside
            document.getElementById('record-modal').addEventListener('click', function(event) {
                if (event.target === this) {
                    closeModal();
                }
            });
        });
    </script>
</body>
</html>
`

	t, err := template.New("dbDashboard").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, nil)
}

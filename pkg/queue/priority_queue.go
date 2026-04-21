package queue

import (
	"container/heap"
	"sync"

	"github.com/benar-m/golamv2/internal/domain"
)

const (
	MaxQueueSize    = 100000 // Increased from 50k for better throughput - roughly 80mb for normal urls
	RefillThreshold = 0.2    // Refill when queue is <20% full (more aggressive)
)

type PriorityURLQueue struct {
	mu              sync.RWMutex
	heap            *urlHeap
	storage         domain.Storage
	maxSize         int
	refillThreshold int
	refilling       bool
}

// urlItem represents an item in the priority queue
type urlItem struct {
	task     domain.URLTask
	priority int64 // Lower priority number = higher priority
	index    int
}

// urlHeep implements heap.Interface
type urlHeap []*urlItem

func (h urlHeap) Len() int { return len(h) }

func (h urlHeap) Less(i, j int) bool {
	// Lower priority number means higher priority
	return h[i].priority < h[j].priority
}

func (h urlHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *urlHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*urlItem)
	item.index = n
	*h = append(*h, item)
}

func (h *urlHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// NewPriorityURLQueue creates a new priority URL queue
func NewPriorityURLQueue(storage domain.Storage) *PriorityURLQueue {
	q := &PriorityURLQueue{
		heap:            &urlHeap{},
		storage:         storage,
		maxSize:         MaxQueueSize,
		refillThreshold: int(float64(MaxQueueSize) * RefillThreshold),
		refilling:       false,
	}
	heap.Init(q.heap)
	return q
}

// Push adds a URL task to the queue
func (q *PriorityURLQueue) Push(task domain.URLTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.heap.Len() >= q.maxSize {
		return ErrQueueFull
	}

	// Priority based on depth (lower depth = higher priority) and timestamp
	priority := int64(task.Depth*1000) + task.Timestamp.Unix()

	item := &urlItem{
		task:     task,
		priority: priority,
	}

	heap.Push(q.heap, item)

	return nil
}

// remove and returns the highest priority URL task
func (q *PriorityURLQueue) Pop() (domain.URLTask, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.heap.Len() == 0 {
		return domain.URLTask{}, ErrQueueEmpty
	}

	item := heap.Pop(q.heap).(*urlItem)

	// Check if we need to refill from database
	if q.heap.Len() < q.refillThreshold && !q.refilling {
		go q.refillFromDB()
	}

	return item.task, nil
}

// Size returns the current size of the queue
func (q *PriorityURLQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.heap.Len()
}

// IsFull checks if the queue is full
func (q *PriorityURLQueue) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.heap.Len() >= q.maxSize
}

// IsEmpty checks if the queue is empty
func (q *PriorityURLQueue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.heap.Len() == 0
}

// refillFromDB fills the queue from the database
func (q *PriorityURLQueue) refillFromDB() {
	q.mu.Lock()
	if q.refilling {
		q.mu.Unlock()
		return
	}
	q.refilling = true
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		q.refilling = false
		q.mu.Unlock()
	}()

	// Calculate how many URLs we need
	currentSize := q.Size()
	needed := q.maxSize - currentSize

	if needed <= 0 {
		return
	}

	// Fetch URLs from database
	urls, err := q.storage.GetURLs(needed)
	if err != nil {
		return
	}

	// Add URLs to queue
	for _, task := range urls {
		if err := q.Push(task); err != nil {
			break // Queue might be full
		}
	}
}

// Close closes the queue
func (q *PriorityURLQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Clear the heap
	*q.heap = (*q.heap)[:0]
	return nil
}

// GetMemoryUsageMB estimated memory usage
func (q *PriorityURLQueue) GetMemoryUsageMB() float64 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.heap == nil {
		return 0
	}

	// Estimate memory usage based on queue size
	// Each URLTask is approximately 300 bytes (URL string + metadata)
	// With 50k max URLs: 50k * 300 bytes = ~15MB
	//My Rough Estimates from my tests!, May vary based on URL length and metadata encountered
	currentSize := len(*q.heap)
	bytesPerTask := 300.0

	return float64(currentSize) * bytesPerTask / 1024 / 1024
}

// Custom errors
var (
	ErrQueueFull  = &QueueError{Message: "queue is full"}
	ErrQueueEmpty = &QueueError{Message: "queue is empty"}
)

type QueueError struct {
	Message string
}

func (e *QueueError) Error() string {
	return e.Message
}

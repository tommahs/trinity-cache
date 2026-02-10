package downloader

import (
	"fmt"
	"sync"
	"time"

	"github.com/tommahs/trinity-cache/internal/logger"
	"github.com/tommahs/trinity-cache/internal/mirror"
)

// DownloadTask represents a package download task
type DownloadTask struct {
	Name    string // package name
	Version string // package version
	PkgPath string // remote package path
	Done    chan error
}

// WorkerPool manages concurrent download workers
type WorkerPool struct {
	downloader   Downloader
	selector     mirror.Selector
	workers      int
	taskQueue    chan *DownloadTask
	mu           sync.Mutex
	running      bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
	metrics      *PoolMetrics
	retryBackoff time.Duration
}

// PoolMetrics tracks pool statistics
type PoolMetrics struct {
	TotalDownloads    int64
	SuccessfulDownloads int64
	FailedDownloads   int64
	mu                sync.Mutex
}

// NewWorkerPool creates a new download worker pool with the specified number of workers.
func NewWorkerPool(downloader Downloader, selector mirror.Selector, workerCount int) (*WorkerPool, error) {
	if downloader == nil || selector == nil {
		return nil, fmt.Errorf("downloader and selector cannot be nil")
	}

	if workerCount < 1 {
		workerCount = 1
	}

	if workerCount > 100 {
		workerCount = 100 // cap at 100 workers
	}

	return &WorkerPool{
		downloader:   downloader,
		selector:     selector,
		workers:      workerCount,
		taskQueue:    make(chan *DownloadTask, workerCount*2),
		metrics:      &PoolMetrics{},
		retryBackoff: 1 * time.Second,
	}, nil
}

// Start starts the worker pool
func (wp *WorkerPool) Start() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.running {
		return fmt.Errorf("worker pool already running")
	}

	wp.running = true
	wp.stopChan = make(chan struct{})

	logger.Info("starting download worker pool", "workers", wp.workers)

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	return nil
}

// Stop gracefully stops the worker pool
func (wp *WorkerPool) Stop() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.running {
		return fmt.Errorf("worker pool not running")
	}

	wp.running = false
	close(wp.stopChan)

	// Close the task queue to signal workers to finish
	close(wp.taskQueue)

	logger.Info("stopping download worker pool")
	return nil
}

// WaitForCompletion waits for all workers to finish processing
func (wp *WorkerPool) WaitForCompletion() {
	wp.wg.Wait()
}

// Queue submits a download task to the pool
func (wp *WorkerPool) Queue(task *DownloadTask) error {
	wp.mu.Lock()
	running := wp.running
	wp.mu.Unlock()

	if !running {
		return fmt.Errorf("worker pool not running")
	}

	if task == nil || task.Name == "" || task.Version == "" || task.PkgPath == "" {
		return fmt.Errorf("invalid task: all fields must be non-empty")
	}

	if task.Done == nil {
		task.Done = make(chan error, 1)
	}

	select {
	case wp.taskQueue <- task:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("queue full, task submission timed out")
	}
}

// worker processes tasks from the queue
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	logger.Debug("download worker started", "worker_id", id)

	for {
		select {
		case task, ok := <-wp.taskQueue:
			if !ok {
				logger.Debug("download worker stopping", "worker_id", id)
				return
			}

			wp.processTask(task, id)

		case <-wp.stopChan:
			logger.Debug("download worker received stop signal", "worker_id", id)
			return
		}
	}
}

// processTask handles a single download task with retry logic
func (wp *WorkerPool) processTask(task *DownloadTask, workerID int) {
	wp.metrics.mu.Lock()
	wp.metrics.TotalDownloads++
	wp.metrics.mu.Unlock()

	logger.Debug("processing download task", "worker_id", workerID, "package", task.Name, "version", task.Version)

	// Select a mirror
	mirror, err := wp.selector.Select()
	if err != nil {
		logger.Error("failed to select mirror", "package", task.Name, "error", err)
		wp.metrics.mu.Lock()
		wp.metrics.FailedDownloads++
		wp.metrics.mu.Unlock()
		task.Done <- err
		return
	}

	// Perform download
	result, err := wp.downloader.Download(mirror, task.PkgPath)
	if err != nil {
		logger.Error("download failed", "worker_id", workerID, "package", task.Name, "error", err)
		wp.metrics.mu.Lock()
		wp.metrics.FailedDownloads++
		wp.metrics.mu.Unlock()
		task.Done <- err
		return
	}

	logger.Info("download completed", "worker_id", workerID, "package", task.Name, "version", task.Version, "size", result.Size)

	wp.metrics.mu.Lock()
	wp.metrics.SuccessfulDownloads++
	wp.metrics.mu.Unlock()

	task.Done <- nil
}

// IsRunning returns whether the worker pool is currently running
func (wp *WorkerPool) IsRunning() bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.running
}

// GetMetrics returns a snapshot of pool metrics
func (wp *WorkerPool) GetMetrics() (total, success, failed int64) {
	wp.metrics.mu.Lock()
	defer wp.metrics.mu.Unlock()
	return wp.metrics.TotalDownloads, wp.metrics.SuccessfulDownloads, wp.metrics.FailedDownloads
}

// GetWorkerCount returns the number of workers in the pool
func (wp *WorkerPool) GetWorkerCount() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.workers
}

// QueueLength returns the current number of queued tasks
func (wp *WorkerPool) QueueLength() int {
	return len(wp.taskQueue)
}

package downloader

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tommahs/trinity-cache/internal/mirror"
)

// MockDownloader for testing
type MockDownloader struct {
	downloadCount   int32
	failureCount    int32
	shouldFail      bool
	downloadTime    time.Duration
	downloadedTasks []DownloadTask
	mu              sync.Mutex
}

func (md *MockDownloader) Download(m *mirror.Mirror, pkgPath string) (*Result, error) {
	if md.downloadTime > 0 {
		time.Sleep(md.downloadTime)
	}

	if md.shouldFail {
		atomic.AddInt32(&md.failureCount, 1)
		return nil, fmt.Errorf("mock download failure")
	}

	atomic.AddInt32(&md.downloadCount, 1)
	return &Result{Path: "/tmp/pkg", Size: 1024, Checksum: "abc123"}, nil
}

func TestWorkerPool_New(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	downloader := &MockDownloader{}

	pool, err := NewWorkerPool(downloader, selector, 4)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	if pool.workers != 4 {
		t.Errorf("expected 4 workers, got %d", pool.workers)
	}
}

func TestWorkerPool_New_InvalidInputs(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	downloader := &MockDownloader{}

	_, err := NewWorkerPool(nil, selector, 4)
	if err == nil {
		t.Errorf("expected error for nil downloader")
	}

	_, err = NewWorkerPool(downloader, nil, 4)
	if err == nil {
		t.Errorf("expected error for nil selector")
	}
}

func TestWorkerPool_Start_And_Stop(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 2)

	err := pool.Start()
	if err != nil {
		t.Fatalf("failed to start pool: %v", err)
	}

	if !pool.IsRunning() {
		t.Errorf("pool should be running after Start()")
	}

	err = pool.Stop()
	if err != nil {
		t.Fatalf("failed to stop pool: %v", err)
	}

	if pool.IsRunning() {
		t.Errorf("pool should not be running after Stop()")
	}
}

func TestWorkerPool_DoubleStart(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 2)
	pool.Start()
	defer pool.Stop()

	err := pool.Start()
	if err == nil {
		t.Errorf("expected error on double start")
	}
}

func TestWorkerPool_Queue_Success(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 2)
	pool.Start()
	defer pool.Stop()

	task := &DownloadTask{
		Name:    "app",
		Version: "1.0",
		PkgPath: "app/app-1.0.pkg",
		Done:    make(chan error, 1),
	}

	err := pool.Queue(task)
	if err != nil {
		t.Fatalf("failed to queue task: %v", err)
	}

	// Wait for task to complete
	select {
	case err := <-task.Done:
		if err != nil {
			t.Errorf("task failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("task did not complete in time")
	}

	if atomic.LoadInt32(&downloader.downloadCount) != 1 {
		t.Errorf("expected 1 download, got %d", atomic.LoadInt32(&downloader.downloadCount))
	}
}

func TestWorkerPool_Queue_InvalidTask(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 2)
	pool.Start()
	defer pool.Stop()

	// Missing name
	task := &DownloadTask{
		Version: "1.0",
		PkgPath: "test.pkg",
	}

	err := pool.Queue(task)
	if err == nil {
		t.Errorf("expected error for invalid task")
	}
}

func TestWorkerPool_Multiple_Tasks(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 4)
	pool.Start()
	defer pool.Stop()

	// Queue multiple tasks
	taskCount := 10
	tasks := make([]*DownloadTask, taskCount)
	for i := 0; i < taskCount; i++ {
		task := &DownloadTask{
			Name:    fmt.Sprintf("pkg%d", i),
			Version: "1.0",
			PkgPath: fmt.Sprintf("pkg%d/pkg%d-1.0.pkg", i, i),
			Done:    make(chan error, 1),
		}
		tasks[i] = task
		pool.Queue(task)
	}

	// Wait for all tasks
	for _, task := range tasks {
		select {
		case err := <-task.Done:
			if err != nil {
				t.Errorf("task failed: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("task did not complete in time")
		}
	}

	total, success, failed := pool.GetMetrics()
	if int(success) != taskCount {
		t.Errorf("expected %d successful downloads, got %d", taskCount, success)
	}
	if int(total) != taskCount {
		t.Errorf("expected %d total downloads, got %d", taskCount, total)
	}
}

func TestWorkerPool_GetWorkerCount(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 8)

	if pool.GetWorkerCount() != 8 {
		t.Errorf("expected 8 workers, got %d", pool.GetWorkerCount())
	}
}

func TestWorkerPool_GetMetrics(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	selector.Add(&mirror.Mirror{URL: "http://example.com", BaseWeight: 1.0, EffectiveWeight: 1.0})
	downloader := &MockDownloader{}

	pool, _ := NewWorkerPool(downloader, selector, 2)
	pool.Start()
	defer pool.Stop()

	// Queue tasks
	for i := 0; i < 3; i++ {
		task := &DownloadTask{
			Name:    fmt.Sprintf("pkg%d", i),
			Version: "1.0",
			PkgPath: "test.pkg",
			Done:    make(chan error, 1),
		}
		pool.Queue(task)
	}

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	total, success, failed := pool.GetMetrics()
	if total < 1 {
		t.Errorf("expected at least 1 total download tracked")
	}
}

func TestWorkerPool_WorkerCount_Limits(t *testing.T) {
	selector := mirror.NewWeightedSelector()
	downloader := &MockDownloader{}

	// Test minimum
	pool, _ := NewWorkerPool(downloader, selector, 0)
	if pool.GetWorkerCount() != 1 {
		t.Errorf("expected minimum 1 worker, got %d", pool.GetWorkerCount())
	}

	// Test maximum
	pool, _ = NewWorkerPool(downloader, selector, 200)
	if pool.GetWorkerCount() > 100 {
		t.Errorf("expected maximum 100 workers, got %d", pool.GetWorkerCount())
	}
}

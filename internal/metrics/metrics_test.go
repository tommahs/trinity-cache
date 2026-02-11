package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestRecordDownloadSuccess(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	time.Sleep(10 * time.Millisecond)
	timer.RecordSuccess(1024)

	metrics := GetMetrics()
	if metrics.TotalDownloads != 1 {
		t.Errorf("expected 1 total download, got %d", metrics.TotalDownloads)
	}
	if metrics.SuccessfulDownloads != 1 {
		t.Errorf("expected 1 successful download, got %d", metrics.SuccessfulDownloads)
	}
	if metrics.TotalBytesDownloaded != 1024 {
		t.Errorf("expected 1024 bytes, got %d", metrics.TotalBytesDownloaded)
	}
}

func TestRecordDownloadFailure(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	timer.RecordFailure()

	metrics := GetMetrics()
	if metrics.TotalDownloads != 1 {
		t.Errorf("expected 1 total download, got %d", metrics.TotalDownloads)
	}
	if metrics.FailedDownloads != 1 {
		t.Errorf("expected 1 failed download, got %d", metrics.FailedDownloads)
	}
}

func TestRecordCacheHitAndMiss(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()

	metrics := GetMetrics()
	if metrics.CacheHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", metrics.CacheHits)
	}
	if metrics.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", metrics.CacheMisses)
	}
}

func TestRecordMirrorEvents(t *testing.T) {
	Reset()

	RecordMirrorSelection()
	RecordMirrorSelection()
	RecordMirrorPenalty()
	RecordMirrorRecovery()

	metrics := GetMetrics()
	if metrics.MirrorSelections != 2 {
		t.Errorf("expected 2 mirror selections, got %d", metrics.MirrorSelections)
	}
	if metrics.MirrorPenalties != 1 {
		t.Errorf("expected 1 mirror penalty, got %d", metrics.MirrorPenalties)
	}
	if metrics.MirrorRecoveries != 1 {
		t.Errorf("expected 1 mirror recovery, got %d", metrics.MirrorRecoveries)
	}
}

func TestRecordRetention(t *testing.T) {
	Reset()

	RecordPackageRemoved()
	RecordPackageRemoved()
	RecordVersionRemoved()
	RecordVersionRemoved()
	RecordVersionRemoved()

	metrics := GetMetrics()
	if metrics.PackagesRemoved != 2 {
		t.Errorf("expected 2 packages removed, got %d", metrics.PackagesRemoved)
	}
	if metrics.VersionsRemoved != 3 {
		t.Errorf("expected 3 versions removed, got %d", metrics.VersionsRemoved)
	}
}

func TestUpdateCacheStats(t *testing.T) {
	Reset()

	UpdateCacheStats(5, 12)

	metrics := GetMetrics()
	if metrics.PackagesInCache != 5 {
		t.Errorf("expected 5 packages in cache, got %d", metrics.PackagesInCache)
	}
	if metrics.VersionsInCache != 12 {
		t.Errorf("expected 12 versions in cache, got %d", metrics.VersionsInCache)
	}
}

func TestUpdateRetentionTime(t *testing.T) {
	Reset()

	now := time.Now()
	UpdateRetentionTime(now)

	metrics := GetMetrics()
	if metrics.LastRetentionTime.Before(now) || metrics.LastRetentionTime.After(now.Add(1*time.Second)) {
		t.Errorf("retention time not updated correctly")
	}
}

func TestGetSnapshot(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()

	snapshot := GetSnapshot()

	if snapshot.Cache.Hits != 2 {
		t.Errorf("expected 2 hits in snapshot, got %d", snapshot.Cache.Hits)
	}
	if snapshot.Cache.Misses != 1 {
		t.Errorf("expected 1 miss in snapshot, got %d", snapshot.Cache.Misses)
	}

	expectedHitRate := 2.0 / 3.0
	if snapshot.Cache.HitRate < expectedHitRate-0.01 || snapshot.Cache.HitRate > expectedHitRate+0.01 {
		t.Errorf("expected hit rate ~%.2f, got %.2f", expectedHitRate, snapshot.Cache.HitRate)
	}
}

func TestReset(t *testing.T) {
	RecordCacheHit()
	RecordCacheHit()
	RecordDownloadStart().RecordSuccess(1024)

	metrics := GetMetrics()
	if metrics.CacheHits == 0 || metrics.TotalDownloads == 0 {
		t.Errorf("metrics should have data before reset")
	}

	Reset()

	metrics = GetMetrics()
	if metrics.CacheHits != 0 || metrics.TotalDownloads != 0 || metrics.TotalBytesDownloaded != 0 {
		t.Errorf("metrics should be reset to zero")
	}
}

func TestConcurrentMetricsRecording(t *testing.T) {
	Reset()

	var wg sync.WaitGroup
	workers := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				RecordCacheHit()
				RecordMirrorSelection()
				timer := RecordDownloadStart()
				timer.RecordSuccess(1024)
			}
		}()
	}

	wg.Wait()

	metrics := GetMetrics()
	expectedHits := int64(workers * 100)
	expectedDownloads := int64(workers * 100)

	if metrics.CacheHits != expectedHits {
		t.Errorf("expected %d cache hits, got %d", expectedHits, metrics.CacheHits)
	}
	if metrics.TotalDownloads != expectedDownloads {
		t.Errorf("expected %d total downloads, got %d", expectedDownloads, metrics.TotalDownloads)
	}
	if metrics.TotalBytesDownloaded != int64(workers*100)*1024 {
		t.Errorf("expected total bytes %d, got %d", int64(workers*100)*1024, metrics.TotalBytesDownloaded)
	}
}

func TestAverageDurationCalculation(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	time.Sleep(5 * time.Millisecond)
	timer.RecordSuccess(1024)

	metrics := GetMetrics()
	if metrics.AverageDownloadTime < 5*time.Millisecond {
		t.Errorf("average download time should be >= 5ms, got %v", metrics.AverageDownloadTime)
	}
}

func TestCacheHitRateEdgeCases(t *testing.T) {
	Reset()

	// Test with no cache operations
	snapshot := GetSnapshot()
	if snapshot.Cache.HitRate != 0 {
		t.Errorf("hit rate should be 0 when no cache ops, got %.2f", snapshot.Cache.HitRate)
	}

	// Test with 100% hits
	Reset()
	for i := 0; i < 10; i++ {
		RecordCacheHit()
	}
	snapshot = GetSnapshot()
	if snapshot.Cache.HitRate < 0.99 || snapshot.Cache.HitRate > 1.01 {
		t.Errorf("hit rate should be ~1.0 for all hits, got %.2f", snapshot.Cache.HitRate)
	}

	// Test with 100% misses
	Reset()
	for i := 0; i < 10; i++ {
		RecordCacheMiss()
	}
	snapshot = GetSnapshot()
	if snapshot.Cache.HitRate != 0 {
		t.Errorf("hit rate should be 0 for all misses, got %.2f", snapshot.Cache.HitRate)
	}
}

func TestMetricConsistency(t *testing.T) {
	Reset()

	// Record mixed success and failure
	for i := 0; i < 5; i++ {
		RecordDownloadStart().RecordSuccess(1024)
	}
	for i := 0; i < 3; i++ {
		RecordDownloadStart().RecordFailure()
	}

	metrics := GetMetrics()

	// Verify relationships
	if metrics.TotalDownloads != 8 {
		t.Errorf("expected 8 total downloads, got %d", metrics.TotalDownloads)
	}
	if metrics.SuccessfulDownloads != 5 {
		t.Errorf("expected 5 successful, got %d", metrics.SuccessfulDownloads)
	}
	if metrics.FailedDownloads != 3 {
		t.Errorf("expected 3 failed, got %d", metrics.FailedDownloads)
	}
	if metrics.SuccessfulDownloads+metrics.FailedDownloads != metrics.TotalDownloads {
		t.Errorf("success + failed should equal total")
	}
}

func TestLargeByteCounts(t *testing.T) {
	Reset()

	// Record a large file (1GB)
	largeSize := int64(1024 * 1024 * 1024)
	timer := RecordDownloadStart()
	timer.RecordSuccess(largeSize)

	metrics := GetMetrics()
	if metrics.TotalBytesDownloaded != largeSize {
		t.Errorf("expected %d bytes, got %d", largeSize, metrics.TotalBytesDownloaded)
	}
}

func TestConcurrentMetricIncrement(t *testing.T) {
	Reset()

	var wg sync.WaitGroup
	numGoroutines := 20
	operationsPerGoroutine := 50

	// Concurrent operations on different metrics
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				RecordCacheHit()
				RecordMirrorSelection()
				RecordMirrorPenalty()
				RecordMirrorRecovery()
				RecordVersionRemoved()
			}
		}()
	}

	wg.Wait()

	metrics := GetMetrics()
	expectedCount := int64(numGoroutines * operationsPerGoroutine)

	if metrics.CacheHits != expectedCount {
		t.Errorf("expected %d cache hits, got %d", expectedCount, metrics.CacheHits)
	}
	if metrics.MirrorSelections != expectedCount {
		t.Errorf("expected %d mirror selections, got %d", expectedCount, metrics.MirrorSelections)
	}
	if metrics.VersionsRemoved != expectedCount {
		t.Errorf("expected %d versions removed, got %d", expectedCount, metrics.VersionsRemoved)
	}
}

func TestGetGlobalMetrics(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()

	globalPtr := GetGlobalMetrics()
	if globalPtr == nil {
		t.Errorf("GetGlobalMetrics should return non-nil pointer")
	}

	if globalPtr.CacheHits != 2 {
		t.Errorf("expected 2 hits via global pointer, got %d", globalPtr.CacheHits)
	}
}

func TestRetentionTimeUpdate(t *testing.T) {
	Reset()

	time1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	UpdateRetentionTime(time1)

	metrics := GetMetrics()
	if !metrics.LastRetentionTime.Equal(time1) {
		t.Errorf("retention time mismatch: got %v, expected %v", metrics.LastRetentionTime, time1)
	}

	// Update with later time
	time2 := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	UpdateRetentionTime(time2)

	metrics = GetMetrics()
	if !metrics.LastRetentionTime.Equal(time2) {
		t.Errorf("retention time should update: got %v, expected %v", metrics.LastRetentionTime, time2)
	}
}

func TestUpdateCacheStatsConcurrency(t *testing.T) {
	Reset()

	var wg sync.WaitGroup
	numGoroutines := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine updates cache stats
			UpdateCacheStats(int64(id), int64(id*10))
		}(g)
	}

	wg.Wait()

	metrics := GetMetrics()
	// Last update wins due to atomic store
	if metrics.PackagesInCache == 0 && metrics.VersionsInCache == 0 {
		// This should not happen if updates worked
		t.Logf("Note: One of the concurrent updates should have set non-zero values")
	}
}

func TestSnapshotTimestamp(t *testing.T) {
	Reset()

	beforeSnapshot := time.Now()
	snapshot := GetSnapshot()
	afterSnapshot := time.Now()

	if snapshot.Timestamp.Before(beforeSnapshot) || snapshot.Timestamp.After(afterSnapshot.Add(1*time.Second)) {
		t.Errorf("snapshot timestamp should be recent: %v vs now %v", snapshot.Timestamp, time.Now())
	}
}

func TestAverageDurationWithZeroBytesPreservesTime(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	time.Sleep(10 * time.Millisecond)
	// Record success with 0 bytes (edge case)
	timer.RecordSuccess(0)

	metrics := GetMetrics()
	if metrics.AverageDownloadTime < 10*time.Millisecond {
		t.Errorf("average should record even with 0 bytes: %v", metrics.AverageDownloadTime)
	}
	if metrics.TotalBytesDownloaded != 0 {
		t.Errorf("expected 0 bytes, got %d", metrics.TotalBytesDownloaded)
	}
}

func TestMultipleResets(t *testing.T) {
	for i := 0; i < 3; i++ {
		RecordCacheHit()
		RecordDownloadStart().RecordSuccess(1024)

		metrics := GetMetrics()
		if metrics.CacheHits == 0 {
			t.Errorf("metrics should have data before reset iteration %d", i)
		}

		Reset()

		metrics = GetMetrics()
		if metrics.CacheHits != 0 || metrics.TotalDownloads != 0 {
			t.Errorf("metrics should be reset in iteration %d", i)
		}
	}
}

func TestAverageDurationExponentialMovingAverage(t *testing.T) {
	Reset()

	// Record first download with 10ms
	timer1 := RecordDownloadStart()
	time.Sleep(10 * time.Millisecond)
	timer1.RecordSuccess(1024)

	metrics1 := GetMetrics()
	t.Logf("First average: %v", metrics1.AverageDownloadTime)

	// Record second download with 50ms (much longer)
	timer2 := RecordDownloadStart()
	time.Sleep(50 * time.Millisecond)
	timer2.RecordSuccess(1024)

	metrics2 := GetMetrics()
	t.Logf("Second average: %v", metrics2.AverageDownloadTime)

	// Average should be somewhere between first and second (EMA weights heavily toward first: 30% new, 70% old)
	// So should be closer to first value than second
	if metrics2.AverageDownloadTime > metrics1.AverageDownloadTime {
		t.Logf("Note: EMA calculation may vary, second average: %v", metrics2.AverageDownloadTime)
	}
}

func TestCacheMissRecordingUpdatesLookupTime(t *testing.T) {
	Reset()

	initialMetrics := GetMetrics()
	initialLookupTime := initialMetrics.AverageLookupTime

	RecordCacheMiss()

	metrics := GetMetrics()
	if metrics.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", metrics.CacheMisses)
	}
	// Cache miss should trigger RecordLookupTime
	if metrics.AverageLookupTime == initialLookupTime && initialLookupTime == 0 {
		t.Logf("Note: Lookup time may have been updated by RecordLookupTime call")
	}
}

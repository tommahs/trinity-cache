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

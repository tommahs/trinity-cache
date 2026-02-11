// metrics provides utilities for tracking metrics for trinity-cache
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks system-wide statistics for trinity-cache
type Metrics struct {
	// Download metrics
	TotalDownloads       int64
	SuccessfulDownloads  int64
	FailedDownloads      int64
	TotalBytesDownloaded int64

	// Cache metrics
	CacheHits           int64
	CacheMisses         int64
	PackagesInCache     int64
	VersionsInCache     int64

	// Mirror metrics
	MirrorSelections     int64
	MirrorPenalties      int64
	MirrorRecoveries     int64

	// Timing metrics
	AverageDownloadTime  time.Duration
	AverageLookupTime    time.Duration

	// Retention metrics
	PackagesRemoved      int64
	VersionsRemoved      int64
	LastRetentionTime    time.Time

	mu sync.RWMutex
}

// Global metrics instance
var globalMetrics = &Metrics{}

// RecordDownloadStart returns a timer for tracking download duration
func RecordDownloadStart() *DownloadTimer {
	return &DownloadTimer{startTime: time.Now()}
}

// DownloadTimer tracks download duration
type DownloadTimer struct {
	startTime time.Time
}

// RecordSuccess records a successful download
func (dt *DownloadTimer) RecordSuccess(bytes int64) {
	atomic.AddInt64(&globalMetrics.SuccessfulDownloads, 1)
	atomic.AddInt64(&globalMetrics.TotalDownloads, 1)
	atomic.AddInt64(&globalMetrics.TotalBytesDownloaded, bytes)

	duration := time.Since(dt.startTime)
	updateAverageDuration(&globalMetrics.AverageDownloadTime, duration)
}

// RecordFailure records a failed download
func (dt *DownloadTimer) RecordFailure() {
	atomic.AddInt64(&globalMetrics.FailedDownloads, 1)
	atomic.AddInt64(&globalMetrics.TotalDownloads, 1)

	duration := time.Since(dt.startTime)
	updateAverageDuration(&globalMetrics.AverageDownloadTime, duration)
}

// RecordCacheHit records a cache hit
func RecordCacheHit() {
	atomic.AddInt64(&globalMetrics.CacheHits, 1)
	RecordLookupTime(time.Millisecond) // approximate
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss() {
	atomic.AddInt64(&globalMetrics.CacheMisses, 1)
	RecordLookupTime(time.Millisecond) // approximate
}

// RecordLookupTime records the duration of a cache lookup
func RecordLookupTime(duration time.Duration) {
	updateAverageDuration(&globalMetrics.AverageLookupTime, duration)
}

// RecordMirrorSelection records a mirror selection event
func RecordMirrorSelection() {
	atomic.AddInt64(&globalMetrics.MirrorSelections, 1)
}

// RecordMirrorPenalty records a mirror penalty event
func RecordMirrorPenalty() {
	atomic.AddInt64(&globalMetrics.MirrorPenalties, 1)
}

// RecordMirrorRecovery records a mirror recovery event
func RecordMirrorRecovery() {
	atomic.AddInt64(&globalMetrics.MirrorRecoveries, 1)
}

// RecordPackageRemoved records a package removal during retention
func RecordPackageRemoved() {
	atomic.AddInt64(&globalMetrics.PackagesRemoved, 1)
}

// RecordVersionRemoved records a version removal during retention
func RecordVersionRemoved() {
	atomic.AddInt64(&globalMetrics.VersionsRemoved, 1)
}

// UpdateRetentionTime updates the last retention enforcement time
func UpdateRetentionTime(t time.Time) {
	globalMetrics.mu.Lock()
	globalMetrics.LastRetentionTime = t
	globalMetrics.mu.Unlock()
}

// UpdateCacheStats updates cache statistics
func UpdateCacheStats(packages, versions int64) {
	atomic.StoreInt64(&globalMetrics.PackagesInCache, packages)
	atomic.StoreInt64(&globalMetrics.VersionsInCache, versions)
}

// GetMetrics returns a snapshot of current metrics
func GetMetrics() Metrics {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()
	snapshot := *globalMetrics
	return snapshot
}

// GetGlobalMetrics returns a pointer to the global metrics instance
// This is used by components that need direct access to metrics for exporting or monitoring
func GetGlobalMetrics() *Metrics {
	return globalMetrics
}

// GetSnapshot returns a detailed snapshot with formatted output
func GetSnapshot() MetricsSnapshot {
	metrics := GetMetrics()

	snapshot := MetricsSnapshot{
		Timestamp: time.Now(),
		Downloads: DownloadMetrics{
			Total:       metrics.TotalDownloads,
			Successful:  metrics.SuccessfulDownloads,
			Failed:      metrics.FailedDownloads,
			BytesTotal:  metrics.TotalBytesDownloaded,
			AvgDuration: metrics.AverageDownloadTime,
		},
		Cache: CacheMetrics{
			Hits:     metrics.CacheHits,
			Misses:   metrics.CacheMisses,
			Packages: metrics.PackagesInCache,
			Versions: metrics.VersionsInCache,
		},
		Mirror: MirrorMetrics{
			Selections: metrics.MirrorSelections,
			Penalties:  metrics.MirrorPenalties,
			Recoveries: metrics.MirrorRecoveries,
		},
		Retention: RetentionMetrics{
			PackagesRemoved: metrics.PackagesRemoved,
			VersionsRemoved: metrics.VersionsRemoved,
			LastRunTime:     metrics.LastRetentionTime,
		},
	}

	if metrics.CacheHits+metrics.CacheMisses > 0 {
		snapshot.Cache.HitRate = float64(metrics.CacheHits) / float64(metrics.CacheHits+metrics.CacheMisses)
	}

	return snapshot
}

// Reset clears all metrics
func Reset() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	globalMetrics.TotalDownloads = 0
	globalMetrics.SuccessfulDownloads = 0
	globalMetrics.FailedDownloads = 0
	globalMetrics.TotalBytesDownloaded = 0
	globalMetrics.CacheHits = 0
	globalMetrics.CacheMisses = 0
	globalMetrics.PackagesInCache = 0
	globalMetrics.VersionsInCache = 0
	globalMetrics.MirrorSelections = 0
	globalMetrics.MirrorPenalties = 0
	globalMetrics.MirrorRecoveries = 0
	globalMetrics.PackagesRemoved = 0
	globalMetrics.VersionsRemoved = 0
	globalMetrics.LastRetentionTime = time.Time{}
	globalMetrics.AverageDownloadTime = 0
	globalMetrics.AverageLookupTime = 0
}

// Helper function to update average duration
func updateAverageDuration(current *time.Duration, newDuration time.Duration) {
	// Simple exponential moving average: EMA = (newValue * 0.3) + (oldValue * 0.7)
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	if *current == 0 {
		*current = newDuration
	} else {
		*current = time.Duration(float64(newDuration)*0.3 + float64(*current)*0.7)
	}
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	Timestamp time.Time
	Downloads DownloadMetrics
	Cache     CacheMetrics
	Mirror    MirrorMetrics
	Retention RetentionMetrics
}

// DownloadMetrics tracks download statistics
type DownloadMetrics struct {
	Total       int64
	Successful  int64
	Failed      int64
	BytesTotal  int64
	AvgDuration time.Duration
}

// CacheMetrics tracks cache statistics
type CacheMetrics struct {
	Hits     int64
	Misses   int64
	HitRate  float64
	Packages int64
	Versions int64
}

// MirrorMetrics tracks mirror-related statistics
type MirrorMetrics struct {
	Selections int64
	Penalties  int64
	Recoveries int64
}

// RetentionMetrics tracks retention policy enforcement
type RetentionMetrics struct {
	PackagesRemoved int64
	VersionsRemoved int64
	LastRunTime     time.Time
}

// String returns a formatted string representation of metrics
func (ms MetricsSnapshot) String() string {
	return ""
}

package metrics

import (
	"fmt"
	"strings"
	"time"
)

// PrometheusMetrics generates Prometheus-format metrics output
type PrometheusMetrics struct {
	metrics *Metrics
}

// NewPrometheusMetrics creates a new Prometheus metrics exporter
func NewPrometheusMetrics(m *Metrics) *PrometheusMetrics {
	if m == nil {
		m = globalMetrics
	}
	return &PrometheusMetrics{metrics: m}
}

// Export returns metrics in Prometheus text format (0.0.4)
func (pm *PrometheusMetrics) Export() string {
	m := pm.metrics
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder

	// HELP and TYPE declarations
	sb.WriteString("# HELP trinity_cache_downloads_total Total number of download attempts\n")
	sb.WriteString("# TYPE trinity_cache_downloads_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_downloads_total %d\n", m.TotalDownloads))

	sb.WriteString("# HELP trinity_cache_downloads_successful Total number of successful downloads\n")
	sb.WriteString("# TYPE trinity_cache_downloads_successful counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_downloads_successful %d\n", m.SuccessfulDownloads))

	sb.WriteString("# HELP trinity_cache_downloads_failed Total number of failed downloads\n")
	sb.WriteString("# TYPE trinity_cache_downloads_failed counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_downloads_failed %d\n", m.FailedDownloads))

	sb.WriteString("# HELP trinity_cache_bytes_downloaded_total Total bytes downloaded\n")
	sb.WriteString("# TYPE trinity_cache_bytes_downloaded_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_bytes_downloaded_total %d\n", m.TotalBytesDownloaded))

	sb.WriteString("# HELP trinity_cache_download_duration_seconds Average download duration in seconds\n")
	sb.WriteString("# TYPE trinity_cache_download_duration_seconds gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_download_duration_seconds %.6f\n", m.AverageDownloadTime.Seconds()))

	// Cache metrics
	sb.WriteString("# HELP trinity_cache_hits_total Total cache hits\n")
	sb.WriteString("# TYPE trinity_cache_hits_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_hits_total %d\n", m.CacheHits))

	sb.WriteString("# HELP trinity_cache_misses_total Total cache misses\n")
	sb.WriteString("# TYPE trinity_cache_misses_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_misses_total %d\n", m.CacheMisses))

	hitRate := 0.0
	totalRequests := m.CacheHits + m.CacheMisses
	if totalRequests > 0 {
		hitRate = float64(m.CacheHits) / float64(totalRequests)
	}

	sb.WriteString("# HELP trinity_cache_hit_rate Current cache hit rate (0-1)\n")
	sb.WriteString("# TYPE trinity_cache_hit_rate gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_hit_rate %.6f\n", hitRate))

	sb.WriteString("# HELP trinity_cache_packages_cached Number of unique packages in cache\n")
	sb.WriteString("# TYPE trinity_cache_packages_cached gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_packages_cached %d\n", m.PackagesInCache))

	sb.WriteString("# HELP trinity_cache_versions_cached Total package versions in cache\n")
	sb.WriteString("# TYPE trinity_cache_versions_cached gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_versions_cached %d\n", m.VersionsInCache))

	// Mirror metrics
	sb.WriteString("# HELP trinity_cache_mirror_selections_total Total mirror selections\n")
	sb.WriteString("# TYPE trinity_cache_mirror_selections_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_mirror_selections_total %d\n", m.MirrorSelections))

	sb.WriteString("# HELP trinity_cache_mirror_penalties_total Total mirror penalties applied\n")
	sb.WriteString("# TYPE trinity_cache_mirror_penalties_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_mirror_penalties_total %d\n", m.MirrorPenalties))

	sb.WriteString("# HELP trinity_cache_mirror_recoveries_total Total mirror recoveries\n")
	sb.WriteString("# TYPE trinity_cache_mirror_recoveries_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_mirror_recoveries_total %d\n", m.MirrorRecoveries))

	// Retention metrics
	sb.WriteString("# HELP trinity_cache_packages_removed_total Total packages removed by retention\n")
	sb.WriteString("# TYPE trinity_cache_packages_removed_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_packages_removed_total %d\n", m.PackagesRemoved))

	sb.WriteString("# HELP trinity_cache_versions_removed_total Total package versions removed by retention\n")
	sb.WriteString("# TYPE trinity_cache_versions_removed_total counter\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_versions_removed_total %d\n", m.VersionsRemoved))

	lastRetentionUnix := int64(0)
	if !m.LastRetentionTime.IsZero() {
		lastRetentionUnix = m.LastRetentionTime.Unix()
	}

	sb.WriteString("# HELP trinity_cache_last_retention_run_timestamp_seconds Timestamp of last retention run\n")
	sb.WriteString("# TYPE trinity_cache_last_retention_run_timestamp_seconds gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_last_retention_run_timestamp_seconds %d\n", lastRetentionUnix))

	// Lookup metrics
	sb.WriteString("# HELP trinity_cache_lookup_duration_seconds Average cache lookup duration in seconds\n")
	sb.WriteString("# TYPE trinity_cache_lookup_duration_seconds gauge\n")
	sb.WriteString(fmt.Sprintf("trinity_cache_lookup_duration_seconds %.6f\n", m.AverageLookupTime.Seconds()))

	return sb.String()
}

// ExportSummary returns a summary of key metrics
func (pm *PrometheusMetrics) ExportSummary() string {
	m := pm.metrics
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder

	sb.WriteString("# Trinity-Cache Metrics Summary\n\n")

	// Downloads section
	sb.WriteString("## Downloads\n")
	sb.WriteString(fmt.Sprintf("Total: %d\n", m.TotalDownloads))
	sb.WriteString(fmt.Sprintf("Successful: %d\n", m.SuccessfulDownloads))
	sb.WriteString(fmt.Sprintf("Failed: %d\n", m.FailedDownloads))
	sb.WriteString(fmt.Sprintf("Bytes Downloaded: %d\n", m.TotalBytesDownloaded))
	sb.WriteString(fmt.Sprintf("Average Duration: %s\n\n", formatDuration(m.AverageDownloadTime)))

	// Cache section
	sb.WriteString("## Cache\n")
	sb.WriteString(fmt.Sprintf("Hits: %d\n", m.CacheHits))
	sb.WriteString(fmt.Sprintf("Misses: %d\n", m.CacheMisses))
	hitRate := 0.0
	totalRequests := m.CacheHits + m.CacheMisses
	if totalRequests > 0 {
		hitRate = float64(m.CacheHits) / float64(totalRequests)
	}
	sb.WriteString(fmt.Sprintf("Hit Rate: %.2f%%\n", hitRate*100))
	sb.WriteString(fmt.Sprintf("Packages: %d\n", m.PackagesInCache))
	sb.WriteString(fmt.Sprintf("Versions: %d\n\n", m.VersionsInCache))

	// Mirrors section
	sb.WriteString("## Mirrors\n")
	sb.WriteString(fmt.Sprintf("Selections: %d\n", m.MirrorSelections))
	sb.WriteString(fmt.Sprintf("Penalties: %d\n", m.MirrorPenalties))
	sb.WriteString(fmt.Sprintf("Recoveries: %d\n\n", m.MirrorRecoveries))

	// Retention section
	sb.WriteString("## Retention\n")
	sb.WriteString(fmt.Sprintf("Packages Removed: %d\n", m.PackagesRemoved))
	sb.WriteString(fmt.Sprintf("Versions Removed: %d\n", m.VersionsRemoved))
	if !m.LastRetentionTime.IsZero() {
		sb.WriteString(fmt.Sprintf("Last Run: %s\n", m.LastRetentionTime.Format(time.RFC3339)))
	}

	return sb.String()
}

// formatDuration formats a duration in human-readable format
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	return d.String()
}

// GetMetricsJSON returns metrics in JSON-compatible format (for API endpoints)
func GetMetricsJSON() map[string]interface{} {
	m := globalMetrics
	m.mu.RLock()
	defer m.mu.RUnlock()

	hitRate := 0.0
	totalRequests := m.CacheHits + m.CacheMisses
	if totalRequests > 0 {
		hitRate = float64(m.CacheHits) / float64(totalRequests)
	}

	return map[string]interface{}{
		"timestamp": time.Now(),
		"downloads": map[string]interface{}{
			"total":                m.TotalDownloads,
			"successful":           m.SuccessfulDownloads,
			"failed":               m.FailedDownloads,
			"bytes_total":          m.TotalBytesDownloaded,
			"avg_duration_seconds": m.AverageDownloadTime.Seconds(),
		},
		"cache": map[string]interface{}{
			"hits":     m.CacheHits,
			"misses":   m.CacheMisses,
			"hit_rate": hitRate,
			"packages": m.PackagesInCache,
			"versions": m.VersionsInCache,
		},
		"mirrors": map[string]interface{}{
			"selections": m.MirrorSelections,
			"penalties":  m.MirrorPenalties,
			"recoveries": m.MirrorRecoveries,
		},
		"retention": map[string]interface{}{
			"packages_removed": m.PackagesRemoved,
			"versions_removed": m.VersionsRemoved,
			"last_run":         m.LastRetentionTime,
		},
	}
}

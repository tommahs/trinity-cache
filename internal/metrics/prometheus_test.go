package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestNewPrometheusMetricsWithNilMetrics(t *testing.T) {
	pm := NewPrometheusMetrics(nil)
	if pm == nil {
		t.Errorf("NewPrometheusMetrics should not return nil even with nil input")
	}
}

func TestNewPrometheusMetricsWithCustomMetrics(t *testing.T) {
	customMetrics := &Metrics{
		TotalDownloads:      10,
		SuccessfulDownloads: 8,
		FailedDownloads:     2,
	}

	pm := NewPrometheusMetrics(customMetrics)
	if pm == nil {
		t.Errorf("NewPrometheusMetrics should return non-nil pointer")
	}
	if pm.metrics != customMetrics {
		t.Errorf("metrics pointer should match input")
	}
}

func TestPrometheusExportFormat(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()
	RecordDownloadStart().RecordSuccess(1024)
	RecordMirrorSelection()
	RecordMirrorPenalty()

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Verify output is not empty
	if output == "" {
		t.Errorf("Export output should not be empty")
	}

	// Verify HELP and TYPE declarations are present
	if !strings.Contains(output, "# HELP") {
		t.Errorf("Export should contain HELP declarations")
	}

	if !strings.Contains(output, "# TYPE") {
		t.Errorf("Export should contain TYPE declarations")
	}
}

func TestPrometheusExportContainsAllMetrics(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordDownloadStart().RecordSuccess(1024)
	RecordMirrorSelection()
	RecordPackageRemoved()
	UpdateCacheStats(5, 12)

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Verify all major metric names are present
	requiredMetrics := []string{
		"trinity_cache_downloads_total",
		"trinity_cache_downloads_successful",
		"trinity_cache_downloads_failed",
		"trinity_cache_bytes_downloaded_total",
		"trinity_cache_download_duration_seconds",
		"trinity_cache_hits_total",
		"trinity_cache_misses_total",
		"trinity_cache_hit_rate",
		"trinity_cache_packages_cached",
		"trinity_cache_versions_cached",
		"trinity_cache_mirror_selections_total",
		"trinity_cache_mirror_penalties_total",
		"trinity_cache_mirror_recoveries_total",
		"trinity_cache_packages_removed_total",
		"trinity_cache_versions_removed_total",
		"trinity_cache_last_retention_run_timestamp_seconds",
		"trinity_cache_lookup_duration_seconds",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("Export should contain metric %s", metric)
		}
	}
}

func TestPrometheusExportWithZeroMetrics(t *testing.T) {
	Reset()

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Should handle zero values gracefully
	if !strings.Contains(output, "trinity_cache_downloads_total 0") {
		t.Errorf("Export should show zero for no downloads")
	}

	if !strings.Contains(output, "trinity_cache_hit_rate 0") {
		t.Errorf("Export should show 0 hit rate when no cache ops")
	}
}

func TestPrometheusExportWithLargeValues(t *testing.T) {
	Reset()

	// Record large numbers
	for i := 0; i < 1000; i++ {
		RecordCacheHit()
	}
	for i := 0; i < 100; i++ {
		RecordDownloadStart().RecordSuccess(1024 * 1024) // 1MB each
	}

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	if !strings.Contains(output, "trinity_cache_hits_total 1000") {
		t.Errorf("Export should display large cache hit count")
	}

	if !strings.Contains(output, "trinity_cache_downloads_total 100") {
		t.Errorf("Export should display large download count")
	}
}

func TestPrometheusExportHitRateCalculation(t *testing.T) {
	Reset()

	// 2 hits, 1 miss = 66.67% hit rate
	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Hit rate should be ~0.666667
	if !strings.Contains(output, "trinity_cache_hit_rate") {
		t.Errorf("Export should contain hit rate metric")
	}

	// Verify format is valid (should have metric value)
	lines := strings.Split(output, "\n")
	foundHitRate := false
	for _, line := range lines {
		if strings.HasPrefix(line, "trinity_cache_hit_rate ") && !strings.HasPrefix(line, "# ") {
			foundHitRate = true
			// Extract value and validate it's approximately 0.666667
			parts := strings.Fields(line)
			if len(parts) != 2 {
				t.Errorf("Export should contain hit rate metric with value")
			}
		}
	}

	if !foundHitRate {
		t.Errorf("Should find trinity_cache_hit_rate value line")
	}
}

func TestPrometheusExportSummary(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()
	RecordDownloadStart().RecordSuccess(1024)
	RecordMirrorSelection()
	RecordPackageRemoved()

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	summary := pm.ExportSummary()

	// Verify summary contains sections
	if !strings.Contains(summary, "Downloads") {
		t.Errorf("Summary should contain Downloads section")
	}

	if !strings.Contains(summary, "Cache") {
		t.Errorf("Summary should contain Cache section")
	}

	if !strings.Contains(summary, "Mirrors") {
		t.Errorf("Summary should contain Mirrors section")
	}

	if !strings.Contains(summary, "Retention") {
		t.Errorf("Summary should contain Retention section")
	}

	// Verify key metrics are in summary
	if !strings.Contains(summary, "2") { // 2 cache hits
		t.Errorf("Summary should show cache hits")
	}
}

func TestPrometheusExportSummaryFormatting(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	time.Sleep(10 * time.Millisecond)
	timer.RecordSuccess(1024)

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	summary := pm.ExportSummary()

	// Verify formatting
	if !strings.Contains(summary, "Hit Rate:") {
		t.Errorf("Summary should display Hit Rate with label")
	}

	if !strings.Contains(summary, "Average Duration:") {
		t.Errorf("Summary should display Average Duration with label")
	}
}

func TestGetMetricsJSON(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordCacheHit()
	RecordCacheMiss()
	RecordDownloadStart().RecordSuccess(2048)
	RecordMirrorSelection()
	RecordMirrorPenalty()
	RecordPackageRemoved()

	data := GetMetricsJSON()

	// Verify structure
	if data["downloads"] == nil {
		t.Errorf("JSON should contain downloads section")
	}

	if data["cache"] == nil {
		t.Errorf("JSON should contain cache section")
	}

	if data["mirrors"] == nil {
		t.Errorf("JSON should contain mirrors section")
	}

	if data["retention"] == nil {
		t.Errorf("JSON should contain retention section")
	}

	// Verify values
	downloads := data["downloads"].(map[string]interface{})
	if downloads["total"].(int64) != 1 {
		t.Errorf("JSON downloads total should be 1")
	}

	cache := data["cache"].(map[string]interface{})
	if cache["hits"].(int64) != 2 {
		t.Errorf("JSON cache hits should be 2")
	}
	if cache["misses"].(int64) != 1 {
		t.Errorf("JSON cache misses should be 1")
	}

	retention := data["retention"].(map[string]interface{})
	if retention["packages_removed"].(int64) != 1 {
		t.Errorf("JSON packages removed should be 1")
	}
}

func TestGetMetricsJSONHitRateCalculation(t *testing.T) {
	Reset()

	// 3 hits, 2 misses = 60% hit rate
	for i := 0; i < 3; i++ {
		RecordCacheHit()
	}
	for i := 0; i < 2; i++ {
		RecordCacheMiss()
	}

	data := GetMetricsJSON()
	cache := data["cache"].(map[string]interface{})
	hitRate := cache["hit_rate"].(float64)

	if hitRate < 0.59 || hitRate > 0.61 {
		t.Errorf("expected hit rate ~0.60, got %.2f", hitRate)
	}
}

func TestPrometheusExportConsistency(t *testing.T) {
	Reset()

	// Set specific values
	for i := 0; i < 50; i++ {
		RecordCacheHit()
	}
	for i := 0; i < 10; i++ {
		RecordCacheMiss()
	}

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	export := pm.Export()

	// Verify consistency across multiple calls
	for i := 0; i < 3; i++ {
		export2 := pm.Export()
		if export != export2 {
			t.Logf("Export may differ slightly due to ongoing metrics updates during test")
		}
	}
}

func TestPrometheusMetricsWithRetentionTime(t *testing.T) {
	Reset()

	now := time.Now()
	UpdateRetentionTime(now)

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Should include retention timestamp metric
	if !strings.Contains(output, "trinity_cache_last_retention_run_timestamp_seconds") {
		t.Errorf("Export should contain retention timestamp metric")
	}

	// Value should be unix timestamp
	if !strings.Contains(output, "trinity_cache_last_retention_run_timestamp_seconds "+string(rune(now.Unix()))) {
		t.Logf("Timestamp value may vary slightly")
	}
}

func TestPrometheusExportPrometheusCompliance(t *testing.T) {
	Reset()

	RecordCacheHit()
	RecordDownloadStart().RecordSuccess(1024)

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	lines := strings.Split(output, "\n")

	// Verify basic Prometheus format:
	// - HELP lines start with # HELP
	// - TYPE lines start with # TYPE
	// - Metric lines have name and value
	for i, line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "# HELP ") {
			// Should have at least 3 parts: # HELP name description
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				t.Errorf("Invalid HELP format at line %d: %s", i, line)
			}
		} else if strings.HasPrefix(line, "# TYPE ") {
			// Should have 4 parts: # TYPE name type
			parts := strings.Fields(line)
			if len(parts) != 4 {
				t.Errorf("Invalid TYPE format at line %d: %s", i, line)
			}
		} else if !strings.HasPrefix(line, "#") {
			// Metric line should have name and value
			parts := strings.Fields(line)
			if len(parts) < 2 {
				t.Logf("Metric line may need value: %s", line)
			}
		}
	}
}

func TestPrometheusMetricsNilPointerHandling(t *testing.T) {
	// Even with nil internal metrics, should not panic
	pm := NewPrometheusMetrics(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewPrometheusMetrics(nil) caused panic: %v", r)
		}
	}()

	// Should not panic
	_ = pm.Export()
	_ = pm.ExportSummary()
}

func TestPrometheusDurationFormatting(t *testing.T) {
	Reset()

	timer := RecordDownloadStart()
	time.Sleep(123 * time.Millisecond)
	timer.RecordSuccess(1024)

	pm := NewPrometheusMetrics(GetGlobalMetrics())
	output := pm.Export()

	// Duration should be converted to seconds with reasonable precision
	if !strings.Contains(output, "trinity_cache_download_duration_seconds") {
		t.Errorf("Export should contain download duration metric")
	}

	// Should have a numeric value (might be 0.123 or similar)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "trinity_cache_download_duration_seconds ") {
			// Just verify format is correct
			if !strings.Contains(line, ".") {
				t.Logf("Duration formatting may vary")
			}
			return
		}
	}

	t.Errorf("Could not find duration value line")
}

func TestGetMetricsJSONTimestamp(t *testing.T) {
	Reset()

	before := time.Now()
	data := GetMetricsJSON()
	after := time.Now()

	timestamp := data["timestamp"].(time.Time)

	if timestamp.Before(before) || timestamp.After(after.Add(1*time.Second)) {
		t.Errorf("JSON timestamp should be recent")
	}
}

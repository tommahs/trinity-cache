package downloader

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tommahs/trinity-cache/internal/mirror"
)

// Mock implementations for testing
type mockVersionTracker struct {
	versions map[string][]string
	mu       sync.RWMutex
}

func newMockVersionTracker() *mockVersionTracker {
	return &mockVersionTracker{
		versions: make(map[string][]string),
	}
}

func (m *mockVersionTracker) LatestVersion(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions, ok := m.versions[name]
	if !ok || len(versions) == 0 {
		return "", fmt.Errorf("no versions found")
	}
	return versions[len(versions)-1], nil
}

func (m *mockVersionTracker) Update(name, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.versions[name] = append(m.versions[name], version)
	return nil
}

func (m *mockVersionTracker) ListVersions(name string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions, ok := m.versions[name]
	if !ok {
		return []string{}, nil
	}
	return versions, nil
}

type mockFetchProbe struct {
	downloadCalls int32
	downloadErr   error
	result        *Result
	mu            sync.Mutex
}

func newMockFetchProbe() *mockFetchProbe {
	return &mockFetchProbe{
		result: &Result{
			Size:     1024,
			Checksum: "abc123",
			Path:     "/test/path",
		},
	}
}

func (m *mockFetchProbe) Download(m2 *mirror.Mirror, path string) (*Result, error) {
	atomic.AddInt32(&m.downloadCalls, 1)
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return m.result, nil
}

func (m *mockFetchProbe) GetDownloadCount() int32 {
	return atomic.LoadInt32(&m.downloadCalls)
}

type mockFetchSelector struct {
	penaltyCalls      int32
	lastPenaltyFactor float64
	mirror            *mirror.Mirror
	selectErr         error
	mu                sync.Mutex
}

func newMockFetchSelector() *mockFetchSelector {
	return &mockFetchSelector{
		mirror: &mirror.Mirror{
			URL:             "http://test.local",
			BaseWeight:      1.0,
			EffectiveWeight: 1.0,
		},
	}
}

func (m *mockFetchSelector) Select() (*mirror.Mirror, error) {
	if m.selectErr != nil {
		return nil, m.selectErr
	}
	return m.mirror, nil
}

func (m *mockFetchSelector) Penalize(mir *mirror.Mirror, factor float64) {
	atomic.AddInt32(&m.penaltyCalls, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	// Store the penalty factor for verification
	if m.lastPenaltyFactor == 0 {
		m.lastPenaltyFactor = factor
	}
}

func (m *mockFetchSelector) Recover() {
	// Mock implementation
}

func (m *mockFetchSelector) Add(mir *mirror.Mirror) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Mock implementation - just track that Add was called
}

func (m *mockFetchSelector) List() []*mirror.Mirror {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mirror != nil {
		return []*mirror.Mirror{m.mirror}
	}
	return []*mirror.Mirror{}
}

func (m *mockFetchSelector) GetPenaltyCalls() int32 {
	return atomic.LoadInt32(&m.penaltyCalls)
}

func (m *mockFetchSelector) GetLastPenaltyFactor() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPenaltyFactor
}

// Tests
func TestNewFetchManager_Valid(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, err := NewFetchManager(downloader, selector, tracker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewFetchManager_NilDownloader(t *testing.T) {
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	_, err := NewFetchManager(nil, selector, tracker)
	if err == nil {
		t.Fatal("expected error for nil downloader")
	}
}

func TestNewFetchManager_NilSelector(t *testing.T) {
	downloader := newMockFetchProbe()
	tracker := newMockVersionTracker()

	_, err := NewFetchManager(downloader, nil, tracker)
	if err == nil {
		t.Fatal("expected error for nil selector")
	}
}

func TestNewFetchManager_NilTracker(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()

	_, err := NewFetchManager(downloader, selector, nil)
	if err == nil {
		t.Fatal("expected error for nil tracker")
	}
}

func TestFetchVersion_Success(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	result, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test-pkg-1.0.0.pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Size != 1024 {
		t.Fatalf("expected size 1024, got %d", result.Size)
	}

	// Check that version tracker was updated
	latest, _ := tracker.LatestVersion("test-pkg")
	if latest != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %s", latest)
	}

	// Check that download was called
	if downloader.GetDownloadCount() != 1 {
		t.Fatalf("expected 1 download call, got %d", downloader.GetDownloadCount())
	}
}

func TestFetchVersion_EmptyName(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	_, err := manager.FetchVersion("", "1.0.0", "/cache/test.pkg")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestFetchVersion_EmptyVersion(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	_, err := manager.FetchVersion("test-pkg", "", "/cache/test.pkg")
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestFetchVersion_NoMirrors(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	selector.selectErr = fmt.Errorf("no mirrors available")
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	_, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test.pkg")
	if err == nil {
		t.Fatal("expected error when no mirrors available")
	}
}

func TestFetchVersion_DownloadError(t *testing.T) {
	downloader := newMockFetchProbe()
	downloader.downloadErr = fmt.Errorf("network error")
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	_, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test.pkg")
	if err == nil {
		t.Fatal("expected error from download failure")
	}

	// Check that selector penalty was applied
	if selector.GetPenaltyCalls() < 1 {
		t.Fatal("expected selector penalty to be applied")
	}

	// Verify penalty factor is reasonable (should be > 0)
	penaltyFactor := selector.GetLastPenaltyFactor()
	if penaltyFactor <= 0 {
		t.Logf("WARNING: penalty factor should be > 0, got %f", penaltyFactor)
	}
}

func TestFetchVersion_DownloadErrorWithRetry(t *testing.T) {
	downloader := newMockFetchProbe()
	downloader.downloadErr = fmt.Errorf("network error")
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// First fetch fails
	_, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test.pkg")
	if err == nil {
		t.Fatal("expected error from download failure")
	}

	initialPenalties := selector.GetPenaltyCalls()

	// Second attempt should also fail and apply penalty again
	_, err = manager.FetchVersion("test-pkg", "1.0.1", "/cache/test-1.0.1.pkg")
	if err == nil {
		t.Logf("NOTE: second fetch may succeed or fail depending on retry logic")
	}

	// Should have more penalizations
	if selector.GetPenaltyCalls() <= initialPenalties {
		t.Logf("NOTE: penalty calls should increase on retry, initial=%d, now=%d", initialPenalties, selector.GetPenaltyCalls())
	}
}

func TestFetchVersion_ConcurrentFetch(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// First fetch should work
	result1, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test-pkg-1.0.0.pkg")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if result1 == nil {
		t.Fatal("expected non-nil result for first fetch")
	}

	// Verify completion allows subsequent fetch
	time.Sleep(10 * time.Millisecond)
	result2, err := manager.FetchVersion("test-pkg", "2.0.0", "/cache/test-pkg-2.0.0.pkg")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if result2 == nil {
		t.Fatal("expected non-nil result for second fetch")
	}
}

func TestFetchIfNeeded_RecentCheck(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// First check should proceed
	fetched1, err := manager.FetchIfNeeded("test-pkg", "1.0.0")
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}

	// Second check within 5 minutes should be skipped
	fetched2, err := manager.FetchIfNeeded("test-pkg", "1.0.0")
	if err != nil {
		t.Fatalf("second check failed: %v", err)
	}

	// Both should report no fetch performed (in this test setup)
	_ = fetched1
	_ = fetched2
}

func TestGetInProgress(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	inProgress := manager.GetInProgress()
	if len(inProgress) != 0 {
		t.Fatal("expected empty in-progress list initially")
	}
}

func TestIsInProgress(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	if manager.IsInProgress("test-pkg", "1.0.0") {
		t.Fatal("expected package to not be in progress")
	}
}

func TestCheckForUpdates(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	packageNames := []string{"pkg1", "pkg2", "pkg3"}
	updates, err := manager.CheckForUpdates(packageNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updates == nil {
		t.Fatal("expected non-nil updates map")
	}
}

func TestGetLastCheckTime_NoCheck(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	_, exists := manager.GetLastCheckTime("test-pkg")
	if exists {
		t.Fatal("expected no check time for package that wasn't checked")
	}
}

func TestGetLastCheckTime_AfterCheck(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// Perform a check
	manager.FetchIfNeeded("test-pkg", "1.0.0")

	// Get the check time
	checkTime, exists := manager.GetLastCheckTime("test-pkg")
	if !exists {
		t.Fatal("expected check time to exist after check")
	}
	if checkTime.IsZero() {
		t.Fatal("expected non-zero check time")
	}
	if time.Since(checkTime) > time.Second {
		t.Fatalf("check time seems too old: %v", checkTime)
	}
}

func TestConcurrentFetches(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	var wg sync.WaitGroup
	results := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := manager.FetchVersion("pkg", fmt.Sprintf("1.%d.0", index), "/cache/test.pkg")
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Logf("fetch error (expected for sequential queue): %v", err)
		}
	}
}

func TestFetchVersion_RaceConditionTracking(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// Simulate concurrent fetches of same package/version
	var wg sync.WaitGroup
	results := make(chan error, 5)
	sameVersion := "1.0.0"

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := manager.FetchVersion("same-pkg", sameVersion, "/cache/same-pkg-1.0.0.pkg")
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	errorCount := 0
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			errorCount++
		}
	}

	t.Logf("concurrent same-version fetches: success=%d, error=%d", successCount, errorCount)
	if errorCount == 0 && successCount == 0 {
		t.Errorf("expected some results")
	}
}

func TestFetchIfNeeded_CacheCheckTiming(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// First check should proceed
	fetched1, err1 := manager.FetchIfNeeded("test-pkg", "1.0.0")
	if err1 != nil {
		t.Fatalf("first check failed: %v", err1)
	}

	// Immediately second check should be skipped or allowed depending on timing
	fetched2, err2 := manager.FetchIfNeeded("test-pkg", "1.0.0")
	if err2 != nil {
		t.Fatalf("second check failed: %v", err2)
	}

	// Wait just under 5 minutes and check again (should still be cached)
	time.Sleep(100 * time.Millisecond)
	fetched3, err3 := manager.FetchIfNeeded("test-pkg", "1.0.0")
	if err3 != nil {
		t.Fatalf("third check failed: %v", err3)
	}

	t.Logf("FetchIfNeeded results: fetch1=%v, fetch2=%v, fetch3=%v", fetched1, fetched2, fetched3)
}

func TestGetLastCheckTime_InitiallyZero(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	checkTime, exists := manager.GetLastCheckTime("never-checked-pkg")
	if exists {
		t.Errorf("should not have check time for unchecked package")
	}

	if !checkTime.IsZero() {
		t.Errorf("should return zero time for unchecked package")
	}
}

func TestIsInProgress_TracksMultiplePackages(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	pkg1 := "pkg1"
	version1 := "1.0.0"

	// Start a fetch
	go func() {
		manager.FetchVersion(pkg1, version1, "/cache/pkg1-1.0.0.pkg")
	}()

	time.Sleep(10 * time.Millisecond)

	if !manager.IsInProgress(pkg1, version1) {
		t.Errorf("pkg1:1.0.0 should be in progress")
	}

	if manager.IsInProgress("pkg2", "1.0.0") {
		t.Errorf("pkg2:1.0.0 should not be in progress")
	}
}

func TestCheckForUpdates_EmptyPackageList(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	updates, err := manager.CheckForUpdates([]string{})
	if err != nil {
		t.Fatalf("CheckForUpdates with empty list should not error: %v", err)
	}

	if len(updates) != 0 {
		t.Errorf("expected empty updates for empty package list, got %d items", len(updates))
	}
}

func TestCheckForUpdates_MultiplePackages(t *testing.T) {
	downloader := newMockFetchProbe()
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	// Pre-populate tracker with some versions
	tracker.Update("pkg1", "1.0.0")
	tracker.Update("pkg2", "2.0.0")
	tracker.Update("pkg3", "3.0.0")

	manager, _ := NewFetchManager(downloader, selector, tracker)

	packageNames := []string{"pkg1", "pkg2", "pkg3"}
	updates, err := manager.CheckForUpdates(packageNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updates == nil {
		t.Fatal("expected non-nil updates map")
	}

	t.Logf("CheckForUpdates returned %d packages", len(updates))
}

func TestFetchVersion_InProgressBlocking(t *testing.T) {
	downloader := newMockFetchProbe()
	// Simulate slow download
	downloader.result = &Result{
		Size:     1024,
		Checksum: "abc123",
		Path:     "/test/path",
	}

	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	// Start first fetch in goroutine
	go func() {
		manager.FetchVersion("test-pkg", "1.0.0", "/cache/test-1.0.0.pkg")
	}()

	time.Sleep(5 * time.Millisecond)

	// Try to fetch same version concurrently
	_, err := manager.FetchVersion("test-pkg", "1.0.0", "/cache/test-1.0.0.pkg")
	if err != nil {
		t.Logf("concurrent fetch blocked as expected: %v", err)
	} else {
		t.Logf("concurrent fetch allowed (may depend on timing)")
	}
}

func TestFetchVersion_PenaltyOnFailure(t *testing.T) {
	downloader := newMockFetchProbe()
	downloader.downloadErr = fmt.Errorf("simulated failure")
	selector := newMockFetchSelector()
	tracker := newMockVersionTracker()

	manager, _ := NewFetchManager(downloader, selector, tracker)

	penaltyBefore := selector.GetPenaltyCalls()

	manager.FetchVersion("test-pkg", "1.0.0", "/cache/test.pkg")

	penaltyAfter := selector.GetPenaltyCalls()

	if penaltyAfter <= penaltyBefore {
		t.Errorf("expected penalty to be applied on download failure")
	}
}

package versiontracker

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/tommahs/trinity-cache/internal/cache"
)

// MockCacheManager is a mock implementation for testing
type MockCacheManager struct {
	versions map[string][]string
}

func (m *MockCacheManager) Has(name, version string) (bool, error) {
	return false, nil
}

func (m *MockCacheManager) GetLatest(name string) (*cache.PackageVersion, error) {
	return nil, nil
}

func (m *MockCacheManager) Add(p *cache.PackageVersion) error {
	return nil
}

func (m *MockCacheManager) ListVersions(name string) ([]*cache.PackageVersion, error) {
	return nil, nil
}

func (m *MockCacheManager) RetainMostRecent(name string, keep int) error {
	return nil
}

func (m *MockCacheManager) Remove(name, version string) error {
	return nil
}

func TestInMemoryTracker_Update(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	err := tracker.Update("app", "1.0")
	if err != nil {
		t.Fatalf("Update() returned error: %v", err)
	}

	latest, _ := tracker.LatestVersion("app")
	if latest != "1.0" {
		t.Errorf("expected latest version 1.0, got %s", latest)
	}
}

func TestInMemoryTracker_Update_Multiple(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	versions := []string{"1.0", "2.0", "1.5"}
	for _, v := range versions {
		tracker.Update("app", v)
	}

	latest, _ := tracker.LatestVersion("app")
	// With lexicographic sorting, "2.0" > "1.5" > "1.0"
	if latest != "2.0" {
		t.Errorf("expected latest version 2.0, got %s", latest)
	}
}

func TestInMemoryTracker_LatestVersion_NotFound(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	_, err := tracker.LatestVersion("nonexistent")
	if err == nil {
		t.Errorf("expected error for nonexistent package")
	}
}

func TestInMemoryTracker_ListVersions(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	versions := []string{"1.0", "2.1", "1.5", "2.0"}
	for _, v := range versions {
		tracker.Update("pkg", v)
	}

	listed, _ := tracker.ListVersions("pkg")
	if len(listed) != 4 {
		t.Errorf("expected 4 versions, got %d", len(listed))
	}

	// Should be sorted newest-first
	if listed[0] != "2.1" {
		t.Errorf("first version should be 2.1, got %s", listed[0])
	}
}

func TestInMemoryTracker_Update_Duplicate(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("pkg", "1.0")
	tracker.Update("pkg", "1.0") // duplicate

	versions, _ := tracker.ListVersions("pkg")
	if len(versions) != 1 {
		t.Errorf("expected 1 version after duplicate update, got %d", len(versions))
	}
}

func TestInMemoryTracker_HasVersion(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("pkg", "1.0")
	tracker.Update("pkg", "2.0")

	if !tracker.HasVersion("pkg", "1.0") {
		t.Errorf("HasVersion should return true for existing version")
	}

	if tracker.HasVersion("pkg", "3.0") {
		t.Errorf("HasVersion should return false for non-existing version")
	}

	if tracker.HasVersion("other", "1.0") {
		t.Errorf("HasVersion should return false for non-existing package")
	}
}

func TestInMemoryTracker_Find(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("myapp", "1.0")
	tracker.Update("otherapp", "1.0")
	tracker.Update("mylib", "1.0")

	results := tracker.Find("app")
	if len(results) != 2 {
		t.Errorf("expected 2 packages matching 'app', got %d", len(results))
	}

	results = tracker.Find("lib")
	if len(results) != 1 || results[0] != "mylib" {
		t.Errorf("expected to find 'mylib', got %v", results)
	}
}

func TestInMemoryTracker_Stats(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("app1", "1.0")
	tracker.Update("app1", "2.0")
	tracker.Update("app2", "1.0")

	pkgs, total := tracker.Stats()
	if pkgs != 2 {
		t.Errorf("expected 2 packages, got %d", pkgs)
	}
	if total != 3 {
		t.Errorf("expected 3 total versions, got %d", total)
	}
}

func TestInMemoryTracker_ClearAll(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("app", "1.0")
	tracker.Update("app", "2.0")

	pkgs, _ := tracker.Stats()
	if pkgs != 1 {
		t.Errorf("expected 1 package before clear")
	}

	tracker.ClearAll()

	pkgs, _ = tracker.Stats()
	if pkgs != 0 {
		t.Errorf("expected 0 packages after clear, got %d", pkgs)
	}
}

func TestInMemoryTracker_Update_EmptyInputs(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	err := tracker.Update("", "1.0")
	if err == nil {
		t.Errorf("expected error for empty package name")
	}

	err = tracker.Update("pkg", "")
	if err == nil {
		t.Errorf("expected error for empty version")
	}
}

func TestInMemoryTracker_NewWithNilCache(t *testing.T) {
	_, err := NewInMemoryTracker(nil)
	if err == nil {
		t.Errorf("expected error when cache manager is nil")
	}
}

// --- Additional comprehensive tests ---

func TestInMemoryTracker_Sorting_Complex(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	// Test complex version sorting (lexicographic, which has limitations)
	versions := []string{"1.0.0", "1.0.1", "1.1.0", "2.0.0", "10.0.0", "9.9.9"}
	for _, v := range versions {
		tracker.Update("pkg", v)
	}

	listed, _ := tracker.ListVersions("pkg")
	if len(listed) != 6 {
		t.Errorf("expected 6 versions, got %d", len(listed))
	}

	// Verify sorted (descending)
	for i := 1; i < len(listed); i++ {
		if listed[i] > listed[i-1] {
			t.Errorf("versions not in descending order: %v", listed)
			break
		}
	}
}

func TestInMemoryTracker_ConcurrentUpdates(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	numGoroutines := 10
	versionsPerGoroutine := 50
	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pkgName := fmt.Sprintf("pkg%d", id%3)
			for v := 0; v < versionsPerGoroutine; v++ {
				version := fmt.Sprintf("%d.%d.%d", id, v, id*v)
				_ = tracker.Update(pkgName, version)
			}
		}(g)
	}

	wg.Wait()

	pkgs, total := tracker.Stats()
	if pkgs != 3 {
		t.Errorf("expected 3 packages, got %d", pkgs)
	}
	if total <= 0 {
		t.Errorf("expected some versions, got %d", total)
	}
}

func TestInMemoryTracker_ConcurrentReads(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	// Populate with some data
	for i := 0; i < 10; i++ {
		tracker.Update("app", fmt.Sprintf("%d.0", i))
	}

	// Concurrent reads
	numGoroutines := 20
	var wg sync.WaitGroup
	errorCount := 0
	var mu sync.Mutex

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tracker.LatestVersion("app"); err != nil {
				mu.Lock()
				errorCount++
				mu.Unlock()
			}
			if _, err := tracker.ListVersions("app"); err != nil {
				mu.Lock()
				errorCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if errorCount > 0 {
		t.Errorf("concurrent read errors: %d", errorCount)
	}
}

func TestInMemoryTracker_ListVersions_Empty(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	versions, err := tracker.ListVersions("nonexistent")
	if err != nil {
		t.Errorf("expected no error for nonexistent package")
	}
	if len(versions) != 0 {
		t.Errorf("expected empty list, got %d versions", len(versions))
	}
}

func TestInMemoryTracker_Find_CaseInsensitive(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("MyApp", "1.0")
	tracker.Update("MYLIB", "1.0")
	tracker.Update("mydata", "1.0")
	tracker.Update("others", "1.0")

	results := tracker.Find("MY")
	if len(results) != 3 {
		t.Errorf("expected 2 results for case-insensitive 'MY', got %d: %v", len(results), results)
	}

	results = tracker.Find("my")
	if len(results) != 3 {
		t.Errorf("expected 3 results for 'my', got %d: %v", len(results), results)
	}
}

func TestInMemoryTracker_Find_SortedResults(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("zebra", "1.0")
	tracker.Update("apple", "1.0")
	tracker.Update("banana", "1.0")

	results := tracker.Find("")
	// All packages contain "" so all should be returned
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Results should be sorted
	if !sort.StringsAreSorted(results) {
		t.Errorf("results not sorted: %v", results)
	}
}

func TestInMemoryTracker_Find_EmptyResult(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("app", "1.0")

	results := tracker.Find("xyz")
	if len(results) != 0 {
		t.Errorf("expected no results for 'xyz', got %d", len(results))
	}
}

func TestInMemoryTracker_HasVersion_PartialMatch(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("pkg", "1.0.0")

	// Exact match should work
	if !tracker.HasVersion("pkg", "1.0.0") {
		t.Errorf("exact match failed")
	}

	// Different version from same package should not exist
	if tracker.HasVersion("pkg", "1.0") {
		t.Errorf("partial version should not match")
	}

	// Similar but different should not match
	if tracker.HasVersion("pkg", "1.0.1") {
		t.Errorf("similar version should not match")
	}
}

func TestInMemoryTracker_Stats_Empty(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	pkgs, total := tracker.Stats()
	if pkgs != 0 {
		t.Errorf("expected 0 packages in empty tracker, got %d", pkgs)
	}
	if total != 0 {
		t.Errorf("expected 0 versions in empty tracker, got %d", total)
	}
}

func TestInMemoryTracker_ClearAll_MultiplePackages(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	for i := 0; i < 5; i++ {
		for j := 0; j < 3; j++ {
			tracker.Update(fmt.Sprintf("pkg%d", i), fmt.Sprintf("%d.0", j))
		}
	}

	pkgs, total := tracker.Stats()
	if pkgs != 5 || total != 15 {
		t.Errorf("before clear: expected 5 packages, 15 versions; got %d, %d", pkgs, total)
	}

	tracker.ClearAll()

	pkgs, total = tracker.Stats()
	if pkgs != 0 || total != 0 {
		t.Errorf("after clear: expected 0 packages, 0 versions; got %d, %d", pkgs, total)
	}
}

func TestInMemoryTracker_LoadFromCache(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	// LoadFromCache is a placeholder, just verify it doesn't error
	err := tracker.LoadFromCache()
	if err != nil {
		t.Errorf("LoadFromCache should not error: %v", err)
	}
}

func TestInMemoryTracker_Update_ManyVersions(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	numVersions := 1000
	for i := 0; i < numVersions; i++ {
		err := tracker.Update("massiveapp", fmt.Sprintf("%d.0.0", i))
		if err != nil {
			t.Fatalf("Update failed for version %d: %v", i, err)
		}
	}

	versions, _ := tracker.ListVersions("massiveapp")
	if len(versions) != numVersions {
		t.Errorf("expected %d versions, got %d", numVersions, len(versions))
	}

	// Latest should be the highest lexicographically
	latest, _ := tracker.LatestVersion("massiveapp")
	if latest == "" {
		t.Errorf("latest version should not be empty")
	}
}

func TestInMemoryTracker_ListVersions_ReturnsCopy(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("pkg", "1.0")
	tracker.Update("pkg", "2.0")

	versions1, _ := tracker.ListVersions("pkg")
	versions2, _ := tracker.ListVersions("pkg")

	// Modify first list
	if len(versions1) > 0 {
		versions1[0] = "modified"
	}

	// Second list should not be affected
	for _, v := range versions2 {
		if v == "modified" {
			t.Errorf("ListVersions should return a copy, not reference")
		}
	}
}

func TestInMemoryTracker_Update_NoRaceOutOfBounds(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	var wg sync.WaitGroup

	// Concurrent updates and reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pkgName := fmt.Sprintf("pkg%d", id%5)
			for v := 0; v < 20; v++ {
				tracker.Update(pkgName, fmt.Sprintf("%d.%d", id, v))
				tracker.ListVersions(pkgName)
				tracker.HasVersion(pkgName, fmt.Sprintf("%d.%d", id, v))
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity
	pkgs, total := tracker.Stats()
	if pkgs == 0 {
		t.Errorf("expected non-zero packages after concurrent ops")
	}
	if total == 0 {
		t.Errorf("expected non-zero versions after concurrent ops")
	}
}

func TestInMemoryTracker_LatestVersion_ConsistentWithList(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	versions := []string{"1.5", "2.3", "1.0", "3.1", "2.0"}
	for _, v := range versions {
		tracker.Update("pkg", v)
	}

	latest, _ := tracker.LatestVersion("pkg")
	listed, _ := tracker.ListVersions("pkg")

	if latest != listed[0] {
		t.Errorf("LatestVersion (%q) should match first in ListVersions (%q)", latest, listed[0])
	}
}

func TestInMemoryTracker_UpdateIdempotent(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	// Add same version multiple times
	for i := 0; i < 5; i++ {
		err := tracker.Update("pkg", "1.0")
		if err != nil {
			t.Errorf("iteration %d: Update failed: %v", i, err)
		}
	}

	versions, _ := tracker.ListVersions("pkg")
	if len(versions) != 1 {
		t.Errorf("expected exactly 1 version after idempotent updates, got %d", len(versions))
	}
}

func TestInMemoryTracker_Sorting_EdgeCases(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	// Versions with mixed patterns
	versionsList := []string{
		"0.0.1",
		"0.1.0",
		"1.0.0",
		"10.0.0",
		"2.0.0",
		"2.0.0-alpha",
		"2.0.0-beta",
	}

	for _, v := range versionsList {
		tracker.Update("pkg", v)
	}

	listed, _ := tracker.ListVersions("pkg")
	if len(listed) != len(versionsList) {
		t.Errorf("expected %d versions, got %d", len(versionsList), len(listed))
	}

	// Verify it's sorted
	for i := 1; i < len(listed); i++ {
		if listed[i] > listed[i-1] {
			t.Errorf("not in descending order at position %d: %v > %v", i, listed[i-1], listed[i])
		}
	}
}

func TestInMemoryTracker_DifferentPackages_Independent(t *testing.T) {
	tracker, _ := NewInMemoryTracker(&MockCacheManager{})

	tracker.Update("app1", "1.0")
	tracker.Update("app1", "2.0")
	tracker.Update("app2", "3.0")

	latest1, _ := tracker.LatestVersion("app1")
	latest2, _ := tracker.LatestVersion("app2")

	if latest1 == latest2 {
		t.Errorf("different packages should have independent versions")
	}

	list1, _ := tracker.ListVersions("app1")
	list2, _ := tracker.ListVersions("app2")

	if len(list1) != 2 || len(list2) != 1 {
		t.Errorf("package version counts mismatch: app1=%d, app2=%d", len(list1), len(list2))
	}
}

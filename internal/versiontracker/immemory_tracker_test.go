package versiontracker

import (
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

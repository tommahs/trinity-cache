package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemCache_New(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFilesystemCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	if cache.storagePath != tmpDir {
		t.Errorf("storage path mismatch: got %s, want %s", cache.storagePath, tmpDir)
	}
}

func TestFilesystemCache_Add_And_Has(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	// Create a mock package file
	packageDir := filepath.Join(tmpDir, "test-package")
	os.MkdirAll(packageDir, 0755)
	pkgFile := filepath.Join(packageDir, "test-package-1.0.pkg")
	os.WriteFile(pkgFile, []byte("mock content"), 0644)

	// Add the package
	err := cache.Add(&PackageVersion{
		Name:    "test-package",
		Version: "1.0",
		Path:    pkgFile,
	})
	if err != nil {
		t.Fatalf("failed to add package: %v", err)
	}

	// Check if it exists
	exists, err := cache.Has("test-package", "1.0")
	if err != nil {
		t.Fatalf("Has() returned error: %v", err)
	}
	if !exists {
		t.Errorf("package should exist after Add()")
	}
}

func TestFilesystemCache_GetLatest(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "test-pkg")
	os.MkdirAll(packageDir, 0755)

	// Create multiple versions
	versions := []string{"1.0.0", "2.0.0", "1.5.0"}
	for _, v := range versions {
		pkgFile := filepath.Join(packageDir, "test-pkg-"+v+".pkg")
		os.WriteFile(pkgFile, []byte("content"), 0644)
	}

	latest, err := cache.GetLatest("test-pkg")
	if err != nil {
		t.Fatalf("GetLatest() returned error: %v", err)
	}

	// With lexicographic sorting, "2.0.0" > "1.5.0" > "1.0.0"
	if latest == nil || latest.Version != "2.0.0" {
		t.Errorf("got latest version %v, expected 2.0.0", latest)
	}
}

func TestFilesystemCache_ListVersions(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "my-app")
	os.MkdirAll(packageDir, 0755)

	// Create multiple versions
	versions := []string{"1.0", "2.1", "1.5"}
	for _, v := range versions {
		pkgFile := filepath.Join(packageDir, "my-app-"+v+".pkg")
		os.WriteFile(pkgFile, []byte(""), 0644)
	}

	listed, err := cache.ListVersions("my-app")
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}

	if len(listed) != 3 {
		t.Errorf("expected 3 versions, got %d", len(listed))
	}

	// Should be sorted newest-first (lexicographically descending)
	if listed[0].Version != "2.1" {
		t.Errorf("first version should be 2.1, got %s", listed[0].Version)
	}
}

func TestFilesystemCache_ListVersions_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	listed, err := cache.ListVersions("nonexistent")
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}

	if len(listed) != 0 {
		t.Errorf("expected 0 versions for nonexistent package, got %d", len(listed))
	}
}

func TestFilesystemCache_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "pkg")
	os.MkdirAll(packageDir, 0755)
	pkgFile := filepath.Join(packageDir, "pkg-1.0.pkg")
	os.WriteFile(pkgFile, []byte("content"), 0644)

	// Verify it exists
	exists, _ := cache.Has("pkg", "1.0")
	if !exists {
		t.Errorf("package should exist before removal")
	}

	// Remove it
	err := cache.Remove("pkg", "1.0")
	if err != nil {
		t.Fatalf("Remove() returned error: %v", err)
	}

	// Verify it's gone
	exists, _ = cache.Has("pkg", "1.0")
	if exists {
		t.Errorf("package should not exist after removal")
	}
}

func TestFilesystemCache_RetainMostRecent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "app")
	os.MkdirAll(packageDir, 0755)

	// Create 5 versions
	versions := []string{"1.0", "1.1", "1.2", "2.0", "2.1"}
	for _, v := range versions {
		pkgFile := filepath.Join(packageDir, "app-"+v+".pkg")
		os.WriteFile(pkgFile, []byte(""), 0644)
	}

	// Retain only 2 most recent
	err := cache.RetainMostRecent("app", 2)
	if err != nil {
		t.Fatalf("RetainMostRecent() returned error: %v", err)
	}

	// Check remaining versions
	remaining, _ := cache.ListVersions("app")
	if len(remaining) != 2 {
		t.Errorf("expected 2 remaining versions, got %d", len(remaining))
	}

	// Should be 2.1 and 2.0 (highest lexicographic)
	if remaining[0].Version != "2.1" || remaining[1].Version != "2.0" {
		t.Errorf("expected [2.1, 2.0], got [%s, %s]", remaining[0].Version, remaining[1].Version)
	}
}

func TestFilesystemCache_RetainMostRecent_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "pkg")
	os.MkdirAll(packageDir, 0755)
	pkgFile := filepath.Join(packageDir, "pkg-1.0.pkg")
	os.WriteFile(pkgFile, []byte(""), 0644)

	// Retain 5 but only 1 exists
	err := cache.RetainMostRecent("pkg", 5)
	if err != nil {
		t.Fatalf("RetainMostRecent() returned error: %v", err)
	}

	// Should still have 1 version
	remaining, _ := cache.ListVersions("pkg")
	if len(remaining) != 1 {
		t.Errorf("expected 1 version, got %d", len(remaining))
	}
}

func TestFilesystemCache_GetPackagePath(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	path := cache.GetPackagePath("test", "1.0")
	expected := filepath.Join(tmpDir, "test", "test-1.0.pkg")
	if path != expected {
		t.Errorf("got path %s, expected %s", path, expected)
	}
}

func TestFilesystemCache_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	// Create empty package directory
	emptyDir := filepath.Join(tmpDir, "empty-pkg")
	os.MkdirAll(emptyDir, 0755)

	// Create package directory with files
	pkgDir := filepath.Join(tmpDir, "real-pkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "real-pkg-1.0.pkg"), []byte(""), 0644)

	// Run cleanup
	err := cache.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup() returned error: %v", err)
	}

	// Empty directory should be removed
	if _, err := os.Stat(emptyDir); !os.IsNotExist(err) {
		t.Errorf("empty package directory should have been removed")
	}

	// Real package directory should still exist
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		t.Errorf("package directory with files should still exist")
	}
}

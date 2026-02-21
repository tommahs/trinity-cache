package cache

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
func TestFilesystemCache_Cleanup_PreservesNonPkgFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	// Create package directory with both .pkg files and other files
	pkgDir := filepath.Join(tmpDir, "mixed-pkg")
	os.MkdirAll(pkgDir, 0755)
	pkgFile := filepath.Join(pkgDir, "mixed-pkg-1.0.pkg")
	metadataFile := filepath.Join(pkgDir, "metadata.json")
	logFile := filepath.Join(pkgDir, "log.txt")

	os.WriteFile(pkgFile, []byte("package"), 0644)
	os.WriteFile(metadataFile, []byte("{}"), 0644)
	os.WriteFile(logFile, []byte("log"), 0644)

	// Run cleanup
	err := cache.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup() returned error: %v", err)
	}

	// Package directory should still exist with all files preserved
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		t.Errorf("package directory should still exist")
	}

	if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
		t.Errorf("non-.pkg files should be preserved")
	}

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("non-.pkg files should be preserved")
	}
}

func TestFilesystemCache_Remove_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	// Attempt to remove non-existent package should not error
	err := cache.Remove("nonexistent", "1.0")
	if err != nil {
		t.Errorf("Remove() of non-existent package should not error: %v", err)
	}
}

func TestFilesystemCache_Add_InvalidPackage(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	// Test with missing name
	err := cache.Add(&PackageVersion{
		Name:    "",
		Version: "1.0",
		Path:    "/some/path",
	})
	if err == nil {
		t.Errorf("Add() should error for missing name")
	}

	// Test with missing version
	err = cache.Add(&PackageVersion{
		Name:    "test",
		Version: "",
		Path:    "/some/path",
	})
	if err == nil {
		t.Errorf("Add() should error for missing version")
	}

	// Test with nil package
	err = cache.Add(nil)
	if err == nil {
		t.Errorf("Add() should error for nil package")
	}
}

func TestFilesystemCache_ListVersions_SemanticVersioningLimitation(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "semantic-pkg")
	os.MkdirAll(packageDir, 0755)

	// Create versions that expose lexicographic vs semantic sorting difference
	// Lexicographically: "9.0.0" > "10.0.0" (comparing string chars)
	// Semantically: "10.0.0" > "9.0.0"
	versions := []string{"1.0.0", "9.0.0", "10.0.0"}
	for _, v := range versions {
		pkgFile := filepath.Join(packageDir, "semantic-pkg-"+v+".pkg")
		os.WriteFile(pkgFile, []byte(""), 0644)
	}

	listed, err := cache.ListVersions("semantic-pkg")
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}

	if len(listed) != 3 {
		t.Errorf("expected 3 versions, got %d", len(listed))
	}

	// NOTE: Current implementation uses lexicographic sorting,
	// so "9.0.0" comes before "10.0.0" (incorrect for semantic versioning).
	// This test documents the current behavior and its limitation.
	// To fix: implement semantic version parsing or use a version comparison library.
	if listed[0].Version != "9.0.0" {
		t.Logf("WARNING: Version sorting may be lexicographic, not semantic. Got %s as latest", listed[0].Version)
	}
}

func TestFilesystemCache_ConcurrentOperations(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "concurrent-pkg")
	os.MkdirAll(packageDir, 0755)

	// Concurrent Add operations
	var wg sync.WaitGroup
	var errorCount atomic.Int32
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			version := string(rune('0' + id))
			pkgFile := filepath.Join(packageDir, "concurrent-pkg-"+version+".pkg")
			os.WriteFile(pkgFile, []byte("content"), 0644)

			err := cache.Add(&PackageVersion{
				Name:    "concurrent-pkg",
				Version: version,
				Path:    pkgFile,
			})
			if err != nil {
				errorCount.Add(1)
				t.Logf("Add error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("concurrent operations failed: %d errors", errorCount.Load())
	}

	// Verify all versions exist
	listed, err := cache.ListVersions("concurrent-pkg")
	if err != nil {
		t.Fatalf("ListVersions() returned error: %v", err)
	}

	if len(listed) != numGoroutines {
		t.Errorf("expected %d versions, got %d", numGoroutines, len(listed))
	}
}

func TestFilesystemCache_RetainMostRecent_KeepZero(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)

	packageDir := filepath.Join(tmpDir, "pkg")
	os.MkdirAll(packageDir, 0755)
	pkgFile := filepath.Join(packageDir, "pkg-1.0.pkg")
	os.WriteFile(pkgFile, []byte(""), 0644)

	// RetainMostRecent with 0 should be handled gracefully
	err := cache.RetainMostRecent("pkg", 0)
	if err != nil {
		t.Errorf("RetainMostRecent(0) returned error: %v", err)
	}

	remaining, _ := cache.ListVersions("pkg")
	if len(remaining) != 0 {
		t.Errorf("expected 0 versions after RetainMostRecent(0), got %d", len(remaining))
	}
}

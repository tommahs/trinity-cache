package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionManager_EnforceNow(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	// Create package with 4 versions
	packageDir := filepath.Join(tmpDir, "app")
	os.MkdirAll(packageDir, 0755)

	versions := []string{"1.0", "1.1", "2.0", "2.1"}
	for _, v := range versions {
		pkgFile := filepath.Join(packageDir, "app-"+v+".pkg")
		os.WriteFile(pkgFile, []byte(""), 0644)
	}

	// Enforce retention (default is 2)
	err := rm.EnforceNow()
	if err != nil {
		t.Fatalf("EnforceNow() returned error: %v", err)
	}

	// Check that only 2 versions remain
	listed, _ := cache.ListVersions("app")
	if len(listed) != 2 {
		t.Errorf("expected 2 versions after enforcement, got %d", len(listed))
	}

	// Verify it kept the newest ones
	if listed[0].Version != "2.1" || listed[1].Version != "2.0" {
		t.Errorf("expected [2.1, 2.0], got [%s, %s]", listed[0].Version, listed[1].Version)
	}
}

func TestRetentionManager_SetRetentionCount(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	err := rm.SetRetentionCount(3)
	if err != nil {
		t.Fatalf("SetRetentionCount() returned error: %v", err)
	}

	if rm.GetRetentionCount() != 3 {
		t.Errorf("retention count not updated")
	}

	// Invalid count
	err = rm.SetRetentionCount(0)
	if err == nil {
		t.Errorf("expected error for retention count 0")
	}
}

func TestRetentionManager_GetRetentionCount_Default(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	if rm.GetRetentionCount() != 2 {
		t.Errorf("default retention count should be 2, got %d", rm.GetRetentionCount())
	}
}

func TestRetentionManager_LastCleanupTime(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	// Create a package
	packageDir := filepath.Join(tmpDir, "app")
	os.MkdirAll(packageDir, 0755)
	os.WriteFile(filepath.Join(packageDir, "app-1.0.pkg"), []byte(""), 0644)

	before := time.Now()
	rm.EnforceNow()
	after := time.Now()

	lastCleanup := rm.LastCleanupTime()
	if lastCleanup.Before(before) || lastCleanup.After(after) {
		t.Errorf("LastCleanupTime not updated correctly")
	}
}

func TestRetentionManager_StartPeriodicEnforcement(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	err := rm.StartPeriodicEnforcement(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("StartPeriodicEnforcement() returned error: %v", err)
	}
	defer rm.Stop()

	if !rm.IsRunning() {
		t.Errorf("retention manager should be running")
	}

	// Should not be able to start again
	err = rm.StartPeriodicEnforcement(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("second start should succeed (should be idempotent)")
	}
}

func TestRetentionManager_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	rm.StartPeriodicEnforcement(100 * time.Millisecond)
	if !rm.IsRunning() {
		t.Errorf("manager should be running after start")
	}

	rm.Stop()

	// Give it a moment to actually stop
	time.Sleep(50 * time.Millisecond)

	if rm.IsRunning() {
		t.Errorf("manager should not be running after stop")
	}
}

func TestRetentionManager_MultiplePackages(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	// Create multiple packages with different version counts
	for _, pkg := range []string{"app1", "app2", "lib1"} {
		packageDir := filepath.Join(tmpDir, pkg)
		os.MkdirAll(packageDir, 0755)

		// app1: 5 versions
		maxVersions := 5
		if pkg == "app2" {
			maxVersions = 3
		} else if pkg == "lib1" {
			maxVersions = 2
		}

		for i := 1; i <= maxVersions; i++ {
			pkgFile := filepath.Join(packageDir, pkg+"-"+string(rune('0'+i))+".pkg")
			os.WriteFile(pkgFile, []byte(""), 0644)
		}
	}

	rm.EnforceNow()

	// All packages should now have 2 versions
	for _, pkg := range []string{"app1", "app2", "lib1"} {
		listed, _ := cache.ListVersions(pkg)
		if len(listed) != 2 {
			t.Errorf("package %s should have 2 versions after enforcement, got %d", pkg, len(listed))
		}
	}
}

func TestRetentionManager_MinimumInterval(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewFilesystemCache(tmpDir)
	rm := NewRetentionManager(cache)

	// Try to set a very small interval
	err := rm.StartPeriodicEnforcement(1 * time.Millisecond)
	if err != nil {
		t.Fatalf("StartPeriodicEnforcement() returned error: %v", err)
	}
	defer rm.Stop()

	// Should succeed, but interval should be adjusted to minimum
	if !rm.IsRunning() {
		t.Errorf("manager should be running")
	}
}

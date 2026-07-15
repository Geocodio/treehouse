package provision

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymlinkEntries_CreatesSymlinkToMainRepository(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	sharedDir := filepath.Join(mainRepository, "data", "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("creating shared dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	if err := symlinkEntries(mainRepository, instancePath, []string{"data/shared"}); err != nil {
		t.Fatalf("symlinkEntries() returned unexpected error: %v", err)
	}

	link := filepath.Join(instancePath, "data", "shared")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(link): %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", link)
	}

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != sharedDir {
		t.Errorf("symlink target = %q, want %q", target, sharedDir)
	}

	contents, err := os.ReadFile(filepath.Join(link, "file.txt"))
	if err != nil {
		t.Fatalf("reading through symlink: %v", err)
	}
	if string(contents) != "hi" {
		t.Errorf("contents through symlink = %q, want hi", contents)
	}
}

func TestSymlinkEntries_ReplacesExistingSymlink(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	sharedDir := filepath.Join(mainRepository, "data")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("creating shared dir: %v", err)
	}

	staleTarget := t.TempDir()
	link := filepath.Join(instancePath, "data")
	if err := os.Symlink(staleTarget, link); err != nil {
		t.Fatalf("creating stale symlink: %v", err)
	}

	if err := symlinkEntries(mainRepository, instancePath, []string{"data"}); err != nil {
		t.Fatalf("symlinkEntries() returned unexpected error: %v", err)
	}

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != sharedDir {
		t.Errorf("symlink target = %q, want it replaced with %q", target, sharedDir)
	}
}

func TestSymlinkEntries_RefusesNonEmptyRealDirectory(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.MkdirAll(filepath.Join(mainRepository, "data"), 0o755); err != nil {
		t.Fatalf("creating main repo dir: %v", err)
	}

	realDir := filepath.Join(instancePath, "data")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("creating real dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "important.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := symlinkEntries(mainRepository, instancePath, []string{"data"})
	if err == nil {
		t.Fatalf("symlinkEntries() succeeded against a non-empty real directory, want refusal")
	}

	if _, statErr := os.Stat(filepath.Join(realDir, "important.txt")); statErr != nil {
		t.Errorf("important.txt was removed despite refusal: %v", statErr)
	}
}

// TestSymlinkEntries_FailsOnMissingSource guards BUG 2's fix in the other
// direction from copy/copy_files: a provision.symlink source that doesn't
// exist in the main repository is a real misconfiguration (there is no
// data anywhere for the instance to share), so it must fail outright
// rather than silently creating a dangling symlink.
func TestSymlinkEntries_FailsOnMissingSource(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	err := symlinkEntries(mainRepository, instancePath, []string{"data/shared"})
	if err == nil {
		t.Fatalf("symlinkEntries() succeeded for a missing source, want a failure")
	}

	if _, statErr := os.Lstat(filepath.Join(instancePath, "data", "shared")); !os.IsNotExist(statErr) {
		t.Errorf("symlinkEntries() created a dangling symlink for a missing source")
	}
}

func TestSymlinkEntries_ReplacesEmptyRealDirectory(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.MkdirAll(filepath.Join(mainRepository, "data"), 0o755); err != nil {
		t.Fatalf("creating main repo dir: %v", err)
	}

	emptyDir := filepath.Join(instancePath, "data")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("creating empty dir: %v", err)
	}

	if err := symlinkEntries(mainRepository, instancePath, []string{"data"}); err != nil {
		t.Fatalf("symlinkEntries() returned unexpected error for an empty real directory: %v", err)
	}

	info, err := os.Lstat(emptyDir)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("empty real directory was not replaced with a symlink")
	}
}

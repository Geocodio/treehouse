package provision

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/manifest"
)

// requiredSymlinks wraps plain relative paths as required (non-optional)
// provision.symlink entries, the string form's meaning.
func requiredSymlinks(paths ...string) []manifest.SymlinkEntry {
	entries := make([]manifest.SymlinkEntry, len(paths))
	for i, path := range paths {
		entries[i] = manifest.SymlinkEntry{Path: path}
	}
	return entries
}

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

	if err := symlinkEntries(mainRepository, instancePath, requiredSymlinks("data/shared"), io.Discard); err != nil {
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

	if err := symlinkEntries(mainRepository, instancePath, requiredSymlinks("data"), io.Discard); err != nil {
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

	err := symlinkEntries(mainRepository, instancePath, requiredSymlinks("data"), io.Discard)
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

	err := symlinkEntries(mainRepository, instancePath, requiredSymlinks("data/shared"), io.Discard)
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

	if err := symlinkEntries(mainRepository, instancePath, requiredSymlinks("data"), io.Discard); err != nil {
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

// TestSymlinkEntries_SkipsMissingOptionalSourceWithWarning covers the opt-in
// relaxation of the hard-fail rule: an entry flagged `optional: true` whose
// source is absent from the main repository is skipped with a warning
// instead of aborting provisioning, and no dangling symlink is left behind.
func TestSymlinkEntries_SkipsMissingOptionalSourceWithWarning(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	var warnings bytes.Buffer
	entries := []manifest.SymlinkEntry{{Path: "dev-certs", Optional: true}}
	if err := symlinkEntries(mainRepository, instancePath, entries, &warnings); err != nil {
		t.Fatalf("symlinkEntries() returned an error for a missing optional source: %v", err)
	}

	if _, statErr := os.Lstat(filepath.Join(instancePath, "dev-certs")); !os.IsNotExist(statErr) {
		t.Errorf("symlinkEntries() created a link for a skipped optional source")
	}
	if !strings.Contains(warnings.String(), "skipping dev-certs") {
		t.Errorf("warnings = %q, want it to mention skipping dev-certs", warnings.String())
	}
}

// TestSymlinkEntries_LinksPresentOptionalSource confirms `optional` only
// changes the missing-source behavior: when the source does exist, an
// optional entry is symlinked exactly like a required one.
func TestSymlinkEntries_LinksPresentOptionalSource(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	sharedDir := filepath.Join(mainRepository, "dev-certs")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("creating shared dir: %v", err)
	}

	var warnings bytes.Buffer
	entries := []manifest.SymlinkEntry{{Path: "dev-certs", Optional: true}}
	if err := symlinkEntries(mainRepository, instancePath, entries, &warnings); err != nil {
		t.Fatalf("symlinkEntries() returned unexpected error: %v", err)
	}

	link := filepath.Join(instancePath, "dev-certs")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(link): %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", link)
	}
	if warnings.Len() != 0 {
		t.Errorf("warnings = %q, want none when the optional source is present", warnings.String())
	}
}

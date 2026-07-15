package provision

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/manifest"
)

func TestCopyEntry_ClonesDirectoryContents(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	vendorDir := filepath.Join(mainRepository, "vendor")
	if err := os.MkdirAll(filepath.Join(vendorDir, "pkg"), 0o755); err != nil {
		t.Fatalf("creating vendor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "pkg", "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	var warnings bytes.Buffer
	skipped, err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}, &warnings)
	if err != nil {
		t.Fatalf("copyEntry() returned unexpected error: %v", err)
	}
	if skipped {
		t.Errorf("copyEntry() skipped = true for a source that exists, want false")
	}
	if warnings.Len() != 0 {
		t.Errorf("copyEntry() wrote a warning for a source that exists: %q", warnings.String())
	}

	copied := filepath.Join(instancePath, "vendor", "pkg", "file.txt")
	contents, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("reading copied file: %v", err)
	}
	if string(contents) != "hi" {
		t.Errorf("copied contents = %q, want hi", contents)
	}

	// It must be a real, independent copy, not a symlink back to the
	// source: mutating the instance's copy must not affect the main repo.
	info, err := os.Lstat(filepath.Join(instancePath, "vendor"))
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("copyEntry() produced a symlink, want a real copy")
	}
}

func TestCopyEntry_IsIdempotent(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	vendorDir := filepath.Join(mainRepository, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("creating vendor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "original.txt"), []byte("original"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	if _, err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("first copyEntry() returned unexpected error: %v", err)
	}

	// Simulate a local edit inside the instance's copy that a second,
	// blind re-copy must not clobber.
	localFile := filepath.Join(instancePath, "vendor", "local-edit.txt")
	if err := os.WriteFile(localFile, []byte("local"), 0o644); err != nil {
		t.Fatalf("writing local edit: %v", err)
	}

	if _, err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("second copyEntry() returned unexpected error: %v", err)
	}

	if _, err := os.Stat(localFile); err != nil {
		t.Errorf("local edit was clobbered by a repeat copyEntry(): %v", err)
	}
}

// TestCopyEntry_SkipsMissingSourceWithWarning guards BUG 2's fix: a
// provision.copy source that simply doesn't exist in the main repository
// (a generated build-artifact directory nobody has built there yet) must
// warn and skip, never fail the whole create/provision run.
func TestCopyEntry_SkipsMissingSourceWithWarning(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	var warnings bytes.Buffer
	skipped, err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}, &warnings)
	if err != nil {
		t.Fatalf("copyEntry() returned unexpected error for a missing source, want a warning instead: %v", err)
	}
	if !skipped {
		t.Errorf("copyEntry() skipped = false for a missing source, want true")
	}
	if !strings.Contains(warnings.String(), "vendor") || !strings.Contains(warnings.String(), "not present in main repository") {
		t.Errorf("warnings = %q, want it to mention vendor and the main repository", warnings.String())
	}

	if _, statErr := os.Stat(filepath.Join(instancePath, "vendor")); !os.IsNotExist(statErr) {
		t.Errorf("copyEntry() created a destination for a missing source")
	}
}

func TestCopyFiles_CopiesPlainFiles(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.WriteFile(filepath.Join(mainRepository, ".env.docker"), []byte("KEY=value\n"), 0o644); err != nil {
		t.Fatalf("writing .env.docker: %v", err)
	}

	if err := copyFiles(mainRepository, instancePath, []string{".env.docker"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("copyFiles() returned unexpected error: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(instancePath, ".env.docker"))
	if err != nil {
		t.Fatalf("reading copied file: %v", err)
	}
	if string(contents) != "KEY=value\n" {
		t.Errorf("copied contents = %q, want KEY=value", contents)
	}
}

func TestCopyFiles_DoesNotOverwriteExistingDestination(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.WriteFile(filepath.Join(mainRepository, ".env.docker"), []byte("upstream\n"), 0o644); err != nil {
		t.Fatalf("writing .env.docker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(instancePath, ".env.docker"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatalf("writing local .env.docker: %v", err)
	}

	if err := copyFiles(mainRepository, instancePath, []string{".env.docker"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("copyFiles() returned unexpected error: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(instancePath, ".env.docker"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(contents) != "local edit\n" {
		t.Errorf("contents = %q, want local edit preserved", contents)
	}
}

// TestCopyFiles_SkipsMissingSourceWithWarning mirrors
// TestCopyEntry_SkipsMissingSourceWithWarning for copy_files: a missing
// source file warns and is skipped rather than failing the run.
func TestCopyFiles_SkipsMissingSourceWithWarning(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	var warnings bytes.Buffer
	if err := copyFiles(mainRepository, instancePath, []string{".env.docker"}, &warnings); err != nil {
		t.Fatalf("copyFiles() returned unexpected error for a missing source, want a warning instead: %v", err)
	}

	if !strings.Contains(warnings.String(), ".env.docker") || !strings.Contains(warnings.String(), "not present in main repository") {
		t.Errorf("warnings = %q, want it to mention .env.docker and the main repository", warnings.String())
	}

	if _, statErr := os.Stat(filepath.Join(instancePath, ".env.docker")); !os.IsNotExist(statErr) {
		t.Errorf("copyFiles() created a destination for a missing source")
	}
}

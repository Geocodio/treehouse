package provision

import (
	"os"
	"path/filepath"
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

	if err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}); err != nil {
		t.Fatalf("copyEntry() returned unexpected error: %v", err)
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

	if err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}); err != nil {
		t.Fatalf("first copyEntry() returned unexpected error: %v", err)
	}

	// Simulate a local edit inside the instance's copy that a second,
	// blind re-copy must not clobber.
	localFile := filepath.Join(instancePath, "vendor", "local-edit.txt")
	if err := os.WriteFile(localFile, []byte("local"), 0o644); err != nil {
		t.Fatalf("writing local edit: %v", err)
	}

	if err := copyEntry(mainRepository, instancePath, manifest.CopyEntry{Path: "vendor"}); err != nil {
		t.Fatalf("second copyEntry() returned unexpected error: %v", err)
	}

	if _, err := os.Stat(localFile); err != nil {
		t.Errorf("local edit was clobbered by a repeat copyEntry(): %v", err)
	}
}

func TestCopyFiles_CopiesPlainFiles(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.WriteFile(filepath.Join(mainRepository, ".env.docker"), []byte("KEY=value\n"), 0o644); err != nil {
		t.Fatalf("writing .env.docker: %v", err)
	}

	if err := copyFiles(mainRepository, instancePath, []string{".env.docker"}); err != nil {
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

	if err := copyFiles(mainRepository, instancePath, []string{".env.docker"}); err != nil {
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

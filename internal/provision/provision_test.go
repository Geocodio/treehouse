package provision

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
)

func newTestInstance(t *testing.T, mainRepository, instancePath string) *instance.Instance {
	t.Helper()

	inst := instance.New(instance.NewParams{
		Project:        "myapp",
		Name:           "feature-a",
		Branch:         "feature-a",
		MainRepository: mainRepository,
		Path:           instancePath,
		BlockStart:     4000,
		Ports:          map[string]int{"HTTP_PORT": 4000},
	})

	return &inst
}

func TestHashLockfile_DetectsDrift(t *testing.T) {
	mainRepository := t.TempDir()
	lockfilePath := filepath.Join(mainRepository, "composer.lock")

	if err := os.WriteFile(lockfilePath, []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing lockfile: %v", err)
	}

	firstHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile() returned unexpected error: %v", err)
	}

	secondHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile() returned unexpected error: %v", err)
	}
	if firstHash != secondHash {
		t.Errorf("hashLockfile() is not stable across calls: %q != %q", firstHash, secondHash)
	}

	if err := os.WriteFile(lockfilePath, []byte("v2"), 0o644); err != nil {
		t.Fatalf("rewriting lockfile: %v", err)
	}

	thirdHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile() returned unexpected error: %v", err)
	}
	if thirdHash == firstHash {
		t.Errorf("hashLockfile() did not change after the lockfile's contents changed")
	}
}

func TestMaterializeDependencies_RecordsLockHashForReconcileEntries(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	if err := os.WriteFile(filepath.Join(mainRepository, "composer.lock"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing lockfile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mainRepository, "vendor"), 0o755); err != nil {
		t.Fatalf("creating vendor dir: %v", err)
	}

	inst := newTestInstance(t, mainRepository, instancePath)
	manifestConfig := &manifest.Manifest{
		Provision: manifest.ProvisionConfig{
			Copy: []manifest.CopyEntry{
				{Path: "vendor", Reconcile: &manifest.ReconcileConfig{Lockfile: "composer.lock", Command: "true"}},
			},
		},
	}

	if err := MaterializeDependencies(inst, manifestConfig); err != nil {
		t.Fatalf("MaterializeDependencies() returned unexpected error: %v", err)
	}

	wantHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile(): %v", err)
	}
	if inst.LockHashes["composer.lock"] != wantHash {
		t.Errorf("LockHashes[composer.lock] = %q, want %q", inst.LockHashes["composer.lock"], wantHash)
	}
}

func TestReconcile_RunsCommandOnlyWhenLockfileDrifted(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()
	if err := os.MkdirAll(instancePath, 0o755); err != nil {
		t.Fatalf("creating instance dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(mainRepository, "composer.lock"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing lockfile: %v", err)
	}

	inst := newTestInstance(t, mainRepository, instancePath)
	originalHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile(): %v", err)
	}
	inst.LockHashes["composer.lock"] = originalHash

	marker := filepath.Join(instancePath, "reconcile-ran")
	manifestConfig := &manifest.Manifest{
		Provision: manifest.ProvisionConfig{
			Copy: []manifest.CopyEntry{
				{Path: "vendor", Reconcile: &manifest.ReconcileConfig{
					Lockfile: "composer.lock",
					Command:  "touch " + marker,
				}},
			},
		},
	}

	// No drift yet: Reconcile must not run the command.
	if err := Reconcile(inst, manifestConfig); err != nil {
		t.Fatalf("Reconcile() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Reconcile() ran reconcile.command with no lockfile drift")
	}

	// Now the lockfile drifts: Reconcile must run the command and update
	// the recorded hash.
	if err := os.WriteFile(filepath.Join(mainRepository, "composer.lock"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("rewriting lockfile: %v", err)
	}

	if err := Reconcile(inst, manifestConfig); err != nil {
		t.Fatalf("Reconcile() returned unexpected error: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("Reconcile() did not run reconcile.command after lockfile drift: %v", err)
	}

	newHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile(): %v", err)
	}
	if inst.LockHashes["composer.lock"] != newHash {
		t.Errorf("LockHashes[composer.lock] = %q, want updated hash %q", inst.LockHashes["composer.lock"], newHash)
	}
}

func TestRunHook_StreamsSuccessAndFailure(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()
	if err := os.MkdirAll(instancePath, 0o755); err != nil {
		t.Fatalf("creating instance dir: %v", err)
	}

	hookScript := "#!/bin/sh\nset -eu\n" +
		"test -n \"$TREEHOUSE_INSTANCE_PATH\"\n" +
		"test -n \"$TREEHOUSE_MAIN_REPOSITORY\"\n" +
		"test \"$HTTP_PORT\" = \"4000\"\n"
	hookPath := filepath.Join(instancePath, "hook.sh")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatalf("writing hook script: %v", err)
	}

	inst := newTestInstance(t, mainRepository, instancePath)
	manifestConfig := &manifest.Manifest{Provision: manifest.ProvisionConfig{Hook: "hook.sh"}}

	if err := RunHook(inst, manifestConfig); err != nil {
		t.Fatalf("RunHook() returned unexpected error for a passing hook: %v", err)
	}

	failingHook := "#!/bin/sh\nexit 4\n"
	if err := os.WriteFile(hookPath, []byte(failingHook), 0o755); err != nil {
		t.Fatalf("writing failing hook script: %v", err)
	}

	if err := RunHook(inst, manifestConfig); err == nil {
		t.Errorf("RunHook() returned nil for a failing hook, want an error")
	}
}

func TestRunHook_NoHookConfiguredIsNotAnError(t *testing.T) {
	inst := newTestInstance(t, t.TempDir(), t.TempDir())
	manifestConfig := &manifest.Manifest{}

	if err := RunHook(inst, manifestConfig); err != nil {
		t.Errorf("RunHook() returned unexpected error with no hook configured: %v", err)
	}
}

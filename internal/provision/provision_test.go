package provision

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/manifest"
)

// captureStdoutStderr temporarily swaps the real, process-level os.Stdout
// and os.Stderr for pipes for the duration of fn, returning everything
// written to each. RunHook and runInDir (reconcile.command) wire their
// child processes directly to the os.Stdout/os.Stderr package variables
// rather than to an injectable writer, so this is the only way a test can
// observe which actual stream a hook or reconcile command's output lands
// on.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() for stdout: %v", err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() for stderr: %v", err)
	}

	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWrite, stderrWrite

	fn()

	os.Stdout, os.Stderr = originalStdout, originalStderr
	if err := stdoutWrite.Close(); err != nil {
		t.Fatalf("closing stdout pipe writer: %v", err)
	}
	if err := stderrWrite.Close(); err != nil {
		t.Fatalf("closing stderr pipe writer: %v", err)
	}

	stdoutBytes, err := io.ReadAll(stdoutRead)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrRead)
	if err != nil {
		t.Fatalf("reading captured stderr: %v", err)
	}

	return string(stdoutBytes), string(stderrBytes)
}

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

	if err := MaterializeDependencies(inst, manifestConfig, &bytes.Buffer{}); err != nil {
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

// TestMaterializeDependencies_RunsReconcileUnconditionallyWhenCopySkipped
// guards BUG 2's second half: when a provision.copy entry's source doesn't
// exist in the main repository (so copyEntry skips it with a warning, per
// copy_test.go), an entry with a reconcile step must still end up with a
// working directory — the installer runs unconditionally right away,
// since there is no previously cloned directory for a lockfile-hash
// comparison to mean anything.
func TestMaterializeDependencies_RunsReconcileUnconditionallyWhenCopySkipped(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()

	// composer.lock exists in the main repository, but the vendor/
	// directory itself does not -- e.g. a fresh clone of a branch that has
	// never run `composer install` locally.
	if err := os.WriteFile(filepath.Join(mainRepository, "composer.lock"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing lockfile: %v", err)
	}

	inst := newTestInstance(t, mainRepository, instancePath)
	marker := filepath.Join(instancePath, "vendor", "installed")
	manifestConfig := &manifest.Manifest{
		Provision: manifest.ProvisionConfig{
			Copy: []manifest.CopyEntry{
				{Path: "vendor", Reconcile: &manifest.ReconcileConfig{
					Lockfile: "composer.lock",
					Command:  "mkdir -p vendor && touch vendor/installed",
				}},
			},
		},
	}

	var warnings bytes.Buffer
	if err := MaterializeDependencies(inst, manifestConfig, &warnings); err != nil {
		t.Fatalf("MaterializeDependencies() returned unexpected error: %v", err)
	}

	if !strings.Contains(warnings.String(), "vendor") {
		t.Errorf("warnings = %q, want it to mention the skipped vendor copy", warnings.String())
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("reconcile.command did not run unconditionally for a skipped copy: %v", err)
	}

	wantHash, err := hashLockfile(mainRepository, "composer.lock")
	if err != nil {
		t.Fatalf("hashLockfile(): %v", err)
	}
	if inst.LockHashes["composer.lock"] != wantHash {
		t.Errorf("LockHashes[composer.lock] = %q, want %q (so a later Reconcile() only reruns on real drift)", inst.LockHashes["composer.lock"], wantHash)
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

// TestRunHook_StdoutNeverLandsOnStdout guards DESIGN.md's "Agent mode"
// contract: stdout is reserved for `--json`'s single data document, so a
// chatty hook (e.g. one running database migrations) must never be able to
// interleave its own output into it. Both the hook's stdout and its stderr
// must stream only to the CLI's stderr.
func TestRunHook_StdoutNeverLandsOnStdout(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()
	if err := os.MkdirAll(instancePath, 0o755); err != nil {
		t.Fatalf("creating instance dir: %v", err)
	}

	hookScript := "#!/bin/sh\necho hook-wrote-to-stdout\necho hook-wrote-to-stderr >&2\n"
	hookPath := filepath.Join(instancePath, "hook.sh")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
		t.Fatalf("writing hook script: %v", err)
	}

	inst := newTestInstance(t, mainRepository, instancePath)
	manifestConfig := &manifest.Manifest{Provision: manifest.ProvisionConfig{Hook: "hook.sh"}}

	var runErr error
	stdout, stderr := captureStdoutStderr(t, func() {
		runErr = RunHook(inst, manifestConfig)
	})

	if runErr != nil {
		t.Fatalf("RunHook() returned unexpected error: %v", runErr)
	}
	if strings.Contains(stdout, "hook-wrote-to-stdout") {
		t.Errorf("hook's own stdout leaked onto the CLI's stdout: stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "hook-wrote-to-stdout") {
		t.Errorf("hook's stdout was not redirected to the CLI's stderr: stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "hook-wrote-to-stderr") {
		t.Errorf("hook's own stderr did not stream to the CLI's stderr: stderr = %q", stderr)
	}
}

// TestReconcile_CommandOutputNeverLandsOnStdout guards the same "Agent
// mode" contract as TestRunHook_StdoutNeverLandsOnStdout, but for
// reconcile.command (runInDir), which shares the same stdout/stderr wiring
// as provision.hook — a chatty installer (e.g. "composer install") must not
// be able to corrupt `create --json`'s single data document either.
func TestReconcile_CommandOutputNeverLandsOnStdout(t *testing.T) {
	mainRepository := t.TempDir()
	instancePath := t.TempDir()
	if err := os.MkdirAll(instancePath, 0o755); err != nil {
		t.Fatalf("creating instance dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(mainRepository, "composer.lock"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("writing lockfile: %v", err)
	}

	// inst.LockHashes has no recorded hash for composer.lock yet, so it
	// mismatches the freshly computed hash and Reconcile runs the command
	// unconditionally on this first call.
	inst := newTestInstance(t, mainRepository, instancePath)
	manifestConfig := &manifest.Manifest{
		Provision: manifest.ProvisionConfig{
			Copy: []manifest.CopyEntry{
				{Path: "vendor", Reconcile: &manifest.ReconcileConfig{
					Lockfile: "composer.lock",
					Command:  "echo reconcile-wrote-to-stdout; echo reconcile-wrote-to-stderr >&2",
				}},
			},
		},
	}

	var runErr error
	stdout, stderr := captureStdoutStderr(t, func() {
		runErr = Reconcile(inst, manifestConfig)
	})

	if runErr != nil {
		t.Fatalf("Reconcile() returned unexpected error: %v", runErr)
	}
	if strings.Contains(stdout, "reconcile-wrote-to-stdout") {
		t.Errorf("reconcile.command's own stdout leaked onto the CLI's stdout: stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "reconcile-wrote-to-stdout") {
		t.Errorf("reconcile.command's stdout was not redirected to the CLI's stderr: stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "reconcile-wrote-to-stderr") {
		t.Errorf("reconcile.command's own stderr did not stream to the CLI's stderr: stderr = %q", stderr)
	}
}

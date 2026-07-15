package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

func TestInstanceLabel_ContainsIdentifyingFields(t *testing.T) {
	inst := instance.Instance{
		Project: "myapp", Name: "feature-a", Branch: "feature/a",
		State: instance.StateProvisioned, Path: "/treehouses/myapp/feature-a",
	}

	label := instanceLabel(inst)

	for _, want := range []string{"myapp/feature-a", "feature/a", "provisioned", "/treehouses/myapp/feature-a"} {
		if !strings.Contains(label, want) {
			t.Errorf("instanceLabel() = %q, want it to contain %q", label, want)
		}
	}
}

func TestSelectViaMenu_ValidSelection(t *testing.T) {
	instances := []instance.Instance{
		{Project: "myapp", Name: "feature-a", Branch: "feature/a"},
		{Project: "myapp", Name: "feature-b", Branch: "feature/b"},
	}

	root := NewRootCommand("test")
	var output bytes.Buffer
	root.SetErr(&output)
	root.SetIn(strings.NewReader("2\n"))

	selected, err := selectViaMenu(root, instances)
	if err != nil {
		t.Fatalf("selectViaMenu() returned unexpected error: %v", err)
	}
	if selected.Name != "feature-b" {
		t.Errorf("selected.Name = %q, want feature-b", selected.Name)
	}

	// The menu and prompt must be on stderr, never stdout.
	if !strings.Contains(output.String(), "feature-a") || !strings.Contains(output.String(), "feature-b") {
		t.Errorf("stderr output = %q, want it to list both instances", output.String())
	}
}

func TestSelectViaMenu_OutOfRangeSelectionIsAnError(t *testing.T) {
	instances := []instance.Instance{
		{Project: "myapp", Name: "feature-a", Branch: "feature/a"},
	}

	root := NewRootCommand("test")
	var output bytes.Buffer
	root.SetErr(&output)
	root.SetIn(strings.NewReader("9\n"))

	if _, err := selectViaMenu(root, instances); err == nil {
		t.Errorf("selectViaMenu() returned nil error for an out-of-range selection")
	}
}

func TestSelectViaMenu_NonNumericSelectionIsAnError(t *testing.T) {
	instances := []instance.Instance{
		{Project: "myapp", Name: "feature-a", Branch: "feature/a"},
	}

	root := NewRootCommand("test")
	var output bytes.Buffer
	root.SetErr(&output)
	root.SetIn(strings.NewReader("banana\n"))

	if _, err := selectViaMenu(root, instances); err == nil {
		t.Errorf("selectViaMenu() returned nil error for a non-numeric selection")
	}
}

// TestRunSwitch_RefusesWithoutTTY guards switch's "interactive only" rule:
// `go test` itself has no TTY on stdin, so this exercises the real,
// unmodified IsTTY() gate exactly as a non-interactive agent invocation
// would hit it.
func TestRunSwitch_RefusesWithoutTTY(t *testing.T) {
	var output bytes.Buffer
	root := NewRootCommand("test")
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"switch"})

	err := root.Execute()
	if err == nil {
		t.Fatalf("switch succeeded in a non-interactive test process, want ExitWouldPrompt")
	}
	if ExitCodeFor(err) != ExitWouldPrompt {
		t.Errorf("ExitCodeFor(err) = %v, want ExitWouldPrompt", ExitCodeFor(err))
	}
}

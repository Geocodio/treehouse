package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/geocodio/treehouse/internal/instance"
)

func newSwitchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "switch",
		Short: "Interactively pick an instance and print its path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwitch(cmd)
		},
	}
}

// runSwitch is interactive-only, per DESIGN.md: it refuses to run at all
// without a TTY (ExitWouldPrompt), since there is no non-interactive
// default worth guessing for "pick an instance". Every prompt and menu goes
// to stderr; only the selected instance's path is written to stdout, so `cd
// "$(treehouse switch)"` works exactly like `cd "$(treehouse path <name>)"`.
func runSwitch(cmd *cobra.Command) error {
	if !IsTTY() {
		return NewExitError(ExitWouldPrompt, fmt.Errorf("switch requires an interactive terminal"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	repo, err := loadRepositoryContext(cwd)
	if err != nil {
		return err
	}

	instances, err := instance.Discover(repo.MainRepository)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}
	if len(instances) == 0 {
		return NewExitError(ExitGenericFailure, fmt.Errorf("no instances found"))
	}

	selected, err := selectInstance(cmd, instances)
	if err != nil {
		return NewExitError(ExitGenericFailure, err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), selected.Path)
	return nil
}

// selectInstance uses fzf when it's installed (the richer, fuzzy-searchable
// experience), falling back to a plain numbered menu otherwise.
func selectInstance(cmd *cobra.Command, instances []instance.Instance) (*instance.Instance, error) {
	if _, err := exec.LookPath("fzf"); err == nil {
		return selectViaFzf(instances)
	}

	return selectViaMenu(cmd, instances)
}

// instanceLabel is one instance's single-line, tab-delimited representation
// for both fzf and the numbered menu: project/instance, branch, state, and
// path, in that order.
func instanceLabel(inst instance.Instance) string {
	return fmt.Sprintf("%s/%s\tbranch=%s\tstate=%s\t%s", inst.Project, inst.Name, inst.Branch, inst.State, inst.Path)
}

// selectViaFzf pipes every instance's label to fzf as candidates on stdin;
// fzf itself opens /dev/tty directly for its interactive UI (the standard
// `candidates | fzf` pattern), so none of that UI passes through our own
// stdout/stderr.
func selectViaFzf(instances []instance.Instance) (*instance.Instance, error) {
	var input strings.Builder
	for _, inst := range instances {
		input.WriteString(instanceLabel(inst))
		input.WriteString("\n")
	}

	command := exec.Command("fzf", "--delimiter=\t", "--with-nth=1,2,3")
	command.Stdin = strings.NewReader(input.String())
	command.Stderr = os.Stderr

	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("fzf: %w", err)
	}

	selectedLabel := strings.TrimSpace(string(output))
	for index := range instances {
		if instanceLabel(instances[index]) == selectedLabel {
			return &instances[index], nil
		}
	}

	return nil, fmt.Errorf("fzf returned an unrecognized selection")
}

// selectViaMenu is the numbered-menu fallback when fzf isn't installed. The
// menu itself and its prompt go to cmd's stderr; the answer is read via
// cmd.InOrStdin() (os.Stdin in a real invocation, a fake reader in tests).
func selectViaMenu(cmd *cobra.Command, instances []instance.Instance) (*instance.Instance, error) {
	errWriter := cmd.ErrOrStderr()

	for index, inst := range instances {
		fmt.Fprintf(errWriter, "%d) %s\n", index+1, instanceLabel(inst))
	}
	fmt.Fprint(errWriter, "select an instance: ")

	choice, err := readMenuChoice(cmd.InOrStdin())
	if err != nil {
		return nil, err
	}

	selectedIndex, err := strconv.Atoi(choice)
	if err != nil || selectedIndex < 1 || selectedIndex > len(instances) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}

	return &instances[selectedIndex-1], nil
}

func readMenuChoice(in io.Reader) (string, error) {
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}

	return strings.TrimSpace(line), nil
}

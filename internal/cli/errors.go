package cli

import "errors"

// ExitError pairs an error with the specific ExitCode a command should
// terminate with. Command implementations return one of these instead of a
// bare error whenever the failure carries contractual exit-code meaning (see
// DESIGN.md's "Agent mode" exit code table). Errors without an ExitError
// default to ExitGenericFailure.
type ExitError struct {
	Code ExitCode
	Err  error
}

func (exitError *ExitError) Error() string {
	return exitError.Err.Error()
}

func (exitError *ExitError) Unwrap() error {
	return exitError.Err
}

// NewExitError wraps err so it terminates the process with the given code.
func NewExitError(code ExitCode, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

// ExitCodeFor determines the process exit code for err, defaulting to
// ExitGenericFailure when err carries no explicit ExitError.
func ExitCodeFor(err error) ExitCode {
	if err == nil {
		return ExitSuccess
	}

	var exitError *ExitError
	if errors.As(err, &exitError) {
		return exitError.Code
	}

	return ExitGenericFailure
}

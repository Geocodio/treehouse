package cli

import (
	"reflect"
	"testing"
)

func TestParseExecArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantService string
		wantCommand []string
		wantErr     bool
	}{
		{
			name:        "without separator",
			args:        []string{"web", "sh", "-c", "echo hi"},
			wantService: "web",
			wantCommand: []string{"sh", "-c", "echo hi"},
		},
		{
			name:        "with explicit separator",
			args:        []string{"web", "--", "sh", "-c", "echo hi"},
			wantService: "web",
			wantCommand: []string{"sh", "-c", "echo hi"},
		},
		{
			name:    "no service",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "service with no command",
			args:    []string{"web"},
			wantErr: true,
		},
		{
			name:    "service with only a separator",
			args:    []string{"web", "--"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, command, err := parseExecArgs(test.args)

			if test.wantErr {
				if err == nil {
					t.Fatalf("parseExecArgs(%v) = nil error, want an error", test.args)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseExecArgs(%v) returned unexpected error: %v", test.args, err)
			}
			if service != test.wantService {
				t.Errorf("service = %q, want %q", service, test.wantService)
			}
			if !reflect.DeepEqual(command, test.wantCommand) {
				t.Errorf("command = %v, want %v", command, test.wantCommand)
			}
		})
	}
}

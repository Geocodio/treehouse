package cli

import "testing"

func TestDownKeepsVolumesByDefault(t *testing.T) {
	flag := newDownCommand().Flags().Lookup("volumes")
	if flag == nil {
		t.Fatal("down has no --volumes flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("--volumes defaults to %s, want false so down never deletes data unasked", flag.DefValue)
	}
}

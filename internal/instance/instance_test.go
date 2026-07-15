package instance

import (
	"reflect"
	"testing"
)

func TestPortsForBlock(t *testing.T) {
	got := PortsForBlock(4640, map[string]int{"HTTP_PORT": 0, "DATABASE_PORT": 1}, nil)
	want := map[string]int{"HTTP_PORT": 4640, "DATABASE_PORT": 4641}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("PortsForBlock() = %v, want %v", got, want)
	}
}

func TestPortsForBlock_AppliesAliases(t *testing.T) {
	got := PortsForBlock(
		4640,
		map[string]int{"HTTP_PORT": 0, "DATABASE_PORT": 1},
		map[string]string{"WEB_PORT": "HTTP_PORT"},
	)
	want := map[string]int{"HTTP_PORT": 4640, "DATABASE_PORT": 4641, "WEB_PORT": 4640}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("PortsForBlock() = %v, want %v", got, want)
	}
}

func TestPortsForBlock_SkipsAliasWithUnknownCanonical(t *testing.T) {
	got := PortsForBlock(4640, map[string]int{"HTTP_PORT": 0}, map[string]string{"WEB_PORT": "MISSING_PORT"})
	want := map[string]int{"HTTP_PORT": 4640}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("PortsForBlock() = %v, want %v", got, want)
	}
}

func TestAdvanceState_NeverRegressesOnHappyPath(t *testing.T) {
	instance := &Instance{State: StateProvisioned}

	instance.AdvanceState(StateStackUp)
	if instance.State != StateProvisioned {
		t.Errorf("State = %q after regressive AdvanceState, want unchanged StateProvisioned", instance.State)
	}
}

func TestAdvanceState_AdvancesForward(t *testing.T) {
	instance := &Instance{State: StateWorktreeCreated}

	instance.AdvanceState(StateDepsCloned)
	if instance.State != StateDepsCloned {
		t.Errorf("State = %q, want %q", instance.State, StateDepsCloned)
	}

	instance.AdvanceState(StateStackUp)
	if instance.State != StateStackUp {
		t.Errorf("State = %q, want %q", instance.State, StateStackUp)
	}
}

func TestAdvanceState_ProvisionFailedAlwaysApplies(t *testing.T) {
	instance := &Instance{State: StateProvisioned}

	instance.AdvanceState(StateProvisionFailed)
	if instance.State != StateProvisionFailed {
		t.Errorf("State = %q, want %q", instance.State, StateProvisionFailed)
	}
}

package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
)

var errBoom = errors.New("lsof: command not found")

type fakePortChecker struct {
	ports map[int]bool
	err   error
}

func (checker fakePortChecker) ListeningPorts() (map[int]bool, error) {
	if checker.err != nil {
		return nil, checker.err
	}
	return checker.ports, nil
}

func testInstanceForPortVerify() instance.Instance {
	return instance.New(instance.NewParams{
		Project: "myapp", Name: "feature-a", Branch: "feature-a",
		MainRepository: "/repo", Path: "/treehouses/myapp/feature-a",
		BlockStart: 4640, Ports: map[string]int{"HTTP_PORT": 4640, "DATABASE_PORT": 4641},
	})
}

// noProcesses stands in for registry.ListeningPortProcesses in tests that
// don't care about process-name enrichment, so they never shell out to a
// real lsof.
func noProcesses() (map[int]string, error) {
	return map[int]string{}, nil
}

func TestVerifyPortBlockFree_NoConflict(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{ports: map[int]bool{9999: true}}

	if err := verifyPortBlockFree(checker, nil, noProcesses, &inst, 32); err != nil {
		t.Errorf("verifyPortBlockFree() = %v, want nil", err)
	}
}

func TestVerifyPortBlockFree_NamesConflictingPort(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{ports: map[int]bool{4640: true}}

	err := verifyPortBlockFree(checker, nil, noProcesses, &inst, 32)
	if err == nil {
		t.Fatalf("verifyPortBlockFree() = nil, want a conflict error")
	}
	if !strings.Contains(err.Error(), "4640") || !strings.Contains(err.Error(), "HTTP_PORT") {
		t.Errorf("error %q does not name the conflicting port and its env var", err)
	}
}

func TestVerifyPortBlockFree_ConflictOutsideNamedPortsStillDetected(t *testing.T) {
	inst := testInstanceForPortVerify()
	// Port 4650 is inside the claimed block but not a manifest-named port.
	checker := fakePortChecker{ports: map[int]bool{4650: true}}

	err := verifyPortBlockFree(checker, nil, noProcesses, &inst, 32)
	if err == nil {
		t.Fatalf("verifyPortBlockFree() = nil, want a conflict error for a port inside the block")
	}
	if !strings.Contains(err.Error(), "4650") {
		t.Errorf("error %q does not name port 4650", err)
	}
}

func TestVerifyPortBlockFree_DegradesGracefullyWhenCheckerFails(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{err: errBoom}

	if err := verifyPortBlockFree(checker, nil, noProcesses, &inst, 32); err != nil {
		t.Errorf("verifyPortBlockFree() = %v, want nil when the checker itself fails", err)
	}
}

func TestVerifyPortBlockFree_ExcludesOwnProjectsPorts(t *testing.T) {
	inst := testInstanceForPortVerify()
	// 4640 is listening, but it belongs to this project's own (already
	// partially up) containers, so it must not be reported as a conflict.
	checker := fakePortChecker{ports: map[int]bool{4640: true}}
	ownPorts := map[int]bool{4640: true}

	if err := verifyPortBlockFree(checker, ownPorts, noProcesses, &inst, 32); err != nil {
		t.Errorf("verifyPortBlockFree() = %v, want nil for a port owned by this project", err)
	}
}

func TestVerifyPortBlockFree_OwnPortsExclusionIsPerPort(t *testing.T) {
	inst := testInstanceForPortVerify()
	// 4640 belongs to this project; 4641 (DATABASE_PORT) is squatted by
	// something else and must still be reported.
	checker := fakePortChecker{ports: map[int]bool{4640: true, 4641: true}}
	ownPorts := map[int]bool{4640: true}

	err := verifyPortBlockFree(checker, ownPorts, noProcesses, &inst, 32)
	if err == nil {
		t.Fatalf("verifyPortBlockFree() = nil, want a conflict error for 4641")
	}
	if !strings.Contains(err.Error(), "4641") {
		t.Errorf("error %q does not name port 4641", err)
	}
	if strings.Contains(err.Error(), "4640") {
		t.Errorf("error %q unexpectedly mentions this project's own port 4640", err)
	}
}

func TestVerifyPortBlockFree_NamesSquattingProcessWhenIdentifiable(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{ports: map[int]bool{4640: true}}
	lookup := func() (map[int]string, error) {
		return map[int]string{4640: "nginx"}, nil
	}

	err := verifyPortBlockFree(checker, nil, lookup, &inst, 32)
	if err == nil {
		t.Fatalf("verifyPortBlockFree() = nil, want a conflict error")
	}
	if !strings.Contains(err.Error(), "nginx") {
		t.Errorf("error %q does not name the squatting process", err)
	}
}

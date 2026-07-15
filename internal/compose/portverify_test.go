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

func TestVerifyPortBlockFree_NoConflict(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{ports: map[int]bool{9999: true}}

	if err := verifyPortBlockFree(checker, &inst, 32); err != nil {
		t.Errorf("verifyPortBlockFree() = %v, want nil", err)
	}
}

func TestVerifyPortBlockFree_NamesConflictingPort(t *testing.T) {
	inst := testInstanceForPortVerify()
	checker := fakePortChecker{ports: map[int]bool{4640: true}}

	err := verifyPortBlockFree(checker, &inst, 32)
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

	err := verifyPortBlockFree(checker, &inst, 32)
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

	if err := verifyPortBlockFree(checker, &inst, 32); err != nil {
		t.Errorf("verifyPortBlockFree() = %v, want nil when the checker itself fails", err)
	}
}

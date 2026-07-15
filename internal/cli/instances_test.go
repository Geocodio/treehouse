package cli

import (
	"testing"

	"github.com/geocodio/treehouse/internal/instance"
	"github.com/geocodio/treehouse/internal/machineconfig"
	"github.com/geocodio/treehouse/internal/manifest"
)

func TestResolveInstancesHome_ManifestLocationWins(t *testing.T) {
	manifestConfig := &manifest.Manifest{Location: "/manifest/override"}
	config := &machineconfig.Config{InstancesHome: "/machine/config/home"}

	if got := resolveInstancesHome(manifestConfig, config); got != "/manifest/override" {
		t.Errorf("resolveInstancesHome() = %q, want manifest override", got)
	}
}

func TestResolveInstancesHome_FallsBackToMachineConfig(t *testing.T) {
	manifestConfig := &manifest.Manifest{}
	config := &machineconfig.Config{InstancesHome: "/machine/config/home"}

	if got := resolveInstancesHome(manifestConfig, config); got != "/machine/config/home" {
		t.Errorf("resolveInstancesHome() = %q, want machine config default", got)
	}
}

func TestFindByBranch(t *testing.T) {
	instances := []instance.Instance{
		{Name: "feature-a", Branch: "feature/a"},
		{Name: "feature-b", Branch: "feature/b"},
	}

	found, ok := findByBranch(instances, "feature/b")
	if !ok || found.Name != "feature-b" {
		t.Errorf("findByBranch(feature/b) = %+v, %v, want feature-b, true", found, ok)
	}

	_, ok = findByBranch(instances, "feature/c")
	if ok {
		t.Errorf("findByBranch(feature/c) = _, true, want false")
	}
}

func TestCollidesWithDifferentBranch(t *testing.T) {
	instances := []instance.Instance{
		{Name: "feature-a", Branch: "feature/a"},
	}

	if collidesWithDifferentBranch(instances, "feature-a", "feature/a") {
		t.Errorf("collidesWithDifferentBranch() = true for the same branch, want false")
	}
	if !collidesWithDifferentBranch(instances, "feature-a", "feature/other") {
		t.Errorf("collidesWithDifferentBranch() = false for a different branch reusing the name, want true")
	}
	if collidesWithDifferentBranch(instances, "unused-name", "feature/other") {
		t.Errorf("collidesWithDifferentBranch() = true for an unused name, want false")
	}
}

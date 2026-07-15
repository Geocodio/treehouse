package manifest

import "testing"

// TestLoad_FixtureManifestIsValid guards against the committed
// testdata/fixture/.treehouse.yml drifting out of sync with the schema
// this package enforces.
func TestLoad_FixtureManifestIsValid(t *testing.T) {
	result, warnings, err := Load("../../testdata/fixture/.treehouse.yml")
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("Load() returned unexpected warnings: %v", warnings)
	}

	if result.Project != "fixture" {
		t.Errorf("Project = %q, want %q", result.Project, "fixture")
	}
	if result.Provision.Hook != "scripts/treehouse-post-create.sh" {
		t.Errorf("Provision.Hook = %q, want scripts/treehouse-post-create.sh", result.Provision.Hook)
	}
}

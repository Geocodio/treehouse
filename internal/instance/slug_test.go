package instance

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"my-feature", "my-feature"},
		{"feature/my-feature", "my-feature"},
		{"feature/nested/my-feature", "my-feature"},
		{"Feature/My_Feature", "my_feature"},
		{"ENG-1234-fix-the-thing", "eng-1234-fix-the-thing"},
		// Per DESIGN.md's rule ([^a-z0-9_-] -> -), each disallowed
		// character folds to its own hyphen; runs are not collapsed.
		{"feature/Weird!! Name??", "weird---name"},
		{"---", fallbackSlug},
		{"", fallbackSlug},
	}

	for _, test := range tests {
		if got := Slugify(test.branch); got != test.want {
			t.Errorf("Slugify(%q) = %q, want %q", test.branch, got, test.want)
		}
	}
}

func TestResolveName_NoCollision(t *testing.T) {
	name, warning := ResolveName(nil, "feature/my-feature")
	if name != "my-feature" {
		t.Errorf("name = %q, want my-feature", name)
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}

func TestResolveName_IdempotentForSameBranch(t *testing.T) {
	existing := []Instance{{Name: "my-feature", Branch: "feature/my-feature"}}

	name, warning := ResolveName(existing, "feature/my-feature")
	if name != "my-feature" {
		t.Errorf("name = %q, want my-feature (idempotent)", name)
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}

func TestResolveName_CollisionWithDifferentBranchAutoSuffixes(t *testing.T) {
	existing := []Instance{{Name: "my-feature", Branch: "other/my-feature"}}

	name, warning := ResolveName(existing, "feature/my-feature")
	if name != "my-feature-2" {
		t.Errorf("name = %q, want my-feature-2", name)
	}
	if warning == "" {
		t.Errorf("warning = empty, want a collision warning")
	}
}

func TestResolveName_MultipleCollisionsIncrementSuffix(t *testing.T) {
	existing := []Instance{
		{Name: "my-feature", Branch: "other/my-feature"},
		{Name: "my-feature-2", Branch: "another/my-feature"},
	}

	name, warning := ResolveName(existing, "feature/my-feature")
	if name != "my-feature-3" {
		t.Errorf("name = %q, want my-feature-3", name)
	}
	if warning == "" {
		t.Errorf("warning = empty, want a collision warning")
	}
}

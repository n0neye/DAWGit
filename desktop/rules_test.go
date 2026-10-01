package main

import (
	"fmt"
	"testing"

	"dawgit/internal/profile"
)

// The file DAWGit creates for "Create .dawgit.yaml" is valid as it is.
func TestRulesTemplate(t *testing.T) {
	p, err := profile.Parse([]byte(fmt.Sprintf(rulesTemplate, "0.6")), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if a := p.Applied(); len(a) != 1 || a[0].Preset != "ableton" || a[0].Detected {
		t.Fatalf("applied: %+v", a)
	}
	if len(p.Rules) != 0 || !p.Ignored("Backup", true) {
		t.Fatalf("rules: %+v", p.Rules)
	}
}

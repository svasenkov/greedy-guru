package status_test

import (
	"testing"

	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/status"
)

func TestDiffUnchanged(t *testing.T) {
	a := sample("login", "abc")
	b := sample("login", "abc")
	d := status.Diff(a, b)
	if d.Changed {
		t.Fatalf("%+v", d)
	}
}

func TestDiffFingerprintAndStep(t *testing.T) {
	a := sample("login", "old")
	b := sample("login", "new")
	b.Steps = append(b.Steps, crystal.Step{Op: "click", Selector: "[data-testid=x]"})
	d := status.Diff(a, b)
	if !d.Changed || d.FingerprintA == d.FingerprintB {
		t.Fatalf("%+v", d)
	}
	if len(d.Changes) < 2 {
		t.Fatalf("changes %+v", d.Changes)
	}
}

func TestDiffFirstCrystal(t *testing.T) {
	d := status.Diff(nil, sample("login", "abc"))
	if !d.Changed {
		t.Fatal(d)
	}
}

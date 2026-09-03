package status

import (
	"fmt"

	"greedy.guru/greedy/internal/crystal"
)

type Change struct {
	Path string `json:"path"`
	A    string `json:"a,omitempty"`
	B    string `json:"b,omitempty"`
}

type DiffResult struct {
	Changed      bool     `json:"changed"`
	FingerprintA string   `json:"fingerprint_a,omitempty"`
	FingerprintB string   `json:"fingerprint_b,omitempty"`
	Changes      []Change `json:"changes"`
}

func Diff(a, b *crystal.Crystal) DiffResult {
	d := DiffResult{Changes: []Change{}}
	if b == nil {
		d.Changed = true
		d.Changes = append(d.Changes, Change{Path: "crystal", A: present(a), B: ""})
		return d
	}
	if a == nil {
		d.FingerprintB = FingerprintOf(b)
		d.Changed = true
		d.Changes = append(d.Changes, Change{Path: "crystal", A: "", B: b.ID})
		return d
	}
	d.FingerprintA = FingerprintOf(a)
	d.FingerprintB = FingerprintOf(b)
	add := func(path, av, bv string) {
		if av == bv {
			return
		}
		d.Changes = append(d.Changes, Change{Path: path, A: av, B: bv})
	}
	add("id", a.ID, b.ID)
	add("kind", a.Kind, b.Kind)
	add("version", fmt.Sprintf("%d", a.Version), fmt.Sprintf("%d", b.Version))
	add("source.as_id", a.Source.ASID, b.Source.ASID)
	add("source.lang", a.Source.Lang, b.Source.Lang)
	add("source.fingerprint", d.FingerprintA, d.FingerprintB)
	n := len(a.Steps)
	if len(b.Steps) > n {
		n = len(b.Steps)
	}
	for i := 0; i < n; i++ {
		if i >= len(a.Steps) {
			add(fmt.Sprintf("steps[%d]", i), "", stepLine(b.Steps[i]))
			continue
		}
		if i >= len(b.Steps) {
			add(fmt.Sprintf("steps[%d]", i), stepLine(a.Steps[i]), "")
			continue
		}
		add(fmt.Sprintf("steps[%d]", i), stepLine(a.Steps[i]), stepLine(b.Steps[i]))
	}
	d.Changed = len(d.Changes) > 0
	return d
}

func stepLine(s crystal.Step) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s", s.Op, s.Selector, s.URL, s.Value)
}

func present(c *crystal.Crystal) string {
	if c == nil {
		return ""
	}
	return c.ID
}

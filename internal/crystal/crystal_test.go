package crystal_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"greedy.guru/greedy/internal/crystal"
)

func TestLoadLoginExample(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "crystals", "login.example.json")
	c, err := crystal.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "example" || c.Kind != "cdp" || len(c.Steps) != 7 {
		t.Fatalf("unexpected crystal: %+v", c)
	}
}

func TestRejectUnknownOp(t *testing.T) {
	c := crystal.Crystal{ID: "x", Kind: "cdp", Version: 1, Steps: []crystal.Step{{Op: "hover"}}}
	if err := c.Validate(); err == nil {
		t.Fatal("want error")
	}
}

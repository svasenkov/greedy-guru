package liveutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMillCrystalFindsLogin(t *testing.T) {
	t.Setenv("GREEDY_CRYSTALS", "")
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path, err := MillCrystal(filepath.Dir(file), "login.json")
	if err != nil {
		t.Skip(err.Error())
	}
	if filepath.Base(path) != "login.json" {
		t.Fatalf("%s", path)
	}
}

func TestMillCrystalsDirEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "login.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREEDY_CRYSTALS", dir)
	got, err := MillCrystalsDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %s", got)
	}
	path, err := MillCrystal(t.TempDir(), "login.json")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("%s", path)
	}
}

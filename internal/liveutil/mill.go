package liveutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Mill crystals = live IR etalon (tests-go-cdp). Guru crystals/ keeps login.example.json only.
func millRelatives() []string {
	return []string{
		filepath.Join("tests", "go", "tests-go-cdp", "crystals"),
		filepath.Join("autotests-ai-multistack-app", "tests", "go", "tests-go-cdp", "crystals"),
		filepath.Join("projects", "autotests-ai-multistack-home", "autotests-ai-multistack-app", "tests", "go", "tests-go-cdp", "crystals"),
	}
}

// MillCrystalsDir finds tests-go-cdp/crystals. GREEDY_CRYSTALS overrides (dir, not a file).
func MillCrystalsDir(start string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("GREEDY_CRYSTALS")); v != "" {
		st, err := os.Stat(v)
		if err != nil {
			return "", fmt.Errorf("GREEDY_CRYSTALS: %w", err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("GREEDY_CRYSTALS: not a directory")
		}
		return v, nil
	}
	dir := start
	for i := 0; i < 14; i++ {
		for _, rel := range millRelatives() {
			cand := filepath.Join(dir, rel)
			if _, err := os.Stat(filepath.Join(cand, "login.json")); err == nil {
				return cand, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("mill crystals: set GREEDY_CRYSTALS or run in the monorepo (tests-go-cdp)")
}

func MillCrystal(start, name string) (string, error) {
	dir, err := MillCrystalsDir(start)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("mill crystal %s: %w", name, err)
	}
	return path, nil
}

func MillCrystalOrSkip(t *testing.T, start, name string) string {
	t.Helper()
	path, err := MillCrystal(start, name)
	if err != nil {
		t.Skip(err.Error())
	}
	return path
}

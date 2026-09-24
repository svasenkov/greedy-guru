package cli_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"greedy.guru/greedy/internal/cli"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden files with actual output")

// goldenDir is <module>/testdata/golden.
func goldenDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "testdata", "golden")
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(goldenDir(t), name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -update to create)", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden %s mismatch\n--- got ---\n%s\n--- want ---\n%s\n(run with -update to regenerate)", name, got, want)
	}
}

// dispatchGolden runs the CLI and returns stdout bytes; stderr is dropped
// (usage text on errors is covered by TestHelpStable, not golden).
func dispatchGolden(t *testing.T, args ...string) ([]byte, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Dispatch(args, &out, &errb)
	return out.Bytes(), code
}

func TestGoldenCrystallizeGates(t *testing.T) {
	out, code := dispatchGolden(t,
		"crystallize", "--hits", "3", "--days", "2", "--pattern", "login valid credentials",
	)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "crystallize-gates.golden", out)
}

func TestGoldenCrystallizeTrace(t *testing.T) {
	src := filepath.Join(moduleRoot(t), "testdata", "trace", "login.trace")
	outFile := filepath.Join(t.TempDir(), "login.json")
	out, code := dispatchGolden(t,
		"crystallize", "--hits", "3", "--days", "2", "--pattern", "login valid credentials",
		"--trace", src, "--out", outFile,
		"--id", "login",
		"--as-id", "Пользователь может войти с валидными credentials",
	)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "crystallize-trace.stdout.golden", out)

	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "crystallize-trace.ir.golden", raw)
}

func TestGoldenValidate(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	out, code := dispatchGolden(t, "validate", path)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "validate-example.golden", out)
}

func TestGoldenDiffSame(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	out, code := dispatchGolden(t, "diff", path, path)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "diff-same.golden", out)
}

func TestGoldenObserveLoginAutoLive(t *testing.T) {
	out, code := dispatchGolden(t,
		"observe", "--id", "login", "--eligible", "--pw-green", "3",
	)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "observe-login-autolive.golden", out)
}

func TestGoldenObservePendingReview(t *testing.T) {
	out, code := dispatchGolden(t,
		"observe", "--id", "register-user1", "--eligible", "--pw-green", "3",
	)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "observe-pending.golden", out)
}

func TestGoldenApprove(t *testing.T) {
	t.Setenv("ALLURE_ENDPOINT", "")
	t.Setenv("ALLURE_TOKEN", "")
	t.Setenv("ALLURE_API_TOKEN", "")
	src := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	outPath := filepath.Join(t.TempDir(), "login.json")
	out, code := dispatchGolden(t,
		"approve", "--proposed", src, "--live", src, "--out", outPath,
		"--pw-green", "3", "--eligible", "--testcase", "99",
	)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "approve.stdout.golden",
		bytes.ReplaceAll(out, []byte(outPath), []byte("<OUT>")))

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "approve-file.golden", raw)
}

func TestGoldenUnknownCommand(t *testing.T) {
	out, code := dispatchGolden(t, "wat")
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "unknown-command.golden", out)
}

func TestGoldenValidateMissing(t *testing.T) {
	out, code := dispatchGolden(t, "validate", "testdata/trace/nope.json")
	if code != cli.ExitFail {
		t.Fatalf("code %d %s", code, out)
	}
	assertGolden(t, "validate-missing.golden", out)
}

func TestVersionFormat(t *testing.T) {
	out, code := dispatchGolden(t, "version")
	if code != cli.ExitOK {
		t.Fatalf("code %d", code)
	}
	s := strings.TrimSpace(string(out))
	if !strings.HasPrefix(s, "greedy ") || len(strings.TrimPrefix(s, "greedy ")) == 0 {
		t.Fatalf("version output %q", s)
	}
}

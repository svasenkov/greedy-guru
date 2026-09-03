package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"greedy.guru/greedy/internal/cli"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod")
	return ""
}

func TestHelpStable(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"help"}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d", code)
	}
	s := errb.String()
	for _, n := range []string{"greedy search", "greedy crystallize", "greedy run", "greedy bench", "greedy validate", "greedy observe", "greedy diff", "greedy approve", "Exit: 0 ok", "--parallel", "--mode none", "from_test_result"} {
		if !strings.Contains(s, n) {
			t.Fatalf("help missing %q\n%s", n, s)
		}
	}
	if errb.String() != cli.Usage {
		t.Fatal("help text drifted from Usage const")
	}
}

func TestUsageExit2(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch(nil, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d", code)
	}
}

func TestValidateJSON(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"validate", path}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true || m["id"] != "example" {
		t.Fatalf("%v", m)
	}
}

func TestSearchFixture(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not in PATH")
	}
	dir := filepath.Join(moduleRoot(t), "testdata", "search")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"search", "--path", dir, "p1-search-marker-a1b2"}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "p1-search-marker-a1b2") {
		t.Fatal(out.String())
	}
}

func TestSearchNoHits(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not in PATH")
	}
	dir := filepath.Join(moduleRoot(t), "testdata", "search")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"search", "--path", dir, "zzznomatchzzzguru"}, &out, &errb)
	if code != cli.ExitFail {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestCrystallizeEligible(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"crystallize", "--hits", "3", "--days", "2", "--pattern", "login valid credentials",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"codegen":false`) {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), "pending_review") {
		t.Fatal(out.String())
	}
}

func TestCrystallizeReject(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"crystallize", "--hits", "1", "--pattern", "login valid credentials"}, &out, &errb)
	if code != cli.ExitFail {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestCrystallizeFromTrace(t *testing.T) {
	src := filepath.Join(moduleRoot(t), "testdata", "trace", "login.trace")
	out := filepath.Join(t.TempDir(), "login.json")
	var stdout, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"crystallize", "--hits", "3", "--days", "2", "--pattern", "login valid credentials",
		"--trace", src, "--out", out,
		"--id", "login",
		"--as-id", "Пользователь может войти с валидными credentials",
	}, &stdout, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"codegen":true`) {
		t.Fatal(stdout.String())
	}
	if !strings.Contains(stdout.String(), `[data-testid=login-input]`) {
		t.Fatal(stdout.String())
	}
	c, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(c), `"id": "login"`) {
		t.Fatal(string(c))
	}
}

func TestBenchMissingCrystal(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"bench", "--base-url", "http://127.0.0.1/"}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestBenchMissingBaseURL(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"bench", path}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestRunMissingCDPUsage(t *testing.T) {
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", "off")
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"run", path}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestRunModeNotNone(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"run", "--cdp", "http://127.0.0.1:9", "--mode", "allure", path}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "mode none") {
		t.Fatal(out.String())
	}
}

func TestRunParallelZero(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"run", "--cdp", "http://127.0.0.1:9", "--parallel", "0", path}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestRunParallelSingleCDPRejected(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"run", "--parallel", "2", "--cdp", "http://127.0.0.1:9222", path,
	}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "not N tabs") {
		t.Fatal(out.String())
	}
}

func TestRunParallelEnvCDPRejected(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	t.Setenv("GREEDY_CDP", "http://127.0.0.1:9222")
	t.Setenv("GREEDY_POOL", "off")
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"run", "--parallel", "2", "--base-url", "http://127.0.0.1/", path}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "not N tabs") {
		t.Fatal(out.String())
	}
}

func TestObservePendingReview(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"observe", "--id", "register-user1", "--eligible", "--pw-green", "3",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"crystal_status":"pending_review"`) {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), `"from_test_result":true`) {
		t.Fatal(out.String())
	}
}

func TestObserveAlreadyLiveNoPatch(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"observe", "--id", "logout", "--eligible", "--pw-green", "3",
		"--was", "live", "--fingerprint", "abc", "--live-fingerprint", "abc",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"crystal_status":"live"`) {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), `"patch"`) {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), "pending_review") {
		t.Fatal(out.String())
	}
}

func TestObserveLoginAutoLive(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"observe", "--id", "login", "--eligible", "--pw-green", "3",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"crystal_status":"live"`) || !strings.Contains(out.String(), `"auto_live":true`) {
		t.Fatal(out.String())
	}
}

func TestObserveSpecFix(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"observe", "--id", "login", "--eligible", "--spec-fix", "--was", "live",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), "pending_review") {
		t.Fatal(out.String())
	}
}

func TestObserveManualLayer(t *testing.T) {
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"observe", "--id", "login", "--eligible", "--pw-green", "3", "--layer", "manual",
	}, &out, &errb)
	if code != cli.ExitFail {
		t.Fatalf("code %d %s", code, out.String())
	}
}

func TestDiffLoginExample(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"diff", path, path}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"changed":false`) {
		t.Fatal(out.String())
	}
}

func TestApproveWritesJSON(t *testing.T) {
	t.Setenv("ALLURE_ENDPOINT", "")
	t.Setenv("ALLURE_TOKEN", "")
	t.Setenv("ALLURE_API_TOKEN", "")
	src := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	outPath := filepath.Join(t.TempDir(), "login.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{
		"approve", "--proposed", src, "--live", src, "--out", outPath,
		"--pw-green", "3", "--eligible", "--testcase", "99",
	}, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"id": "example"`) {
		t.Fatal(string(raw))
	}
	if !strings.Contains(out.String(), `"crystal_status":"live"`) {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), `"wrote":`) {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), `from_test_result":true`) {
		t.Fatal(out.String())
	}
}

func TestApproveRequiresOut(t *testing.T) {
	src := filepath.Join(moduleRoot(t), "crystals", "login.example.json")
	var out, errb bytes.Buffer
	code := cli.Dispatch([]string{"approve", "--proposed", src, "--pw-green", "3", "--eligible"}, &out, &errb)
	if code != cli.ExitUsage {
		t.Fatalf("code %d %s", code, out.String())
	}
}

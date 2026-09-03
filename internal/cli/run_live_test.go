package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/cli"
	"greedy.guru/greedy/internal/liveutil"
)

func parallelN(t *testing.T) int {
	t.Helper()
	s := os.Getenv("GREEDY_PARALLEL")
	if s == "" {
		return 2
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		t.Fatalf("GREEDY_PARALLEL=%q", s)
	}
	return n
}

// dockerAppURL rewrites httptest origin so Chromium in hot-cdp-min-* can fetch the host fixture.
func dockerAppURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return "", err
	}
	u.Host = net.JoinHostPort("host.docker.internal", port)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func TestDispatchRunParallelLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	root := moduleRoot(t)
	n := parallelN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	crystalPath := liveutil.MillCrystalOrSkip(t, root, "login.json")
	cdps := make([]string, n)
	for i := 0; i < n; i++ {
		cdps[i] = liveutil.StartChrome(t, ctx)
	}
	args := []string{"run", "--parallel", strconv.Itoa(n), "--mode", "none", "--base-url", liveutil.AppURL(srv)}
	for _, u := range cdps {
		args = append(args, "--cdp", u)
	}
	args = append(args, crystalPath)

	var out, errb bytes.Buffer
	code := cli.Dispatch(args, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	var res cdp.ParallelResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Parallel != n || res.OKCount != n || res.Mode != "none" || res.AllureGenerate {
		t.Fatalf("%s", out.String())
	}
	t.Logf("greedy run --parallel %d wall_ms=%d workers=%v", n, res.WallMS, res.Workers)

	if dest := os.Getenv("GREEDY_BENCH_OUT"); dest != "" {
		payload := map[string]any{
			"ok":              res.OK,
			"id":              res.ID,
			"steps":           res.Steps,
			"parallel":        res.Parallel,
			"mode":            res.Mode,
			"runs":            res.Runs,
			"ok_count":        res.OKCount,
			"wall_ms":         res.WallMS,
			"allure_generate": res.AllureGenerate,
			"workers":         res.Workers,
			"measured_at":     time.Now().UTC().Format("2006-01-02"),
			"cdp":             strconv.Itoa(n) + " pw-min CDP (qaguru/playwright-chromium:1.61.1-min, not WD, not ensure.py, not selenoid-pool)",
			"crystal":         "tests-go-cdp/crystals/login.json",
			"app":             "testdata/app-live",
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDispatchRunParallelPoolLogin(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_BENCH", "")
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GREEDY_POOL"))) {
	case "0", "off", "false", "no":
		t.Setenv("GREEDY_POOL", liveutil.DefaultPoolURL)
	}
	root := moduleRoot(t)
	n := parallelN(t)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	base, err := dockerAppURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"run", "--parallel", strconv.Itoa(n), "--mode", "none",
		"--base-url", base, liveutil.MillCrystalOrSkip(t, root, "login.json"),
	}
	var out, errb bytes.Buffer
	code := cli.Dispatch(args, &out, &errb)
	if code != cli.ExitOK {
		t.Fatalf("code %d %s", code, out.String())
	}
	var res cdp.ParallelResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Parallel != n || res.OKCount != n || res.Mode != "none" || res.AllureGenerate {
		t.Fatalf("%s", out.String())
	}
	t.Logf("greedy run --parallel %d pool leases wall_ms=%d workers=%v", n, res.WallMS, res.Workers)
}

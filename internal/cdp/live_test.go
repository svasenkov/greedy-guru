package cdp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/liveutil"
	"greedy.guru/greedy/internal/trace"
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

func dialChrome(t *testing.T, ctx context.Context) *cdp.Client {
	t.Helper()
	debug := strings.TrimSpace(os.Getenv("GREEDY_CDP"))
	if debug == "" {
		debug = liveutil.StartChrome(t, ctx)
	}
	sess, err := cdp.Dial(ctx, debug)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestRunLoginFixtureChrome(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	root := moduleRoot(t)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	sess := dialChrome(t, ctx)
	cr, err := crystal.Load(filepath.Join(root, "crystals", "login.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	res := cdp.Run(ctx, sess, cr, cdp.Options{BaseURL: liveutil.AppURL(srv)})
	if !res.OK {
		t.Fatalf("run: %s wall_ms=%d", res.Error, res.WallMS)
	}
	t.Logf("login fixture wall_ms=%d", res.WallMS)
}

func TestRunLoginFromTraceChrome(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	root := moduleRoot(t)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	sess := dialChrome(t, ctx)
	cr, err := trace.ToCrystal(filepath.Join(root, "testdata", "trace", "login.trace"), trace.Options{
		ID:   "login",
		ASID: "Пользователь может войти с валидными credentials",
	})
	if err != nil {
		t.Fatal(err)
	}
	res := cdp.Run(ctx, sess, cr, cdp.Options{BaseURL: liveutil.AppURL(srv)})
	if !res.OK {
		t.Fatalf("run: %s wall_ms=%d", res.Error, res.WallMS)
	}
	t.Logf("login from trace wall_ms=%d", res.WallMS)
}

func TestRunParallelLoginChrome(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	root := moduleRoot(t)
	n := 2
	if s := os.Getenv("GREEDY_PARALLEL"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 32 {
			t.Fatalf("GREEDY_PARALLEL=%q", s)
		}
		n = v
	}
	chromes := 1
	if s := os.Getenv("GREEDY_CHROMES"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > n {
			t.Fatalf("GREEDY_CHROMES=%q", s)
		}
		chromes = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30+20*n)*time.Second)
	t.Cleanup(cancel)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	callers := make([]cdp.Caller, n)
	if chromes == 1 {
		debug := liveutil.StartChrome(t, ctx)
		for i := 0; i < n; i++ {
			sess, err := cdp.Dial(ctx, debug)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			callers[i] = sess
		}
	} else {
		if chromes != n {
			t.Fatalf("GREEDY_CHROMES must be 1 or equal GREEDY_PARALLEL")
		}
		for i := 0; i < n; i++ {
			sess, err := cdp.Dial(ctx, liveutil.StartChrome(t, ctx))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			callers[i] = sess
		}
	}
	cr, err := crystal.Load(liveutil.MillCrystalOrSkip(t, root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	opt := cdp.Options{BaseURL: liveutil.AppURL(srv)}
	if os.Getenv("GREEDY_BENCH") == "1" {
		warm := cdp.RunParallel(ctx, callers, cr, opt)
		if !warm.OK {
			t.Fatalf("warmup: %s", warm.Error)
		}
	}
	res := cdp.RunParallel(ctx, callers, cr, opt)
	if !res.OK {
		t.Fatalf("parallel: %s wall_ms=%d workers=%v", res.Error, res.WallMS, res.Workers)
	}
	if res.Parallel != n || res.OKCount != n || res.Mode != "none" || res.AllureGenerate {
		t.Fatalf("%+v", res)
	}
	t.Logf("parallel %d×login wall_ms=%d chromes=%d workers=%v", n, res.WallMS, chromes, res.Workers)
	if os.Getenv("GREEDY_BENCH") == "1" {
		if res.Parallel > 1 && chromes == 1 {
			t.Fatal("do not write site/bench.json N× from 1 Chrome (tabs, not N-way)")
		}
		writeSiteBench(t, root, res, chromes)
	}
}

func writeSiteBench(t *testing.T, root string, res cdp.ParallelResult, chromes int) {
	t.Helper()
	workers := make([]map[string]any, 0, len(res.Workers))
	for _, w := range res.Workers {
		workers = append(workers, map[string]any{
			"ok": w.OK, "id": w.ID, "steps": w.Steps, "wall_ms": w.WallMS,
		})
	}
	cdpNote := "1 hot Chrome"
	if chromes > 1 {
		cdpNote = strconv.Itoa(chromes) + " Chrome processes (N-way, not tabs)"
	}
	payload := map[string]any{
		"allure_generate": res.AllureGenerate,
		"app":             "testdata/app-live",
		"cdp":             cdpNote,
		"crystal":         "tests-go-cdp/crystals/login.json",
		"id":              res.ID,
		"measured_at":     time.Now().UTC().Format("2006-01-02"),
		"mode":            res.Mode,
		"ok":              res.OK,
		"ok_count":        res.OKCount,
		"parallel":        res.Parallel,
		"runs":            res.Runs,
		"steps":           res.Steps,
		"wall_ms":         res.WallMS,
		"workers":         workers,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "site", "bench.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s wall_ms=%d", path, res.WallMS)
}

func TestRunLoginSPAChrome(t *testing.T) {
	if testing.Short() || os.Getenv("GREEDY_SPA") != "1" {
		t.Skip("set GREEDY_SPA=1 for autotests.ai login")
	}
	root := moduleRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	sess := dialChrome(t, ctx)
	cr, err := crystal.Load(liveutil.MillCrystalOrSkip(t, root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	base := "https://autotests.ai/stack/backend-java-spring/frontend-typescript-react/"
	res := cdp.Run(ctx, sess, cr, cdp.Options{BaseURL: base})
	if !res.OK {
		t.Fatalf("run: %s wall_ms=%d step_ms=%v", res.Error, res.WallMS, res.StepMS)
	}
	t.Logf("spa login wall_ms=%d step_ms=%v", res.WallMS, res.StepMS)
}

func TestRunLoginSPAHotChrome(t *testing.T) {
	if testing.Short() || os.Getenv("GREEDY_SPA_HOT") != "1" {
		t.Skip("set GREEDY_SPA_HOT=1: park on SPA /login, then mill (hot-pool analog, CDP not Playwright WS)")
	}
	root := moduleRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	lease, err := liveutil.ResolveCDP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	sess, err := cdp.Dial(ctx, lease.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	t.Logf("hot cdp source=%s", lease.Source)
	base := "https://autotests.ai/stack/backend-java-spring/frontend-typescript-react/"
	park := &crystal.Crystal{
		ID: "park-login", Kind: "cdp", Version: 1,
		Steps: []crystal.Step{
			{Op: "navigate", URL: "login"},
			{Op: "wait", Selector: "[data-testid=login-input]", TimeoutMS: 15000},
		},
	}
	pr := cdp.Run(ctx, sess, park, cdp.Options{BaseURL: base})
	if !pr.OK {
		t.Fatalf("park: %s wall_ms=%d step_ms=%v", pr.Error, pr.WallMS, pr.StepMS)
	}
	cr, err := crystal.Load(liveutil.MillCrystalOrSkip(t, root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	res := cdp.Run(ctx, sess, cr, cdp.Options{BaseURL: base})
	if !res.OK {
		t.Fatalf("hot mill: %s wall_ms=%d step_ms=%v", res.Error, res.WallMS, res.StepMS)
	}
	t.Logf("spa hot mill wall_ms=%d park_ms=%d step_ms=%v", res.WallMS, pr.WallMS, res.StepMS)
}

func TestRunLoginMetricsHotCold(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	root := moduleRoot(t)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	sess := dialChrome(t, ctx)
	cr, err := crystal.Load(liveutil.MillCrystalOrSkip(t, root, "login.json"))
	if err != nil {
		t.Fatal(err)
	}
	opt := cdp.Options{BaseURL: liveutil.AppURL(srv)}
	cold := cdp.Run(ctx, sess, cr, opt)
	if !cold.OK {
		t.Fatalf("cold: %s wall_ms=%d", cold.Error, cold.WallMS)
	}
	hot := cdp.Run(ctx, sess, cr, opt)
	if !hot.OK {
		t.Fatalf("hot: %s wall_ms=%d", hot.Error, hot.WallMS)
	}
	if cold.CdpCommands != cold.RoundTrips || hot.CdpCommands != hot.RoundTrips {
		t.Fatalf("batching appeared: cold %d/%d hot %d/%d", cold.CdpCommands, cold.RoundTrips, hot.CdpCommands, hot.RoundTrips)
	}
	t.Logf("login.json app-live cold elapsed_ms=%d reset_ms=%d cdp_commands=%d round_trips=%d methods=%v step_ms=%v",
		cold.WallMS, cold.ResetMS, cold.CdpCommands, cold.RoundTrips, cold.CdpMethods, cold.StepMS)
	t.Logf("login.json app-live hot elapsed_ms=%d reset_ms=%d cdp_commands=%d round_trips=%d methods=%v step_ms=%v",
		hot.WallMS, hot.ResetMS, hot.CdpCommands, hot.RoundTrips, hot.CdpMethods, hot.StepMS)
}

func pingSpecs(n int) []cdp.CallSpec {
	out := make([]cdp.CallSpec, n)
	for i := range out {
		out[i] = cdp.CallSpec{
			Method: "Runtime.evaluate",
			Params: map[string]any{"expression": "1", "returnByValue": true, "awaitPromise": false},
		}
	}
	return out
}

func callSeq(ctx context.Context, t *testing.T, sess *cdp.Client, specs []cdp.CallSpec) {
	t.Helper()
	for _, s := range specs {
		if _, err := sess.Call(ctx, s.Method, s.Params); err != nil {
			t.Fatal(err)
		}
	}
}

func medianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func TestMeasureCDPPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("live Chrome")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	sess := dialChrome(t, ctx)
	n := 11
	specs := pingSpecs(n)
	rounds := 5
	callSeq(ctx, t, sess, specs[:1])

	seqUS := make([]int64, rounds)
	pipeUS := make([]int64, rounds)
	for i := 0; i < rounds; i++ {
		t0 := time.Now()
		callSeq(ctx, t, sess, specs)
		seqUS[i] = time.Since(t0).Microseconds()
		t1 := time.Now()
		if err := sess.CallPipeline(ctx, specs); err != nil {
			t.Fatal(err)
		}
		pipeUS[i] = time.Since(t1).Microseconds()
	}

	root := moduleRoot(t)
	srv := liveutil.ServeApp(t, filepath.Join(root, "testdata", "app-live"))
	origin := strings.TrimRight(liveutil.AppURL(srv), "/")
	reset := []cdp.CallSpec{
		{Method: "Network.enable"},
		{Method: "Network.clearBrowserCookies", Params: map[string]any{}},
		{Method: "Storage.clearDataForOrigin", Params: map[string]any{
			"origin": origin, "storageTypes": "cookies,local_storage",
		}},
	}
	callSeq(ctx, t, sess, reset)
	resetSeqUS := make([]int64, rounds)
	resetPipeUS := make([]int64, rounds)
	for i := 0; i < rounds; i++ {
		t2 := time.Now()
		callSeq(ctx, t, sess, reset)
		resetSeqUS[i] = time.Since(t2).Microseconds()
		t3 := time.Now()
		if err := sess.CallPipeline(ctx, reset); err != nil {
			t.Fatal(err)
		}
		resetPipeUS[i] = time.Since(t3).Microseconds()
	}

	seqMed := medianInt64(seqUS)
	pipeMed := medianInt64(pipeUS)
	resetSeqMed := medianInt64(resetSeqUS)
	resetPipeMed := medianInt64(resetPipeUS)
	t.Logf("cdp pipeline n=%d rounds=%d ping_seq_us=%v med=%d ping_pipe_us=%v med=%d ping_save_us=%d reset3_seq_us=%v med=%d reset3_pipe_us=%v med=%d reset3_save_us=%d",
		n, rounds, seqUS, seqMed, pipeUS, pipeMed, seqMed-pipeMed, resetSeqUS, resetSeqMed, resetPipeUS, resetPipeMed, resetSeqMed-resetPipeMed)
	t.Logf("vs P3 step_ms: navigate 44–81 wait 44–61 fill/click ≤2 text 0–2; pipeline cuts WS RTT only, not navigate/wait")
}

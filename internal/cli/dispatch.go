package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/crystallize"
	"greedy.guru/greedy/internal/liveutil"
	"greedy.guru/greedy/internal/search"
	"greedy.guru/greedy/internal/trace"
)

func Dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, Usage)
		emit(stdout, map[string]any{"ok": false, "error": "usage"})
		return ExitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		_, _ = io.WriteString(stderr, Usage)
		return ExitOK
	case "version", "-v", "--version":
		_, _ = io.WriteString(stdout, "greedy "+Version+"\n")
		return ExitOK
	case "search":
		return cmdSearch(args[1:], stdout)
	case "validate":
		return cmdValidate(args[1:], stdout)
	case "crystallize":
		return cmdCrystallize(args[1:], stdout)
	case "run":
		return cmdRun(args[1:], stdout)
	case "bench":
		return cmdBench(args[1:], stdout)
	case "observe":
		return cmdObserve(args[1:], stdout)
	case "diff":
		return cmdDiff(args[1:], stdout)
	case "approve":
		return cmdApprove(args[1:], stdout)
	default:
		_, _ = io.WriteString(stderr, Usage)
		emit(stdout, map[string]any{"ok": false, "error": "unknown command " + args[0]})
		return ExitUsage
	}
}

func cmdSearch(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{"path": true, "glob": true})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	query := strings.Join(rest, " ")
	res, code := search.Run(context.Background(), query, flags["path"], flags["glob"])
	emit(stdout, res)
	return code
}

func cmdValidate(args []string, stdout io.Writer) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		emit(stdout, map[string]any{"ok": false, "error": "validate: path required"})
		return ExitUsage
	}
	c, err := crystal.Load(args[0])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	emit(stdout, map[string]any{"ok": true, "id": c.ID, "kind": c.Kind, "steps": len(c.Steps)})
	return ExitOK
}

func cmdCrystallize(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{
		"hits": true, "days": true, "sessions": true, "pattern": true, "faker": false,
		"trace": true, "out": true, "id": true, "as-id": true,
	})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	if len(rest) > 0 {
		emit(stdout, map[string]any{"ok": false, "error": "crystallize: extra args"})
		return ExitUsage
	}
	hits, err := atoiDefault(flags["hits"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "crystallize: --hits int"})
		return ExitUsage
	}
	days, err := atoiDefault(flags["days"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "crystallize: --days int"})
		return ExitUsage
	}
	sessions, err := atoiDefault(flags["sessions"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "crystallize: --sessions int"})
		return ExitUsage
	}
	_, faker := flags["faker"]
	res := crystallize.Check(hits, days, sessions, faker, flags["pattern"])
	if !res.OK {
		emit(stdout, res)
		return ExitFail
	}
	if flags["trace"] == "" {
		if flags["out"] != "" {
			emit(stdout, map[string]any{"ok": false, "error": "crystallize: --out needs --trace"})
			return ExitUsage
		}
		emit(stdout, res)
		return ExitOK
	}
	c, err := trace.ToCrystal(flags["trace"], trace.Options{ID: flags["id"], ASID: flags["as-id"]})
	if err != nil {
		res.OK = false
		res.Codegen = false
		res.Eligible = false
		res.Promote = ""
		res.Error = err.Error()
		emit(stdout, res)
		return ExitFail
	}
	res.Codegen = true
	res.Crystal = c
	if out := flags["out"]; out != "" {
		raw, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			res.OK = false
			res.Error = err.Error()
			emit(stdout, res)
			return ExitFail
		}
		if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
			res.OK = false
			res.Error = err.Error()
			emit(stdout, res)
			return ExitFail
		}
	}
	emit(stdout, res)
	return ExitOK
}

func cmdRun(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{
		"cdp": true, "base-url": true, "parallel": true, "mode": true,
	})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	if len(rest) != 1 {
		emit(stdout, map[string]any{"ok": false, "error": "run: crystal.json required"})
		return ExitUsage
	}
	mode := flags["mode"]
	if mode == "" {
		mode = "none"
	}
	if mode != "none" {
		emit(stdout, map[string]any{"ok": false, "error": "run: only --mode none (Allure generate is not on the wall)"})
		return ExitUsage
	}
	n, err := atoiDefault(flags["parallel"], 1)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "run: --parallel int"})
		return ExitUsage
	}
	urls, err := splitCDP(flags["cdp"])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	wait := time.Duration(60+20*(n-1)) * time.Second
	if wait < 60*time.Second {
		wait = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	var leases []*liveutil.CDPLease
	defer func() {
		for _, l := range leases {
			l.Release()
		}
	}()
	if len(urls) == 0 {
		leases, err = liveutil.ResolveCDPs(ctx, n)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": err.Error()})
			return ExitUsage
		}
		urls = make([]string, len(leases))
		for i, l := range leases {
			urls[i] = l.URL
		}
	} else {
		urls, err = expandCDP(urls, n)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": err.Error()})
			return ExitUsage
		}
	}
	c, err := crystal.Load(rest[0])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	sessions := make([]*cdp.Client, 0, len(urls))
	callers := make([]cdp.Caller, 0, len(urls))
	defer func() {
		for _, s := range sessions {
			_ = s.Close()
		}
	}()
	for _, u := range urls {
		sess, err := cdp.Dial(ctx, u)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "id": c.ID, "error": err.Error()})
			return ExitFail
		}
		sessions = append(sessions, sess)
		callers = append(callers, sess)
	}
	res := cdp.RunParallel(ctx, callers, c, cdp.Options{BaseURL: flags["base-url"]})
	res.Mode = mode
	emit(stdout, res)
	if !res.OK {
		return ExitFail
	}
	return ExitOK
}

func splitCDP(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("run: empty --cdp")
		}
		out = append(out, p)
	}
	return out, nil
}

func expandCDP(urls []string, n int) ([]string, error) {
	if n < 1 {
		return nil, fmt.Errorf("run: --parallel must be >= 1")
	}
	if n > 32 {
		return nil, fmt.Errorf("run: --parallel max 32")
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("run: --cdp URL required")
	}
	if len(urls) == n {
		return urls, nil
	}
	if len(urls) == 1 && n > 1 {
		return nil, fmt.Errorf("run: --parallel %d needs %d CDP endpoints (N --cdp or N pool leases), not N tabs on one Chrome", n, n)
	}
	return nil, fmt.Errorf("run: got %d --cdp, want --parallel %d", len(urls), n)
}

func emit(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func atoiDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	return strconv.Atoi(s)
}

// parseFlags: valueFlags true = --name VALUE (or --name=VALUE). false = boolean --name.
// Boolean present sets flags[name] = "1".
func parseFlags(args []string, valueFlags map[string]bool) (map[string]string, []string, error) {
	out := map[string]string{}
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			return out, args[i+1:], nil
		}
		if !strings.HasPrefix(a, "--") {
			return out, args[i:], nil
		}
		name := strings.TrimPrefix(a, "--")
		val := ""
		if n, v, ok := strings.Cut(name, "="); ok {
			name, val = n, v
		}
		needVal, known := valueFlags[name]
		if !known {
			return nil, nil, fmt.Errorf("unknown flag --%s", name)
		}
		if needVal {
			if val == "" {
				i++
				if i >= len(args) {
					return nil, nil, fmt.Errorf("flag --%s needs a value", name)
				}
				val = args[i]
			}
			if name == "cdp" {
				if prev, ok := out[name]; ok {
					out[name] = prev + "," + val
				} else {
					out[name] = val
				}
			} else {
				out[name] = val
			}
		} else {
			if val != "" {
				return nil, nil, fmt.Errorf("flag --%s takes no value", name)
			}
			out[name] = "1"
		}
		i++
	}
	return out, nil, nil
}

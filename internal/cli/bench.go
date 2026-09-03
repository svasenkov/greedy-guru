package cli

import (
	"context"
	"io"
	"strings"
	"time"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/liveutil"
)

func cmdBench(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{
		"cdp": true, "base-url": true, "seq": true, "parallel": true, "waves": true, "repeat": true, "mode": true,
	})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	if len(rest) != 1 {
		emit(stdout, map[string]any{"ok": false, "error": "bench: crystal.json required"})
		return ExitUsage
	}
	mode := flags["mode"]
	if mode == "" {
		mode = "none"
	}
	if mode != "none" {
		emit(stdout, map[string]any{"ok": false, "error": "bench: only --mode none"})
		return ExitUsage
	}
	if strings.TrimSpace(flags["base-url"]) == "" {
		emit(stdout, map[string]any{"ok": false, "error": "bench: --base-url required"})
		return ExitUsage
	}
	seq, err := cdp.ParseIntList(flags["seq"])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	if len(seq) == 0 {
		seq = []int{1, 5, 10, 25}
	}
	parN, err := atoiDefault(flags["parallel"], 0)
	if err != nil || parN < 0 {
		emit(stdout, map[string]any{"ok": false, "error": "bench: --parallel int >= 0"})
		return ExitUsage
	}
	waves, err := cdp.ParseIntList(flags["waves"])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	repeat, err := atoiDefault(flags["repeat"], 3)
	if err != nil || repeat < 1 {
		emit(stdout, map[string]any{"ok": false, "error": "bench: --repeat int >= 1"})
		return ExitUsage
	}
	if parN == 0 && len(waves) > 0 {
		emit(stdout, map[string]any{"ok": false, "error": "bench: --waves needs --parallel N"})
		return ExitUsage
	}
	cdpN := parN
	if cdpN < 1 {
		cdpN = 1
	}
	urls, err := splitCDP(flags["cdp"])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	wait := time.Duration(120+15*sumInts(seq)*(repeat+1)+30*parN*(repeat+1)*(1+sumInts(waves))) * time.Second
	if wait < 3*time.Minute {
		wait = 3 * time.Minute
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
		leases, err = liveutil.ResolveCDPs(ctx, cdpN)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": err.Error()})
			return ExitFail
		}
		for _, l := range leases {
			urls = append(urls, l.URL)
		}
	} else if parN == 0 {
		urls = urls[:1]
	} else {
		urls, err = expandCDP(urls, parN)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": strings.Replace(err.Error(), "run:", "bench:", 1)})
			return ExitUsage
		}
	}
	callers := make([]cdp.Caller, len(urls))
	for i, u := range urls {
		sess, err := cdp.Dial(ctx, u)
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": err.Error()})
			return ExitFail
		}
		defer sess.Close()
		callers[i] = sess
	}
	cr, err := crystal.Load(rest[0])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	rep := cdp.Report(ctx, callers, cr, cdp.Options{BaseURL: flags["base-url"]}, seq, parN, waves, repeat)
	emit(stdout, rep)
	if !rep.OK {
		return ExitFail
	}
	return ExitOK
}

func sumInts(xs []int) int {
	n := 0
	for _, v := range xs {
		n += v
	}
	return n
}

package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/status"
	"greedy.guru/greedy/internal/testops"
)

func cmdObserve(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{
		"id": true, "pw-green": true, "fingerprint": true, "live-fingerprint": true,
		"was": true, "layer": true, "testcase": true,
		"eligible": false, "pw-fail": false, "spec-fix": false, "live-fail": false,
	})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	if len(rest) > 0 {
		emit(stdout, map[string]any{"ok": false, "error": "observe: extra args"})
		return ExitUsage
	}
	green, err := atoiDefault(flags["pw-green"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "observe: --pw-green int"})
		return ExitUsage
	}
	tc, err := atoiDefault(flags["testcase"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "observe: --testcase int"})
		return ExitUsage
	}
	_, eligible := flags["eligible"]
	_, pwFail := flags["pw-fail"]
	_, specFix := flags["spec-fix"]
	_, liveFail := flags["live-fail"]
	res := status.Observe(status.Event{
		ID:              flags["id"],
		Eligible:        eligible,
		PWGreen:         green,
		PWFail:          pwFail,
		SpecFix:         specFix,
		LiveFail:        liveFail,
		Fingerprint:     flags["fingerprint"],
		LiveFingerprint: flags["live-fingerprint"],
		Was:             flags["was"],
		Layer:           flags["layer"],
		TestCaseID:      tc,
	})
	emit(stdout, res)
	if !res.OK {
		return ExitFail
	}
	return ExitOK
}

func cmdDiff(args []string, stdout io.Writer) int {
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		emit(stdout, map[string]any{"ok": false, "error": "diff: <live.json> <proposed.json>"})
		return ExitUsage
	}
	a, err := crystal.Load(args[0])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	b, err := crystal.Load(args[1])
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	d := status.Diff(a, b)
	emit(stdout, map[string]any{"ok": true, "from_test_result": false, "diff": d})
	return ExitOK
}

func cmdApprove(args []string, stdout io.Writer) int {
	flags, rest, err := parseFlags(args, map[string]bool{
		"proposed": true, "live": true, "out": true, "id": true, "was": true,
		"layer": true, "testcase": true, "endpoint": true, "token": true,
		"pw-green": true, "fingerprint": true, "live-fingerprint": true,
		"eligible": false,
	})
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitUsage
	}
	proposedPath := flags["proposed"]
	if proposedPath == "" && len(rest) == 1 {
		proposedPath = rest[0]
		rest = nil
	}
	if proposedPath == "" || len(rest) > 0 {
		emit(stdout, map[string]any{"ok": false, "error": "approve: --proposed FILE --out FILE"})
		return ExitUsage
	}
	if flags["out"] == "" {
		emit(stdout, map[string]any{"ok": false, "error": "approve: --out FILE (JSON on the pool, not TMS tick only)"})
		return ExitUsage
	}
	proposed, err := crystal.Load(proposedPath)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": err.Error()})
		return ExitFail
	}
	var live *crystal.Crystal
	if flags["live"] != "" {
		live, err = crystal.Load(flags["live"])
		if err != nil {
			emit(stdout, map[string]any{"ok": false, "error": err.Error()})
			return ExitFail
		}
	}
	green, err := atoiDefault(flags["pw-green"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "approve: --pw-green int"})
		return ExitUsage
	}
	tc, err := atoiDefault(flags["testcase"], 0)
	if err != nil {
		emit(stdout, map[string]any{"ok": false, "error": "approve: --testcase int"})
		return ExitUsage
	}
	_, eligible := flags["eligible"]
	was := flags["was"]
	if was == "" {
		was = status.PendingReview
	}
	id := flags["id"]
	if id == "" {
		id = proposed.ID
	}
	fp := flags["fingerprint"]
	if fp == "" {
		fp = status.FingerprintOf(proposed)
	}
	liveFP := flags["live-fingerprint"]
	if liveFP == "" && live != nil {
		liveFP = status.FingerprintOf(live)
	}
	res := status.Approve(status.Event{
		ID:              id,
		Eligible:        eligible,
		PWGreen:         green,
		Fingerprint:     fp,
		LiveFingerprint: liveFP,
		Was:             was,
		Layer:           flags["layer"],
		TestCaseID:      tc,
	}, live, proposed)
	if !res.OK {
		emit(stdout, res)
		return ExitFail
	}
	raw, err := json.MarshalIndent(proposed, "", "  ")
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		emit(stdout, res)
		return ExitFail
	}
	if err := os.WriteFile(flags["out"], append(raw, '\n'), 0o644); err != nil {
		res.OK = false
		res.Error = err.Error()
		emit(stdout, res)
		return ExitFail
	}
	res.Wrote = flags["out"]
	endpoint := flags["endpoint"]
	if endpoint == "" {
		endpoint = os.Getenv("ALLURE_ENDPOINT")
	}
	token := flags["token"]
	if token == "" {
		token = os.Getenv("ALLURE_TOKEN")
		if token == "" {
			token = os.Getenv("ALLURE_API_TOKEN")
		}
	}
	if endpoint != "" && tc > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		applied := testops.Apply(ctx, testops.Client{
			Endpoint: endpoint,
			Token:    token,
		}, tc, res.CrystalStatus)
		if !applied.OK {
			res.OK = false
			res.Error = applied.Error
			emit(stdout, res)
			return ExitFail
		}
		res.Patch = applied.Patch
		res.WorkflowID = applied.WorkflowID
	}
	emit(stdout, res)
	return ExitOK
}

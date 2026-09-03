package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"greedy.guru/greedy/internal/crystal"
)

type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

type RunResult struct {
	OK          bool           `json:"ok"`
	ID          string         `json:"id"`
	Steps       int            `json:"steps"`
	WallMS      int64          `json:"wall_ms"`
	ResetMS     int64          `json:"reset_ms,omitempty"`
	CdpCommands int            `json:"cdp_commands"`
	RoundTrips  int            `json:"round_trips"`
	CdpMethods  map[string]int `json:"cdp_methods,omitempty"`
	StepMS      []StepTiming   `json:"step_ms,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// tally counts CDP Call()s. Today 1 command = 1 request/response; events skipped
// inside Client.Call are not round-trips. Do not pipeline here — P3b ~1.3 ms on 11 pings.
type tally struct {
	inner Caller
	n     int
	by    map[string]int
}

func (t *tally) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, err := t.inner.Call(ctx, method, params)
	t.n++
	if t.by == nil {
		t.by = map[string]int{}
	}
	t.by[method]++
	return raw, err
}

type StepTiming struct {
	I  int    `json:"i"`
	Op string `json:"op"`
	MS int64  `json:"ms"`
}

type Options struct {
	BaseURL string
}

func Run(ctx context.Context, sess Caller, c *crystal.Crystal, opt Options) (out RunResult) {
	out = RunResult{ID: c.ID, Steps: len(c.Steps)}
	t := &tally{inner: sess}
	defer func() {
		out.CdpCommands = t.n
		out.RoundTrips = t.n
		if len(t.by) > 0 {
			out.CdpMethods = t.by
		}
	}()
	if c.Kind != "cdp" {
		out.Error = "run: kind must be cdp"
		return out
	}
	sess = t
	resetStart := time.Now()
	if !skipReset() {
		if err := resetSession(ctx, sess, opt.BaseURL); err != nil {
			out.Error = fmt.Sprintf("reset: %v", err)
			out.ResetMS = time.Since(resetStart).Milliseconds()
			return out
		}
	}
	out.ResetMS = time.Since(resetStart).Milliseconds()
	start := time.Now()
	for i, step := range c.Steps {
		st := time.Now()
		err := runStep(ctx, sess, opt, c.Steps, i)
		out.StepMS = append(out.StepMS, StepTiming{I: i, Op: step.Op, MS: time.Since(st).Milliseconds()})
		if err != nil {
			out.Error = fmt.Sprintf("steps[%d] %s: %v", i, step.Op, err)
			out.WallMS = time.Since(start).Milliseconds()
			return out
		}
	}
	out.OK = true
	out.WallMS = time.Since(start).Milliseconds()
	return out
}

func skipReset() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GREEDY_RESET"))) {
	case "0", "off", "false", "no":
		return true
	default:
		return false
	}
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// resetSession drops cookies + origin storage so each greedy run is isolated on a sticky hot Chrome.
func resetSession(ctx context.Context, sess Caller, baseURL string) error {
	if _, err := sess.Call(ctx, "Network.enable", nil); err != nil {
		return err
	}
	if _, err := sess.Call(ctx, "Network.clearBrowserCookies", map[string]any{}); err != nil {
		return err
	}
	origin := originOf(baseURL)
	if origin == "" {
		return nil
	}
	_, err := sess.Call(ctx, "Storage.clearDataForOrigin", map[string]any{
		"origin":       origin,
		"storageTypes": "cookies,local_storage",
	})
	return err
}

func runStep(ctx context.Context, sess Caller, opt Options, steps []crystal.Step, i int) error {
	step := steps[i]
	switch step.Op {
	case "navigate", "park":
		abs, err := AbsURL(opt.BaseURL, step.URL)
		if err != nil {
			return err
		}
		if err := navigate(ctx, sess, abs); err != nil {
			return err
		}
		if nextOwnsReady(steps, i) {
			return waitEval(ctx, sess, timeout(step, DefaultTimeoutMS))
		}
		return waitReady(ctx, sess, timeout(step, DefaultTimeoutMS))
	case "wait":
		return awaitUntil(ctx, sess, timeout(step, DefaultTimeoutMS), func(ms int) string {
			return WaitPromiseExpr(step.Selector, ms)
		})
	case "click":
		_, err := eval(ctx, sess, ClickExpr(step.Selector))
		return err
	case "fill":
		_, err := eval(ctx, sess, FillExpr(step.Selector, step.Value))
		return err
	case "text":
		return awaitUntil(ctx, sess, timeout(step, DefaultTimeoutMS), func(ms int) string {
			return TextPromiseExpr(step.Selector, step.Value, ms)
		})
	case "eval":
		_, err := eval(ctx, sess, step.Value)
		return err
	default:
		return fmt.Errorf("unknown op")
	}
}

func nextOwnsReady(steps []crystal.Step, i int) bool {
	if i+1 >= len(steps) {
		return false
	}
	switch steps[i+1].Op {
	case "wait", "fill", "click", "text":
		return true
	default:
		return false
	}
}

func timeout(step crystal.Step, fallback int) time.Duration {
	ms := step.TimeoutMS
	if ms <= 0 {
		ms = fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func navigate(ctx context.Context, sess Caller, abs string) error {
	err := navigateOnce(ctx, sess, abs)
	if err != nil {
		err = navigateOnce(ctx, sess, abs)
	}
	return err
}

func navigateOnce(ctx context.Context, sess Caller, abs string) error {
	nctx, cancel := context.WithTimeout(ctx, time.Duration(DefaultTimeoutMS)*time.Millisecond)
	defer cancel()
	_, err := sess.Call(nctx, "Page.navigate", map[string]string{"url": abs})
	return err
}

func waitEval(ctx context.Context, sess Caller, d time.Duration) error {
	deadline := time.Now().Add(d)
	var last error
	for {
		_, err := evalRaw(ctx, sess, "true", false)
		if err == nil {
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			if last == nil {
				last = fmt.Errorf("timeout")
			}
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func waitReady(ctx context.Context, sess Caller, d time.Duration) error {
	return awaitUntil(ctx, sess, d, ReadyPromiseExpr)
}

func awaitUntil(ctx context.Context, sess Caller, d time.Duration, expr func(remainMS int) string) error {
	deadline := time.Now().Add(d)
	var last error
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			if last == nil {
				last = fmt.Errorf("timeout")
			}
			return last
		}
		remainMS := int(remain / time.Millisecond)
		if remainMS < 1 {
			remainMS = 1
		}
		nctx, cancel := context.WithTimeout(ctx, remain+200*time.Millisecond)
		v, err := eval(nctx, sess, expr(remainMS))
		cancel()
		if err == nil {
			if b, ok := v.(bool); ok && b {
				return nil
			}
			last = fmt.Errorf("not true yet")
		} else {
			last = err
			if strings.Contains(err.Error(), "timeout") {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type evalOut struct {
	Result struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	ExceptionDetails json.RawMessage `json:"exceptionDetails"`
}

func eval(ctx context.Context, sess Caller, expr string) (any, error) {
	return evalRaw(ctx, sess, expr, true)
}

func evalRaw(ctx context.Context, sess Caller, expr string, awaitPromise bool) (any, error) {
	raw, err := sess.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	})
	if err != nil {
		return nil, err
	}
	var out evalOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.ExceptionDetails) > 0 && string(out.ExceptionDetails) != "null" {
		return nil, fmt.Errorf("js: %s", string(out.ExceptionDetails))
	}
	switch out.Result.Type {
	case "boolean":
		var b bool
		if err := json.Unmarshal(out.Result.Value, &b); err != nil {
			return nil, err
		}
		return b, nil
	case "string":
		var s string
		if err := json.Unmarshal(out.Result.Value, &s); err != nil {
			return nil, err
		}
		return s, nil
	case "undefined", "":
		return nil, nil
	default:
		return out.Result.Value, nil
	}
}

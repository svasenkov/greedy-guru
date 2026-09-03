package cdp

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"greedy.guru/greedy/internal/crystal"
)

const BenchClock = "held CDP, Run wall only: no Dial, no daemon park. par/waves before seq. global warmup + first of each cell discarded. best_ms=min of --repeat. seq = same Client. par/waves = same N Clients."

type BenchCell struct {
	N      int     `json:"n"`
	Best   int64   `json:"best_ms"`
	Median int64   `json:"median_ms"`
	Max    int64   `json:"max_ms"`
	Runs   []int64 `json:"runs_ms"`
}

type BenchReport struct {
	OK     bool        `json:"ok"`
	Clock  string      `json:"clock"`
	Reset  string      `json:"reset"`
	Repeat int         `json:"repeat"`
	Seq    []BenchCell `json:"seq,omitempty"`
	Par    *BenchCell  `json:"par,omitempty"`
	Waves  []BenchCell `json:"waves,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func resetLabel() string {
	if skipReset() {
		return "off"
	}
	return "on"
}

func MedianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func MinInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, v := range xs[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func MaxInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, v := range xs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

const benchSampleRetries = 3

func retryMS(fn func() (int64, error)) (int64, error) {
	var last error
	for i := 0; i < benchSampleRetries; i++ {
		ms, err := fn()
		if err == nil {
			return ms, nil
		}
		last = err
	}
	return 0, last
}

func sampleCell(n, repeat int, once func() (int64, error)) (BenchCell, error) {
	if _, err := retryMS(once); err != nil {
		return BenchCell{}, err
	}
	runs := make([]int64, 0, repeat)
	for i := 0; i < repeat; i++ {
		ms, err := retryMS(once)
		if err != nil {
			return BenchCell{}, err
		}
		runs = append(runs, ms)
	}
	return BenchCell{
		N:      n,
		Best:   MinInt64(runs),
		Median: MedianInt64(runs),
		Max:    MaxInt64(runs),
		Runs:   runs,
	}, nil
}

func MeasureSeq(ctx context.Context, sess Caller, c *crystal.Crystal, opt Options, n int) (int64, error) {
	if n < 1 {
		return 0, fmt.Errorf("bench: seq n must be >= 1")
	}
	start := time.Now()
	for i := 0; i < n; i++ {
		res := Run(ctx, sess, c, opt)
		if !res.OK {
			return 0, fmt.Errorf("bench seq[%d/%d]: %s", i+1, n, res.Error)
		}
	}
	return time.Since(start).Milliseconds(), nil
}

func MeasurePar(ctx context.Context, sessions []Caller, c *crystal.Crystal, opt Options) (int64, error) {
	res := RunParallel(ctx, sessions, c, opt)
	if !res.OK {
		return 0, fmt.Errorf("bench par: %s", res.Error)
	}
	return res.WallMS, nil
}

func MeasureWaves(ctx context.Context, sessions []Caller, c *crystal.Crystal, opt Options, waves int) (int64, error) {
	if waves < 1 {
		return 0, fmt.Errorf("bench: waves must be >= 1")
	}
	start := time.Now()
	for w := 0; w < waves; w++ {
		res := RunParallel(ctx, sessions, c, opt)
		if !res.OK {
			return 0, fmt.Errorf("bench wave[%d/%d]: %s", w+1, waves, res.Error)
		}
	}
	return time.Since(start).Milliseconds(), nil
}

func Warmup(ctx context.Context, sessions []Caller, c *crystal.Crystal, opt Options) error {
	if len(sessions) == 0 {
		return fmt.Errorf("bench: no CDP")
	}
	run := func() error {
		if len(sessions) == 1 {
			res := Run(ctx, sessions[0], c, opt)
			if !res.OK {
				return fmt.Errorf("bench warmup: %s", res.Error)
			}
			return nil
		}
		res := RunParallel(ctx, sessions, c, opt)
		if !res.OK {
			return fmt.Errorf("bench warmup: %s", res.Error)
		}
		return nil
	}
	var last error
	for i := 0; i < benchSampleRetries; i++ {
		last = run()
		if last == nil {
			return nil
		}
	}
	return last
}

func Report(ctx context.Context, sessions []Caller, c *crystal.Crystal, opt Options, seq []int, parN int, waves []int, repeat int) BenchReport {
	out := BenchReport{Clock: BenchClock, Reset: resetLabel(), Repeat: repeat}
	if repeat < 1 {
		out.Error = "bench: --repeat must be >= 1"
		return out
	}
	if err := Warmup(ctx, sessions, c, opt); err != nil {
		out.Error = err.Error()
		return out
	}
	if parN > 0 {
		if len(sessions) < parN {
			out.Error = fmt.Sprintf("bench: --parallel %d needs %d CDP, have %d", parN, parN, len(sessions))
			return out
		}
		group := sessions[:parN]
		cell, err := sampleCell(parN, repeat, func() (int64, error) {
			return MeasurePar(ctx, group, c, opt)
		})
		if err != nil {
			out.Error = err.Error()
			return out
		}
		out.Par = &cell
		for _, w := range waves {
			wcell, err := sampleCell(w, repeat, func() (int64, error) {
				return MeasureWaves(ctx, group, c, opt, w)
			})
			if err != nil {
				out.Error = err.Error()
				return out
			}
			out.Waves = append(out.Waves, wcell)
		}
	}
	for _, n := range seq {
		cell, err := sampleCell(n, repeat, func() (int64, error) {
			return MeasureSeq(ctx, sessions[0], c, opt, n)
		})
		if err != nil {
			out.Error = err.Error()
			return out
		}
		out.Seq = append(out.Seq, cell)
	}
	out.OK = true
	return out
}

func ParseIntList(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("bench: bad int %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

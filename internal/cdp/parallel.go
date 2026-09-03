package cdp

import (
	"context"
	"sync"
	"time"

	"greedy.guru/greedy/internal/crystal"
)

type ParallelResult struct {
	OK             bool        `json:"ok"`
	ID             string      `json:"id"`
	Steps          int         `json:"steps"`
	Parallel       int         `json:"parallel"`
	Mode           string      `json:"mode"`
	Runs           int         `json:"runs"`
	OKCount        int         `json:"ok_count"`
	WallMS         int64       `json:"wall_ms"`
	CdpCommands    int         `json:"cdp_commands"`
	RoundTrips     int         `json:"round_trips"`
	AllureGenerate bool        `json:"allure_generate"`
	Error          string      `json:"error,omitempty"`
	Workers        []RunResult `json:"workers"`
}

func RunParallel(ctx context.Context, sessions []Caller, c *crystal.Crystal, opt Options) ParallelResult {
	n := len(sessions)
	out := ParallelResult{
		ID:       c.ID,
		Steps:    len(c.Steps),
		Parallel: n,
		Mode:     "none",
		Runs:     n,
	}
	if n == 0 {
		out.Error = "run: --parallel needs a live CDP"
		return out
	}
	out.Workers = make([]RunResult, n)
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(n)
	for i, sess := range sessions {
		i, sess := i, sess
		go func() {
			defer wg.Done()
			out.Workers[i] = Run(ctx, sess, c, opt)
		}()
	}
	wg.Wait()
	out.WallMS = time.Since(start).Milliseconds()
	for _, w := range out.Workers {
		out.CdpCommands += w.CdpCommands
		out.RoundTrips += w.RoundTrips
		if w.OK {
			out.OKCount++
		} else if out.Error == "" {
			out.Error = w.Error
		}
	}
	out.OK = out.OKCount == n
	return out
}

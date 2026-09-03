package cdp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
)

type delayStub struct {
	stub
	delay time.Duration
}

func (s *delayStub) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if method == "Page.navigate" && s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.stub.Call(ctx, method, params)
}

func TestRunParallelWallIsMaxNotSum(t *testing.T) {
	c := &crystal.Crystal{
		ID:      "stub",
		Kind:    "cdp",
		Version: 1,
		Steps: []crystal.Step{
			{Op: "navigate", URL: "/login"},
			{Op: "eval", Value: "true"},
		},
	}
	d := 80 * time.Millisecond
	sessions := []cdp.Caller{
		&delayStub{delay: d},
		&delayStub{delay: d},
		&delayStub{delay: d},
	}
	res := cdp.RunParallel(context.Background(), sessions, c, cdp.Options{BaseURL: "http://example.test"})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if res.Parallel != 3 || res.OKCount != 3 || res.AllureGenerate || res.Mode != "none" {
		t.Fatalf("%+v", res)
	}
	if res.WallMS > 220 {
		t.Fatalf("wall_ms=%d looks serial (3×%d)", res.WallMS, d.Milliseconds())
	}
	if res.WallMS < 60 {
		t.Fatalf("wall_ms=%d too small", res.WallMS)
	}
}

func TestRunParallelEmpty(t *testing.T) {
	c := &crystal.Crystal{ID: "x", Kind: "cdp", Version: 1, Steps: []crystal.Step{{Op: "eval", Value: "1"}}}
	res := cdp.RunParallel(context.Background(), nil, c, cdp.Options{})
	if res.OK {
		t.Fatal("empty sessions must fail")
	}
}

func TestRunParallelOneFail(t *testing.T) {
	c := &crystal.Crystal{ID: "x", Kind: "cli", Version: 1, Steps: []crystal.Step{{Op: "eval", Value: "1"}}}
	res := cdp.RunParallel(context.Background(), []cdp.Caller{&stub{}, &stub{}}, c, cdp.Options{})
	if res.OK || res.OKCount != 0 {
		t.Fatalf("%+v", res)
	}
}

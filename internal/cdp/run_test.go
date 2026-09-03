package cdp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"greedy.guru/greedy/internal/cdp"
	"greedy.guru/greedy/internal/crystal"
)

type stub struct {
	methods []string
	params  []any
}

func (s *stub) Call(_ context.Context, method string, params any) (json.RawMessage, error) {
	s.methods = append(s.methods, method)
	s.params = append(s.params, params)
	if method != "Runtime.evaluate" {
		return json.RawMessage(`{}`), nil
	}
	_ = params
	return json.RawMessage(`{"result":{"type":"boolean","value":true}}`), nil
}

func TestRunStubLoginOps(t *testing.T) {
	c := &crystal.Crystal{
		ID:      "stub",
		Kind:    "cdp",
		Version: 1,
		Steps: []crystal.Step{
			{Op: "navigate", URL: "/login"},
			{Op: "wait", Selector: "#x", TimeoutMS: 200},
			{Op: "fill", Selector: "#x", Value: "a"},
			{Op: "click", Selector: "#b"},
			{Op: "text", Selector: "#w", Value: "hi"},
			{Op: "park", URL: "/login"},
		},
	}
	s := &stub{}
	res := cdp.Run(context.Background(), s, c, cdp.Options{BaseURL: "http://example.test"})
	if !res.OK {
		t.Fatal(res.Error)
	}
	if res.Steps != 6 {
		t.Fatalf("steps %d", res.Steps)
	}
	sawNav := false
	sawEval := false
	for _, m := range s.methods {
		if m == "Page.navigate" {
			sawNav = true
		}
		if m == "Runtime.evaluate" {
			sawEval = true
		}
	}
	if !sawNav || !sawEval {
		t.Fatalf("methods %v", s.methods)
	}
	if res.CdpCommands != len(s.methods) || res.RoundTrips != len(s.methods) {
		t.Fatalf("cdp_commands=%d round_trips=%d methods=%d", res.CdpCommands, res.RoundTrips, len(s.methods))
	}
	if res.CdpCommands != res.RoundTrips {
		t.Fatalf("no batching: commands %d != round_trips %d", res.CdpCommands, res.RoundTrips)
	}
}

func TestRunClearsStorageFirst(t *testing.T) {
	c := &crystal.Crystal{
		ID: "x", Kind: "cdp", Version: 1,
		Steps: []crystal.Step{{Op: "eval", Value: "true"}},
	}
	s := &stub{}
	res := cdp.Run(context.Background(), s, c, cdp.Options{BaseURL: "https://autotests.ai/stack/"})
	if !res.OK {
		t.Fatal(res.Error)
	}
	want := []string{"Network.enable", "Network.clearBrowserCookies", "Storage.clearDataForOrigin", "Runtime.evaluate"}
	if len(s.methods) < 4 {
		t.Fatalf("methods %v", s.methods)
	}
	for i, m := range want {
		if s.methods[i] != m {
			t.Fatalf("methods[%d]=%s want %s (%v)", i, s.methods[i], m, s.methods)
		}
	}
	clear, _ := s.params[2].(map[string]any)
	if clear["storageTypes"] != "cookies,local_storage" {
		t.Fatalf("storageTypes=%v (all is the mill wall tax)", clear["storageTypes"])
	}
}

func TestRunSkipResetEnv(t *testing.T) {
	t.Setenv("GREEDY_RESET", "off")
	c := &crystal.Crystal{
		ID: "x", Kind: "cdp", Version: 1,
		Steps: []crystal.Step{{Op: "eval", Value: "true"}},
	}
	s := &stub{}
	res := cdp.Run(context.Background(), s, c, cdp.Options{BaseURL: "https://autotests.ai/stack/"})
	if !res.OK {
		t.Fatal(res.Error)
	}
	for _, m := range s.methods {
		if strings.Contains(m, "Network") || strings.Contains(m, "Storage") {
			t.Fatalf("reset ran: %v", s.methods)
		}
	}
}

func TestRunRejectsCLIKind(t *testing.T) {
	c := &crystal.Crystal{ID: "x", Kind: "cli", Version: 1, Steps: []crystal.Step{{Op: "eval", Value: "1"}}}
	res := cdp.Run(context.Background(), &stub{}, c, cdp.Options{})
	if res.OK {
		t.Fatal("cli kind must fail")
	}
	if res.CdpCommands != 0 || res.RoundTrips != 0 {
		t.Fatalf("no CDP on reject: %+v", res)
	}
}

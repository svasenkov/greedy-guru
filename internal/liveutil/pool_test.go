package liveutil

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestResolveCDPLeaseThenRelease(t *testing.T) {
	var leased, released bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pool/lease":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["protocol"] != "cdp" || body["loopback"] != true {
				t.Errorf("lease body=%v", body)
			}
			leased = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"cdpUrl": "http://127.0.0.1:16443/",
				"slot":   map[string]any{"id": "pool-hot-cdp-min-1", "protocol": "cdp"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/pool/release":
			released = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", ts.URL)
	t.Setenv("GREEDY_CDP_FALLBACK", "off")

	lease, err := ResolveCDP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !leased || lease.Source != "lease" || lease.SlotID != "pool-hot-cdp-min-1" {
		t.Fatalf("lease=%+v leased=%v", lease, leased)
	}
	if lease.URL != "http://127.0.0.1:16443/" {
		t.Fatalf("url=%q", lease.URL)
	}
	lease.Release()
	if !released {
		t.Fatal("expected release")
	}
}

func TestResolveCDPEnvBeatsPool(t *testing.T) {
	t.Setenv("GREEDY_CDP", "http://127.0.0.1:19999")
	t.Setenv("GREEDY_POOL", "off")
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	lease, err := ResolveCDP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lease.Source != "env" || lease.URL != "http://127.0.0.1:19999" {
		t.Fatalf("%+v", lease)
	}
	lease.Release() // no-op
}

func TestLeaseWantsLoopback(t *testing.T) {
	if !leaseWantsLoopback("http://127.0.0.1:9090") {
		t.Fatal("host orchestrator")
	}
	if !leaseWantsLoopback(DefaultPoolURL) {
		t.Fatal("default pool")
	}
	if leaseWantsLoopback("http://selenoid-pool:9090") {
		t.Fatal("agent on selenoid-reuse must omit loopback")
	}
}

func TestResolveCDPsTwoLeases(t *testing.T) {
	var n int
	released := map[string]bool{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pool/lease":
			n++
			id := "pool-hot-cdp-min-" + strconv.Itoa(n)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"cdpUrl": "http://127.0.0.1:1644" + strconv.Itoa(n) + "/",
				"slot":   map[string]any{"id": id, "protocol": "cdp"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/pool/release":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]string
			_ = json.Unmarshal(raw, &body)
			released[body["slotId"]] = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", ts.URL)
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	leases, err := ResolveCDPs(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 2 || n != 2 || leases[0].SlotID == leases[1].SlotID {
		t.Fatalf("leases=%+v n=%d", leases, n)
	}
	for _, l := range leases {
		l.Release()
	}
	if !released[leases[0].SlotID] || !released[leases[1].SlotID] {
		t.Fatalf("released=%v", released)
	}
}

func TestResolveCDPsPartialRelease(t *testing.T) {
	var leased, released int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pool/lease":
			leased++
			if leased == 2 {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"ok":false}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"cdpUrl": "http://127.0.0.1:16443/",
				"slot":   map[string]any{"id": "pool-hot-cdp-min-1", "protocol": "cdp"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/pool/release":
			released++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", ts.URL)
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	_, err := ResolveCDPs(context.Background(), 2)
	if err == nil {
		t.Fatal("expected lease 2 fail")
	}
	if leased != 2 || released != 1 {
		t.Fatalf("leased=%d released=%d", leased, released)
	}
}

func TestResolveCDPsEnvRejectsN(t *testing.T) {
	t.Setenv("GREEDY_CDP", "http://127.0.0.1:19999")
	t.Setenv("GREEDY_POOL", "off")
	_, err := ResolveCDPs(context.Background(), 2)
	if err == nil || !strings.Contains(err.Error(), "not N tabs") {
		t.Fatalf("%v", err)
	}
}

func TestResolveCDPsNoPoolRejectsN(t *testing.T) {
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", "off")
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	_, err := ResolveCDPs(context.Background(), 2)
	if err == nil || !strings.Contains(err.Error(), "sidecar is 1 Chrome") {
		t.Fatalf("%v", err)
	}
}

func TestResolveCDPMissingUsage(t *testing.T) {
	t.Setenv("GREEDY_CDP", "")
	t.Setenv("GREEDY_POOL", "off")
	t.Setenv("GREEDY_CDP_FALLBACK", "off")
	_, err := ResolveCDP(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

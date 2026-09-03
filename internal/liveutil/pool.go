package liveutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultPoolURL is the host orchestrator (stand :9090). Not an ensure.py CDP stand.
const DefaultPoolURL = "http://127.0.0.1:9090"

// SidecarCDP is docker-compose.hot-cdp.yml publish — fallback only, until pool leases.
const SidecarCDP = "http://127.0.0.1:16443"

var poolHTTP = &http.Client{Timeout: 3 * time.Second}

// CDPLease is a DevTools HTTP endpoint. Release() is a no-op unless this came from POST /pool/lease.
type CDPLease struct {
	URL    string
	SlotID string
	Source string // env | lease | sidecar
	pool   string
}

func (l *CDPLease) Release() {
	if l == nil || l.pool == "" || l.SlotID == "" {
		return
	}
	body, err := json.Marshal(map[string]string{"slotId": l.SlotID})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, l.pool+"/pool/release", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := poolHTTP.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func skipFlag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "off", "false", "no":
		return true
	default:
		return false
	}
}

func poolBase() (string, bool) {
	v := strings.TrimSpace(os.Getenv("GREEDY_POOL"))
	if skipFlag(v) {
		return "", false
	}
	if v == "" {
		return strings.TrimRight(DefaultPoolURL, "/"), true
	}
	return strings.TrimRight(v, "/"), true
}

func fallbackCDP() (string, bool) {
	v := strings.TrimSpace(os.Getenv("GREEDY_CDP_FALLBACK"))
	if skipFlag(v) {
		return "", false
	}
	if v == "" {
		return strings.TrimRight(SidecarCDP, "/"), true
	}
	return strings.TrimRight(v, "/"), true
}

func probeCDP(ctx context.Context, raw string) bool {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if u == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := poolHTTP.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode < 400
}

func leaseWantsLoopback(poolURL string) bool {
	u, err := url.Parse(poolURL)
	if err != nil {
		return true
	}
	switch u.Hostname() {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func leaseCDP(ctx context.Context, base string) (*CDPLease, error) {
	payload, err := json.Marshal(map[string]any{
		"protocol": "cdp",
		"browser":  "chromium",
		"loopback": leaseWantsLoopback(base),
		"owner":    "greedy",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/pool/lease", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := poolHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pool lease HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var body struct {
		OK     bool   `json:"ok"`
		CdpURL string `json:"cdpUrl"`
		Slot   struct {
			ID string `json:"id"`
		} `json:"slot"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	url := strings.TrimSpace(body.CdpURL)
	if !body.OK || url == "" {
		return nil, fmt.Errorf("pool lease: missing cdpUrl")
	}
	return &CDPLease{
		URL:    url,
		SlotID: body.Slot.ID,
		Source: "lease",
		pool:   base,
	}, nil
}

func nWayTabsError(n int) error {
	return fmt.Errorf("run: --parallel %d needs %d CDP endpoints (N --cdp or N pool leases), not N tabs on one Chrome", n, n)
}

// ResolveCDPs returns n DevTools endpoints. n>1 is N pool leases or N --cdp (caller);
// one GREEDY_CDP / sidecar is 1 Chrome and cannot be cloned into tabs.
func ResolveCDPs(ctx context.Context, n int) ([]*CDPLease, error) {
	if n < 1 {
		return nil, fmt.Errorf("run: --parallel must be >= 1")
	}
	if n == 1 {
		l, err := ResolveCDP(ctx)
		if err != nil {
			return nil, err
		}
		return []*CDPLease{l}, nil
	}
	if strings.TrimSpace(os.Getenv("GREEDY_CDP")) != "" {
		return nil, nWayTabsError(n)
	}
	base, ok := poolBase()
	if !ok {
		return nil, fmt.Errorf("run: --parallel %d needs N --cdp or pool leases (sidecar is 1 Chrome)", n)
	}
	out := make([]*CDPLease, 0, n)
	for i := 0; i < n; i++ {
		l, err := leaseCDP(ctx, base)
		if err != nil {
			for _, x := range out {
				x.Release()
			}
			return nil, fmt.Errorf("run: pool lease %d/%d: %w", i+1, n, err)
		}
		out = append(out, l)
	}
	return out, nil
}

// ResolveCDP picks a live DevTools HTTP URL: GREEDY_CDP, else POST /pool/lease {protocol:cdp}, else sidecar compose.
func ResolveCDP(ctx context.Context) (*CDPLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if u := strings.TrimSpace(os.Getenv("GREEDY_CDP")); u != "" {
		return &CDPLease{URL: u, Source: "env"}, nil
	}
	var leaseErr error
	if base, ok := poolBase(); ok {
		lease, err := leaseCDP(ctx, base)
		if err == nil {
			return lease, nil
		}
		leaseErr = err
	}
	if u, ok := fallbackCDP(); ok && probeCDP(ctx, u) {
		return &CDPLease{URL: u, Source: "sidecar"}, nil
	}
	if leaseErr != nil {
		return nil, fmt.Errorf("run: --cdp URL required (%w)", leaseErr)
	}
	return nil, fmt.Errorf("run: --cdp URL required")
}

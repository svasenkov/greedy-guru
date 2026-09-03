package cdp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPageWSFromListSkipsBrowserUI(t *testing.T) {
	body := []byte(`[
	  {"type":"browser_ui","webSocketDebuggerUrl":"ws://127.0.0.1/ui"},
	  {"type":"page","webSocketDebuggerUrl":"ws://127.0.0.1/page"}
	]`)
	s, err := pageWSFromList(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(s, "/page") {
		t.Fatalf("%s", s)
	}
}

func TestPageWSFromListNone(t *testing.T) {
	body := []byte(`[{"type":"browser_ui","webSocketDebuggerUrl":"ws://127.0.0.1/ui"}]`)
	if _, err := pageWSFromList(body); err == nil {
		t.Fatal("expected error")
	}
}

func TestRewriteWSHostMappedPort(t *testing.T) {
	got, err := rewriteWSHost("ws://0.0.0.0:9222/devtools/page/abc", "http://127.0.0.1:54321")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ws://127.0.0.1:54321/devtools/page/abc" {
		t.Fatalf("%s", got)
	}
}

func TestCanonicalBrowserHTTPKeepsLoopback(t *testing.T) {
	got, err := CanonicalBrowserHTTP(context.Background(), "http://127.0.0.1:9223/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:9223" {
		t.Fatalf("%s", got)
	}
	got, err = CanonicalBrowserHTTP(context.Background(), "http://localhost:9223")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://localhost:9223" {
		t.Fatalf("%s", got)
	}
}

func TestCanonicalBrowserHTTPResolvesDockerDNS(t *testing.T) {
	lookupIPv4 = func(ctx context.Context, host string) (net.IP, error) {
		if host != "hot-cdp-min-1" {
			t.Fatalf("host %s", host)
		}
		return net.ParseIP("172.21.0.13"), nil
	}
	t.Cleanup(func() { lookupIPv4 = lookupIPv4Default })
	got, err := CanonicalBrowserHTTP(context.Background(), "http://hot-cdp-min-1:9223/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://172.21.0.13:9223" {
		t.Fatalf("%s", got)
	}
}

func TestCallPipelineEmpty(t *testing.T) {
	var c *Client
	if err := c.CallPipeline(context.Background(), nil); err == nil {
		t.Fatal("nil client")
	}
	c = &Client{}
	if err := c.CallPipeline(context.Background(), nil); err == nil {
		t.Fatal("closed conn")
	}
}

func TestDeadlineCapsLongContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	got := deadline(ctx, 15*time.Second)
	if got.After(time.Now().Add(20 * time.Second)) {
		t.Fatalf("deadline used hour-long ctx: %s", got)
	}
}

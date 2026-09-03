package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var lookupIPv4 = lookupIPv4Default

func lookupIPv4Default(ctx context.Context, host string) (net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no A record")
	}
	return ips[0], nil
}

func chromeAcceptsHost(host string) bool {
	switch host {
	case "", "localhost":
		return true
	default:
		return net.ParseIP(host) != nil
	}
}

// CanonicalBrowserHTTP rewrites DevTools HTTP so Chrome accepts Host (IP or localhost).
// Docker DNS like hot-cdp-min-1:9223 otherwise 500s: "Host header is not an IP address or localhost".
func CanonicalBrowserHTTP(ctx context.Context, raw string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	raw = trimSlash(strings.TrimSpace(raw))
	if raw == "" {
		return "", fmt.Errorf("cdp: empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("cdp: missing host")
	}
	host := u.Hostname()
	if chromeAcceptsHost(host) {
		return u.String(), nil
	}
	ip, err := lookupIPv4(ctx, host)
	if err != nil {
		return "", fmt.Errorf("cdp: resolve %s: %w", host, err)
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	u.Host = net.JoinHostPort(ip.String(), port)
	return u.String(), nil
}

type Client struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	nextID atomic.Int64
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e rpcErr) Error() string {
	return fmt.Sprintf("cdp %d: %s", e.Code, e.Message)
}

type rpcIn struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Error  *rpcErr         `json:"error"`
	Result json.RawMessage `json:"result"`
}

func Dial(ctx context.Context, browserHTTP string) (*Client, error) {
	if browserHTTP == "" {
		return nil, fmt.Errorf("cdp: --cdp URL required")
	}
	canon, err := CanonicalBrowserHTTP(ctx, browserHTTP)
	if err != nil {
		return nil, err
	}
	wsURL, err := pageWebSocketURL(ctx, canon)
	if err != nil {
		return nil, err
	}
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := d.DialContext(ctx, wsURL, nil)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, fmt.Errorf("cdp dial %s: %w (HTTP %d)", wsURL, err, code)
	}
	c := &Client{conn: conn}
	if _, err := c.Call(ctx, "Page.enable", nil); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := c.Call(ctx, "Runtime.enable", nil); err != nil {
		c.Close()
		return nil, err
	}
	if err := pingRuntime(ctx, c); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func pingRuntime(ctx context.Context, c *Client) error {
	deadline := time.Now().Add(2 * time.Second)
	var last error
	for {
		_, err := c.Call(ctx, "Runtime.evaluate", map[string]any{
			"expression":    "true",
			"returnByValue": true,
			"awaitPromise":  false,
		})
		if err == nil {
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			return fmt.Errorf("cdp: runtime not ready: %w", last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
}

func pageWebSocketURL(ctx context.Context, browserHTTP string) (string, error) {
	base := trimSlash(browserHTTP)
	body, err := httpJSON(ctx, http.MethodPut, base+"/json/new?about:blank")
	if err != nil {
		body, err = httpJSON(ctx, http.MethodGet, base+"/json/new?about:blank")
	}
	if err == nil {
		if s, ok := pageWSFromTab(body); ok {
			return rewriteWSHost(s, base)
		}
	}
	body, err = httpJSON(ctx, http.MethodGet, base+"/json/list")
	if err != nil {
		return "", err
	}
	s, err := pageWSFromList(body)
	if err != nil {
		return "", fmt.Errorf("cdp: no page websocket at %s: %w", base, err)
	}
	return rewriteWSHost(s, base)
}

func rewriteWSHost(wsURL, browserHTTP string) (string, error) {
	if wsURL == "" || browserHTTP == "" {
		return wsURL, nil
	}
	ws, err := url.Parse(wsURL)
	if err != nil {
		return "", err
	}
	httpu, err := url.Parse(browserHTTP)
	if err != nil {
		return "", err
	}
	if httpu.Host == "" {
		return wsURL, nil
	}
	ws.Host = httpu.Host
	switch ws.Scheme {
	case "http", "ws":
		if httpu.Scheme == "https" {
			ws.Scheme = "wss"
		} else {
			ws.Scheme = "ws"
		}
	case "https", "wss":
		if httpu.Scheme == "http" {
			ws.Scheme = "ws"
		} else {
			ws.Scheme = "wss"
		}
	}
	return ws.String(), nil
}

func pageWSFromTab(body []byte) (string, bool) {
	var tab map[string]any
	if err := json.Unmarshal(body, &tab); err != nil {
		return "", false
	}
	if typ, _ := tab["type"].(string); typ != "" && typ != "page" {
		return "", false
	}
	s, _ := tab["webSocketDebuggerUrl"].(string)
	return s, s != ""
}

func pageWSFromList(body []byte) (string, error) {
	var tabs []map[string]any
	if err := json.Unmarshal(body, &tabs); err != nil {
		return "", fmt.Errorf("/json/list: %w", err)
	}
	for _, tab := range tabs {
		if typ, _ := tab["type"].(string); typ != "page" {
			continue
		}
		if s, _ := tab["webSocketDebuggerUrl"].(string); s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("no type=page target")
}

func httpJSON(ctx context.Context, method, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdp %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("cdp %s: HTTP %d %s", rawURL, resp.StatusCode, bytesPreview(b))
	}
	return b, nil
}

func bytesPreview(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID.Add(1)
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := c.conn.SetWriteDeadline(deadline(ctx, 10*time.Second)); err != nil {
		return nil, err
	}
	if err := c.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("cdp write %s: %w", method, err)
	}
	for {
		if err := c.conn.SetReadDeadline(deadline(ctx, 15*time.Second)); err != nil {
			return nil, err
		}
		var in rpcIn
		if err := c.conn.ReadJSON(&in); err != nil {
			return nil, fmt.Errorf("cdp read %s: %w", method, err)
		}
		if in.ID != id {
			continue
		}
		if in.Error != nil {
			return nil, *in.Error
		}
		if in.Result == nil {
			return json.RawMessage("{}"), nil
		}
		return in.Result, nil
	}
}

// CallSpec is one CDP method for CallPipeline. Mill Run still uses Call 1:1.
type CallSpec struct {
	Method string
	Params any
}

// CallPipeline writes every request, then reads replies. Dependent login steps
// cannot use this; it is only for measuring RTT vs sequential Call.
func (c *Client) CallPipeline(ctx context.Context, specs []CallSpec) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("cdp: closed")
	}
	if len(specs) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := make(map[int64]string, len(specs))
	for _, s := range specs {
		id := c.nextID.Add(1)
		pending[id] = s.Method
		msg := map[string]any{"id": id, "method": s.Method}
		if s.Params != nil {
			msg["params"] = s.Params
		}
		if err := c.conn.SetWriteDeadline(deadline(ctx, 10*time.Second)); err != nil {
			return err
		}
		if err := c.conn.WriteJSON(msg); err != nil {
			return fmt.Errorf("cdp write %s: %w", s.Method, err)
		}
	}
	for len(pending) > 0 {
		if err := c.conn.SetReadDeadline(deadline(ctx, 15*time.Second)); err != nil {
			return err
		}
		var in rpcIn
		if err := c.conn.ReadJSON(&in); err != nil {
			return fmt.Errorf("cdp pipeline read: %w", err)
		}
		method, ok := pending[in.ID]
		if !ok {
			continue
		}
		delete(pending, in.ID)
		if in.Error != nil {
			return fmt.Errorf("%s: %v", method, in.Error)
		}
	}
	return nil
}

func deadline(ctx context.Context, d time.Duration) time.Time {
	t := time.Now().Add(d)
	if ctx != nil {
		if ct, ok := ctx.Deadline(); ok && ct.Before(t) {
			return ct
		}
	}
	return t
}

func AbsURL(base, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("cdp: empty url")
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	if u.IsAbs() {
		return u.String(), nil
	}
	if base == "" {
		return "", fmt.Errorf("cdp: --base-url required for relative %s", ref)
	}
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(u).String(), nil
}

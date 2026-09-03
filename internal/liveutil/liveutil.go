package liveutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Playwright chromium-min — same browser as TS PW / hot-pw-min.
// WebDriver chrome-min is later, if we crystallize Selenium.
const defaultPwMinImage = "qaguru/playwright-chromium:1.61.1-min"

// Host Chrome boot: /json/version (not IR step budget).
const hostDebugWait = 10 * time.Second

// Docker pw-min: image start + port map, not IR.
const pwMinDebugWait = 20 * time.Second

var (
	appHostMu sync.Mutex
	appHost   = "127.0.0.1"
)

func chromeLockDir() string {
	return filepath.Join(os.TempDir(), "greedy-guru-chrome.lock.d")
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func staleChromeLock(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		return true
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return true
	}
	return !processAlive(pid)
}

func chromeLock() func() {
	dir := chromeLockDir()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := os.Mkdir(dir, 0o700); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
			return func() { _ = os.RemoveAll(dir) }
		}
		if staleChromeLock(dir) || time.Now().After(deadline) {
			_ = os.RemoveAll(dir)
			continue
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func PwMinImage() string {
	if v := os.Getenv("GREEDY_PW_MIN_IMAGE"); v != "" {
		return v
	}
	return defaultPwMinImage
}

func setAppHost(h string) {
	appHostMu.Lock()
	appHost = h
	appHostMu.Unlock()
}

func currentAppHost() string {
	appHostMu.Lock()
	defer appHostMu.Unlock()
	return appHost
}

func rewriteAppURL(raw, host string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return raw
	}
	u.Host = net.JoinHostPort(host, port)
	return u.String()
}

// AppURL is the fixture origin the browser can actually fetch.
// PW min runs in Docker, so 127.0.0.1 is the container, not the httptest.
func AppURL(srv *httptest.Server) string {
	return rewriteAppURL(srv.URL, currentAppHost())
}

func ChromeBin() string {
	if b := strings.TrimSpace(os.Getenv("CHROME_BIN")); b != "" {
		return b
	}
	if runtime.GOOS == "darwin" {
		p := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func ServeApp(t testing.TB, dir string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, "login.html"))
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, "register.html"))
	})
	mux.HandleFunc("/home", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(dir, "home.html"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "home.html"))
	})
	srv := httptest.NewUnstartedServer(mux)
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func chromeFlags(userDataDir string, debugPort int, debugAddr string) []string {
	return []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-component-extensions-with-background-pages",
		"--disable-background-networking",
		"--disable-features=LocalNetworkAccessChecks,LocalNetworkAccessChecksWebRTC,PrivateNetworkAccessPermissionPrompt,BlockInsecurePrivateNetworkRequests",
		"--remote-allow-origins=*",
		"--user-data-dir=" + userDataDir,
		"--remote-debugging-port=" + strconv.Itoa(debugPort),
		"--remote-debugging-address=" + debugAddr,
	}
}

func StartChrome(t testing.TB, ctx context.Context) string {
	t.Helper()
	unlock := chromeLock()
	defer unlock()
	if ChromeBin() != "" {
		setAppHost("127.0.0.1")
		return startHostChrome(t, ctx)
	}
	setAppHost("host.docker.internal")
	return startPwMin(t, ctx)
}

func startHostChrome(t testing.TB, ctx context.Context) string {
	t.Helper()
	bin := ChromeBin()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	dir := t.TempDir()
	cmd := exec.CommandContext(ctx, bin, append(chromeFlags(dir, port, "127.0.0.1"), "about:blank")...)
	cmd.Env = append(os.Environ(), "HOME="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	waitCtx, cancel := context.WithTimeout(context.Background(), hostDebugWait)
	defer cancel()
	waitDebug(t, waitCtx, url, hostDebugWait, "")
	return url
}

func startPwMin(t testing.TB, _ context.Context) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH; need playwright-chromium min")
	}
	image := PwMinImage()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("need %s (docker pull)", image)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hostPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	proxy := filepath.Join(filepath.Dir(thisFile), "cdpproxy.pl")
	name := fmt.Sprintf("greedy-guru-cdp-%d-%d", os.Getpid(), hostPort)
	chromeCmd := bashQuote(append(chromeFlags("/tmp/greedy-cdp", 9222, "127.0.0.1"), "about:blank"))
	// Image still has node+ffmpeg+headless_shell for Playwright WS; mill execs chromium + CDP only.
	script := `set -e
perl /tmp/cdpproxy.pl &
bin=""
for c in /ms-playwright/chromium-*/chrome-linux64/chrome /ms-playwright/chromium-*/chrome-linux/chrome; do
  if [ -x "$c" ]; then bin=$c; break; fi
done
if [ -z "$bin" ]; then echo "no playwright chromium in image" >&2; exit 1; fi
exec "$bin" ` + chromeCmd
	args := []string{
		"run", "-d", "--rm",
		"--name", name,
		"--init",
		"--shm-size", "256m",
		"--add-host", "host.docker.internal:host-gateway",
		"-p", fmt.Sprintf("127.0.0.1:%d:9223", hostPort),
		"-v", proxy + ":/tmp/cdpproxy.pl:ro",
		"--entrypoint", "/bin/bash",
		image,
		"-lc", script,
	}
	runCtx, runCancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer runCancel()
	out, err := exec.CommandContext(runCtx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run pw-min: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	url := "http://127.0.0.1:" + strconv.Itoa(hostPort)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), pwMinDebugWait)
	defer waitCancel()
	waitDebug(t, waitCtx, url, pwMinDebugWait, name)
	return url
}

func bashQuote(ss []string) string {
	var b strings.Builder
	for i, s := range ss {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Quote(s))
	}
	return b.String()
}

func waitDebug(t testing.TB, ctx context.Context, debugURL string, d time.Duration, dumpName string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, debugURL+"/json/version", nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 400 {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			extra := ""
			if dumpName != "" {
				b, _ := exec.Command("docker", "logs", "--tail", "40", dumpName).CombinedOutput()
				extra = "\n" + string(b)
			}
			t.Fatalf("chrome debug port not ready on %s%s", debugURL, extra)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("chrome debug wait: %v", ctx.Err())
		case <-time.After(80 * time.Millisecond):
		}
	}
}

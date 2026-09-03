package liveutil

import (
	"os"
	"runtime"
	"testing"
)

func TestChromeBinEnvWins(t *testing.T) {
	t.Setenv("CHROME_BIN", "/tmp/greedy-fake-chrome")
	if got := ChromeBin(); got != "/tmp/greedy-fake-chrome" {
		t.Fatalf("%q", got)
	}
}

func TestChromeBinDarwinDefault(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin")
	}
	t.Setenv("CHROME_BIN", "")
	got := ChromeBin()
	if got == "" {
		t.Skip("Google Chrome.app not installed")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal(err)
	}
}

func TestRewriteAppURLDockerHost(t *testing.T) {
	got := rewriteAppURL("http://0.0.0.0:3456/", "host.docker.internal")
	if got != "http://host.docker.internal:3456/" {
		t.Fatalf("%s", got)
	}
}

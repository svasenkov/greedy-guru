package liveutil

import "testing"

func TestRewriteAppURLDockerHost(t *testing.T) {
	got := rewriteAppURL("http://0.0.0.0:3456/", "host.docker.internal")
	if got != "http://host.docker.internal:3456/" {
		t.Fatalf("%s", got)
	}
}

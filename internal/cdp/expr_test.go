package cdp_test

import (
	"strings"
	"testing"

	"greedy.guru/greedy/internal/cdp"
)

func TestFillExprUsesNativeSetter(t *testing.T) {
	s := cdp.FillExpr("[data-testid=login-username]", "user1")
	if !strings.Contains(s, "getOwnPropertyDescriptor") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "user1") {
		t.Fatal(s)
	}
}

func TestWaitPromiseExprUsesObserver(t *testing.T) {
	s := cdp.WaitPromiseExpr("[data-testid=login-input]", 3000)
	for _, n := range []string{"MutationObserver", "requestAnimationFrame", "login-input", "3000"} {
		if !strings.Contains(s, n) {
			t.Fatalf("missing %q in %s", n, s)
		}
	}
}

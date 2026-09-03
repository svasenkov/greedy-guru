package trace

import "testing"

func TestCSSTestIDPlaywright161(t *testing.T) {
	got, err := CSSTestID(`internal:testid=[data-testid="login-input"s]`, "data-testid")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[data-testid=login-input]" {
		t.Fatalf("got %q", got)
	}
}

func TestCSSTestIDChainNth(t *testing.T) {
	got, err := CSSTestID(`internal:testid=[data-testid="submit-button"s] >> nth=0`, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[data-testid=submit-button]" {
		t.Fatalf("got %q", got)
	}
}

func TestCSSTestIDRejectRole(t *testing.T) {
	_, err := CSSTestID("internal:role=button", "data-testid")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestCSSTestIDQuotedSpace(t *testing.T) {
	got, err := CSSTestID(`internal:testid=[data-testid="welcome message"s]`, "data-testid")
	if err != nil {
		t.Fatal(err)
	}
	if got != `[data-testid="welcome message"]` {
		t.Fatalf("got %q", got)
	}
}

func TestCSSTestIDAlreadyCSS(t *testing.T) {
	got, err := CSSTestID(`[data-testid=login-input]`, "data-testid")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[data-testid=login-input]" {
		t.Fatalf("got %q", got)
	}
}

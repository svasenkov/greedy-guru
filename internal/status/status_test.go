package status_test

import (
	"encoding/json"
	"strings"
	"testing"

	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/status"
)

func TestThreePWGreenPendingReview(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "register-user1", Eligible: true, PWGreen: 3,
	})
	if !r.OK || r.CrystalStatus != status.PendingReview || r.AutoLive || r.FromTestResult {
		t.Fatalf("%+v", r)
	}
}

func TestObserveAlreadyLiveNoPatch(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "logout", Eligible: true, PWGreen: 3, Was: status.Live,
		Fingerprint: "abc", LiveFingerprint: "abc",
	})
	if !r.OK || r.CrystalStatus != status.Live || r.Patch != nil || r.AutoLive {
		t.Fatalf("%+v", r)
	}
	if !contains(r.Reasons, "no PATCH") {
		t.Fatalf("reasons %v", r.Reasons)
	}
}

func TestObserveAlreadyLiveNoFingerprintStillNoPatch(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "home", Eligible: true, PWGreen: 3, Was: status.Live,
	})
	if !r.OK || r.CrystalStatus != status.Live || r.Patch != nil {
		t.Fatalf("%+v", r)
	}
}

func TestLoginAllowlistAutoLive(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "login", Eligible: true, PWGreen: 3, Fingerprint: "abc",
	})
	if !r.OK || r.CrystalStatus != status.Live || !r.AutoLive || !r.Allowlisted {
		t.Fatalf("%+v", r)
	}
}

func TestLoginNewFingerprintReview(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "login", Eligible: true, PWGreen: 3,
		Fingerprint: "new", LiveFingerprint: "old", Was: status.Live,
	})
	if r.CrystalStatus != status.PendingReview || r.AutoLive {
		t.Fatalf("%+v", r)
	}
	if !contains(r.Reasons, "fingerprint") {
		t.Fatalf("reasons %v", r.Reasons)
	}
}

func TestSpecFixResetsLive(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "login", Eligible: true, SpecFix: true, Was: status.Live,
	})
	if r.CrystalStatus != status.PendingReview {
		t.Fatalf("%+v", r)
	}
}

func TestLiveFailResets(t *testing.T) {
	r := status.Observe(status.Event{
		ID: "login", Eligible: true, LiveFail: true, Was: status.Live, PWGreen: 3,
	})
	if r.CrystalStatus != status.PendingReview {
		t.Fatalf("%+v", r)
	}
}

func TestNotEligible(t *testing.T) {
	r := status.Observe(status.Event{ID: "login", PWGreen: 3})
	if r.CrystalStatus != status.None {
		t.Fatalf("%+v", r)
	}
}

func TestManualLayerRefused(t *testing.T) {
	r := status.Observe(status.Event{ID: "login", Eligible: true, Layer: "manual", PWGreen: 3})
	if r.OK || !strings.Contains(r.Error, "manual") {
		t.Fatalf("%+v", r)
	}
}

func TestApproveWritesLive(t *testing.T) {
	live := sample("login", "old")
	proposed := sample("login", "new")
	proposed.Steps[0].URL = "login?v=2"
	r := status.Approve(status.Event{
		ID: "login", Eligible: true, PWGreen: 3, Was: status.PendingReview, TestCaseID: 42,
	}, live, proposed)
	if !r.OK || r.CrystalStatus != status.Live || r.FromTestResult || r.Patch == nil {
		t.Fatalf("%+v", r)
	}
	if r.Diff == nil || !r.Diff.Changed {
		t.Fatal("want IR diff")
	}
	if r.Patch.Method != "PATCH" || r.Patch.Path != "/api/rs/testcase/42" {
		t.Fatalf("%+v", r.Patch)
	}
	raw, _ := json.Marshal(r.Patch.Body)
	if strings.Contains(string(raw), "from_test_result") {
		t.Fatal(string(raw))
	}
	if !strings.Contains(string(raw), status.FieldName) {
		t.Fatal(string(raw))
	}
}

func TestApproveNeedsThreeGreen(t *testing.T) {
	r := status.Approve(status.Event{
		Eligible: true, PWGreen: 2, Was: status.PendingReview,
	}, nil, sample("x", "f"))
	if r.OK {
		t.Fatal(r)
	}
}

func TestApproveRejectsManual(t *testing.T) {
	r := status.Approve(status.Event{
		Eligible: true, PWGreen: 3, Was: status.PendingReview, Layer: "manual",
	}, nil, sample("x", "f"))
	if r.OK {
		t.Fatal(r)
	}
}

func TestAllowlistIDs(t *testing.T) {
	if !status.Allowlisted("login") || !status.Allowlisted("login-wrong-password") {
		t.Fatal("login")
	}
	if status.Allowlisted("logout") || status.Allowlisted("example") || status.Allowlisted("loginpage") || status.Allowlisted("register-user1") || status.Allowlisted("register-password-mismatch") || status.Allowlisted("header-login") || status.Allowlisted("header-nav-to-login") {
		t.Fatal("false positive")
	}
}

func sample(id, fp string) *crystal.Crystal {
	return &crystal.Crystal{
		ID: id, Kind: "cdp", Version: 1,
		Source: crystal.Source{Fingerprint: fp, Lang: "typescript-playwright"},
		Steps:  []crystal.Step{{Op: "navigate", URL: "login"}},
	}
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

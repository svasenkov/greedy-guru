package crystallize_test

import (
	"testing"

	"greedy.guru/greedy/internal/crystallize"
)

func TestEligiblePendingReview(t *testing.T) {
	r := crystallize.Check(3, 2, 0, false, "login user1 smoke")
	if !r.OK || !r.Eligible || r.Codegen || r.Promote != "pending_review" {
		t.Fatalf("%+v", r)
	}
}

func TestRejectLowHits(t *testing.T) {
	r := crystallize.Check(1, 2, 0, false, "login user1 smoke")
	if r.OK || r.Eligible {
		t.Fatalf("want reject: %+v", r)
	}
}

func TestRejectFaker(t *testing.T) {
	r := crystallize.Check(5, 2, 0, false, "register with faker username")
	if r.OK {
		t.Fatal("faker pattern must fail")
	}
}

func TestRejectBlocklist(t *testing.T) {
	r := crystallize.Check(5, 2, 0, false, "refactor the login page")
	if r.OK {
		t.Fatal("blocklist must fail")
	}
}

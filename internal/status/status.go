package status

import (
	"fmt"
	"strings"

	"greedy.guru/greedy/internal/crystal"
	"greedy.guru/greedy/internal/trace"
)

const (
	None          = "none"
	PendingReview = "pending_review"
	Live          = "live"

	FieldName      = "crystal_status"
	EligibilityTag = "crystal"
	MinPWGreen     = 3
	ManualLayer    = "manual"

	WorkflowName     = "greedy.guru crystals"
	StatusActiveID   = -3
	StatusOutdatedID = -4
)

type Event struct {
	ID              string
	Eligible        bool
	PWGreen         int
	PWFail          bool
	SpecFix         bool
	LiveFail        bool
	Fingerprint     string
	LiveFingerprint string
	Was             string
	Layer           string
	TestCaseID      int
}

type Patch struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body"`
}

type Result struct {
	OK             bool        `json:"ok"`
	CrystalStatus  string      `json:"crystal_status"`
	Prev           string      `json:"prev,omitempty"`
	AutoLive       bool        `json:"auto_live,omitempty"`
	Allowlisted    bool        `json:"allowlisted"`
	FromTestResult bool        `json:"from_test_result"`
	Layer          string      `json:"layer,omitempty"`
	Reasons        []string    `json:"reasons"`
	Patch          *Patch      `json:"patch,omitempty"`
	Diff           *DiffResult `json:"diff,omitempty"`
	Wrote          string      `json:"wrote,omitempty"`
	Error          string      `json:"error,omitempty"`
	WorkflowID     int         `json:"workflow_id,omitempty"`
}

func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return None
	}
	return s
}

func Allowlisted(id string) bool {
	return id == "login" || strings.HasPrefix(id, "login-")
}

func Observe(e Event) Result {
	r := base(e)
	if err := refuseManual(e.Layer); err != nil {
		return fail(r, err.Error())
	}
	if !e.Eligible {
		r.CrystalStatus = None
		r.Reasons = append(r.Reasons, "eligibility tag "+EligibilityTag+" missing")
		r.Patch = PatchSpec(e.TestCaseID, None)
		return r
	}
	fpChanged := e.Fingerprint != "" && e.LiveFingerprint != "" && e.Fingerprint != e.LiveFingerprint
	wasLive := Normalize(e.Was) == Live
	if e.SpecFix || e.LiveFail || e.PWFail || (wasLive && fpChanged) {
		r.CrystalStatus = PendingReview
		if e.SpecFix {
			r.Reasons = append(r.Reasons, "spec fix resets Live")
		}
		if fpChanged {
			r.Reasons = append(r.Reasons, "fingerprint changed")
		}
		if e.LiveFail {
			r.Reasons = append(r.Reasons, "Live run failed")
		}
		if e.PWFail {
			r.Reasons = append(r.Reasons, "Playwright launch failed")
		}
		r.Patch = PatchSpec(e.TestCaseID, PendingReview)
		return r
	}
	if e.PWGreen < MinPWGreen {
		r.CrystalStatus = Normalize(e.Was)
		if r.CrystalStatus == None {
			r.Reasons = append(r.Reasons, "need 3 PW green")
		}
		return r
	}
	fpOK := e.LiveFingerprint == "" || e.LiveFingerprint == e.Fingerprint
	if wasLive && !fpChanged {
		r.CrystalStatus = Live
		r.Reasons = append(r.Reasons, "already live — no PATCH")
		return r
	}
	if r.Allowlisted && fpOK {
		r.CrystalStatus = Live
		r.AutoLive = true
		r.Reasons = append(r.Reasons, "allowlist login auto-Live")
		r.Patch = PatchSpec(e.TestCaseID, Live)
		return r
	}
	r.CrystalStatus = PendingReview
	if r.Allowlisted && !fpOK {
		r.Reasons = append(r.Reasons, "login fingerprint changed — review")
	} else {
		r.Reasons = append(r.Reasons, "3 PW green → pending_review")
	}
	r.Patch = PatchSpec(e.TestCaseID, PendingReview)
	return r
}

func Approve(e Event, live, proposed *crystal.Crystal) Result {
	r := base(e)
	if proposed == nil {
		return fail(r, "approve: proposed crystal required")
	}
	if err := refuseManual(e.Layer); err != nil {
		return fail(r, err.Error())
	}
	if !e.Eligible {
		return fail(r, "approve: eligibility tag "+EligibilityTag+" required")
	}
	if e.PWGreen < MinPWGreen {
		return fail(r, "approve: need 3 PW green")
	}
	if e.PWFail || e.LiveFail || e.SpecFix {
		return fail(r, "approve: blocked until review (fail/fix)")
	}
	if err := proposed.Validate(); err != nil {
		return fail(r, err.Error())
	}
	id := e.ID
	if id == "" {
		id = proposed.ID
	}
	r.Allowlisted = Allowlisted(id)
	was := Normalize(e.Was)
	if was != PendingReview {
		return fail(r, "approve: want pending_review, got "+was)
	}
	d := Diff(live, proposed)
	r.Diff = &d
	r.CrystalStatus = Live
	r.Reasons = append(r.Reasons, "approve: IR diff + PATCH + JSON file")
	r.Patch = PatchSpec(e.TestCaseID, Live)
	return r
}

func PatchSpec(testCaseID int, crystalStatus string) *Patch {
	path := "/api/rs/testcase/{id}"
	if testCaseID > 0 {
		path = fmt.Sprintf("/api/rs/testcase/%d", testCaseID)
	}
	return &Patch{
		Method: "PATCH",
		Path:   path,
		Body:   PatchBody(crystalStatus),
	}
}

func PatchBody(crystalStatus string) map[string]any {
	return map[string]any{
		"customFields": []any{
			map[string]any{
				"name":        crystalStatus,
				"customField": map[string]any{"name": FieldName},
			},
		},
	}
}

func FingerprintOf(c *crystal.Crystal) string {
	if c == nil {
		return ""
	}
	if c.Source.Fingerprint != "" {
		return c.Source.Fingerprint
	}
	return trace.Fingerprint(c.Steps)
}

func refuseManual(layer string) error {
	if strings.EqualFold(strings.TrimSpace(layer), ManualLayer) {
		return fmt.Errorf("crystal is not @Layer(%q)", ManualLayer)
	}
	return nil
}

func base(e Event) Result {
	return Result{
		OK:             true,
		Prev:           Normalize(e.Was),
		Allowlisted:    Allowlisted(e.ID),
		FromTestResult: false,
		Layer:          e.Layer,
		Reasons:        []string{},
	}
}

func fail(r Result, msg string) Result {
	r.OK = false
	r.Error = msg
	r.Reasons = append(r.Reasons, msg)
	return r
}

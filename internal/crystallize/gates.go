package crystallize

import (
	"strings"
	"unicode"

	"greedy.guru/greedy/internal/crystal"
)

const MinHits = 3
const MinSpan = 2

var blockExact = map[string]struct{}{
	"fix": {}, "update": {}, "check": {}, "audit": {}, "add": {}, "make": {},
}

var blockPrefix = []string{"refactor", "wire", "redesign", "architecture"}

type Gates struct {
	Hits      int    `json:"hits"`
	Days      int    `json:"days"`
	Sessions  int    `json:"sessions"`
	Faker     bool   `json:"faker"`
	PatternOK bool   `json:"pattern_ok"`
	Pattern   string `json:"pattern"`
}

type Result struct {
	OK       bool             `json:"ok"`
	Eligible bool             `json:"eligible"`
	Codegen  bool             `json:"codegen"`
	Promote  string           `json:"promote,omitempty"`
	Reasons  []string         `json:"reasons"`
	Gates    Gates            `json:"gates"`
	Error    string           `json:"error,omitempty"`
	Crystal  *crystal.Crystal `json:"crystal,omitempty"`
}

func Check(hits, days, sessions int, faker bool, pattern string) Result {
	g := Gates{Hits: hits, Days: days, Sessions: sessions, Faker: faker, Pattern: pattern}
	reasons := []string{}
	if hits < MinHits {
		reasons = append(reasons, "hits < 3")
	}
	if days < MinSpan && sessions < MinSpan {
		reasons = append(reasons, "need ≥2 days or ≥2 sessions")
	}
	if faker || looksFaker(pattern) {
		reasons = append(reasons, "faker / non-deterministic data")
		g.Faker = true
	}
	patOK, why := lintPattern(pattern)
	g.PatternOK = patOK
	if !patOK {
		reasons = append(reasons, why)
	}
	r := Result{Reasons: reasons, Gates: g, Codegen: false}
	if len(reasons) > 0 {
		r.OK = false
		r.Eligible = false
		return r
	}
	r.OK = true
	r.Eligible = true
	r.Promote = "pending_review"
	return r
}

func looksFaker(pattern string) bool {
	p := strings.ToLower(pattern)
	return strings.Contains(p, "faker") || strings.Contains(p, "datafaker")
}

func lintPattern(pattern string) (bool, string) {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false, "pattern required"
	}
	if len(p) > 72 {
		return false, "pattern > 72 chars"
	}
	tokens := strings.FieldsFunc(p, func(r rune) bool {
		return unicode.IsSpace(r)
	})
	if len(tokens) < 3 {
		return false, "pattern needs ≥3 tokens"
	}
	low := strings.ToLower(p)
	if low == "implement feature" {
		return false, "blocklist: implement feature"
	}
	first := strings.ToLower(tokens[0])
	if _, ok := blockExact[first]; ok && len(tokens) == 1 {
		return false, "blocklist: lone " + first
	}
	if _, ok := blockExact[low]; ok {
		return false, "blocklist: " + low
	}
	for _, pre := range blockPrefix {
		if first == pre || strings.HasPrefix(low, pre+" ") || low == pre {
			return false, "blocklist: " + pre
		}
	}
	return true, ""
}

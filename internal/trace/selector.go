package trace

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var ident = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Playwright 1.61: internal:testid=[data-testid="id"s]  (flags after the quoted value)
var attrSel = regexp.MustCompile(`^\[([^=\]]+)=(?:"([^"]*)"([a-z]*)|'([^']*)'([a-z]*)|([^\]\s]+))\]$`)

func firstChain(raw string) string {
	part, _, _ := strings.Cut(raw, ">>")
	return strings.TrimSpace(part)
}

// CSSTestID turns a resolved Playwright selector into a CSS testid selector.
// Role / text / nth-only locators are rejected — IR v1 is testid from the trace.
func CSSTestID(raw, attr string) (string, error) {
	if attr == "" {
		attr = "data-testid"
	}
	part := firstChain(raw)
	if part == "" {
		return "", fmt.Errorf("empty selector")
	}
	low := strings.ToLower(part)
	if strings.Contains(low, "internal:role") || strings.HasPrefix(low, "internal:has-text") || strings.HasPrefix(low, "internal:text") {
		return "", fmt.Errorf("selector is not testid: %s", part)
	}
	if strings.HasPrefix(part, "internal:testid=") {
		rest := strings.TrimPrefix(part, "internal:testid=")
		id, err := parseTestIDRest(rest, attr)
		if err != nil {
			return "", err
		}
		return cssAttr(attr, id), nil
	}
	if strings.HasPrefix(part, "["+attr+"=") || strings.HasPrefix(part, "[data-testid=") {
		m := attrSel.FindStringSubmatch(part)
		if m == nil {
			return "", fmt.Errorf("selector is not testid: %s", part)
		}
		id := firstNonEmpty(m[2], m[4], m[6])
		if id == "" {
			return "", fmt.Errorf("selector is not testid: %s", part)
		}
		got := m[1]
		if got != attr && got != "data-testid" {
			return "", fmt.Errorf("selector is not testid: %s", part)
		}
		if got != "" {
			attr = got
		}
		return cssAttr(attr, id), nil
	}
	return "", fmt.Errorf("selector is not testid: %s", part)
}

func parseTestIDRest(rest, attr string) (string, error) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", fmt.Errorf("empty testid")
	}
	if strings.HasPrefix(rest, "[") {
		m := attrSel.FindStringSubmatch(rest)
		if m == nil {
			return "", fmt.Errorf("selector is not testid: %s", rest)
		}
		id := firstNonEmpty(m[2], m[4], m[6])
		if id == "" {
			return "", fmt.Errorf("empty testid")
		}
		return id, nil
	}
	if (strings.HasPrefix(rest, `"`) && strings.HasSuffix(rest, `"`)) || (strings.HasPrefix(rest, `'`) && strings.HasSuffix(rest, `'`)) {
		var s string
		if err := json.Unmarshal([]byte(rest), &s); err != nil {
			s = strings.Trim(rest, `"'`)
		}
		if s == "" {
			return "", fmt.Errorf("empty testid")
		}
		return s, nil
	}
	if ident.MatchString(rest) {
		return rest, nil
	}
	_ = attr
	return "", fmt.Errorf("selector is not testid: %s", rest)
}

func cssAttr(attr, id string) string {
	if ident.MatchString(id) {
		return "[" + attr + "=" + id + "]"
	}
	b, _ := json.Marshal(id)
	return "[" + attr + "=" + string(b) + "]"
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

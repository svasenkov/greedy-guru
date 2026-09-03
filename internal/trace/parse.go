package trace

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"greedy.guru/greedy/internal/crystal"
)

const Lang = "typescript-playwright"

type Options struct {
	ID   string
	ASID string
}

type event struct {
	Type                string          `json:"type"`
	Origin              string          `json:"origin"`
	TestIDAttributeName string          `json:"testIdAttributeName"`
	Options             contextOpts     `json:"options"`
	CallID              string          `json:"callId"`
	Class               string          `json:"class"`
	Method              string          `json:"method"`
	Title               string          `json:"title"`
	Params              json.RawMessage `json:"params"`
	Error               json.RawMessage `json:"error"`
}

type contextOpts struct {
	BaseURL string `json:"baseURL"`
}

type actParams struct {
	URL          string `json:"url"`
	Selector     string `json:"selector"`
	Value        string `json:"value"`
	Expression   string `json:"expression"`
	Expected     string `json:"expected"`
	IsNot        bool   `json:"isNot"`
	Timeout      float64
	ExpectedText []struct {
		String         string `json:"string"`
		MatchSubstring bool   `json:"matchSubstring"`
		IgnoreCase     bool   `json:"ignoreCase"`
		NormalizeSpace bool   `json:"normalizeWhiteSpace"`
	} `json:"expectedText"`
}

func (p *actParams) UnmarshalJSON(b []byte) error {
	type alias actParams
	aux := &struct {
		Timeout json.RawMessage `json:"timeout"`
		*alias
	}{alias: (*alias)(p)}
	if err := json.Unmarshal(b, aux); err != nil {
		return err
	}
	if len(aux.Timeout) > 0 && string(aux.Timeout) != "null" {
		var n float64
		var s string
		if err := json.Unmarshal(aux.Timeout, &n); err == nil {
			p.Timeout = n
		} else if err := json.Unmarshal(aux.Timeout, &s); err == nil {
			fmt.Sscanf(s, "%f", &p.Timeout)
		}
	}
	return nil
}

type action struct {
	class  string
	method string
	params actParams
	failed bool
}

// ToCrystal maps a Playwright trace.zip / .trace (successful run) to IR v1.
// Not AST of .spec.ts.
func ToCrystal(path string, opt Options) (*crystal.Crystal, error) {
	events, err := readEvents(path)
	if err != nil {
		return nil, err
	}
	attr := "data-testid"
	baseURL := ""
	for _, ev := range events {
		if ev.Type != "context-options" {
			continue
		}
		if ev.TestIDAttributeName != "" {
			attr = ev.TestIDAttributeName
		}
		if ev.Options.BaseURL != "" {
			baseURL = ev.Options.BaseURL
		}
	}
	acts, err := pairActions(events)
	if err != nil {
		return nil, err
	}
	steps, err := stepsFrom(acts, attr, baseURL)
	if err != nil {
		return nil, err
	}
	asID := opt.ASID
	if asID == "" {
		asID = testTitle(events)
	}
	if asID == "" {
		return nil, fmt.Errorf("trace has no PW test() title; pass --as-id")
	}
	id := opt.ID
	if id == "" {
		return nil, fmt.Errorf("pass --id (file slug); no default id")
	}
	c := &crystal.Crystal{
		ID:      id,
		Kind:    "cdp",
		Version: crystal.Version,
		Source: crystal.Source{
			ASID:        asID,
			Lang:        Lang,
			Fingerprint: Fingerprint(steps),
		},
		Steps: steps,
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// testTitle is the living Playwright test() string from the trace, not an invented as_id.
func testTitle(events []event) string {
	fromContext := ""
	fromPath := ""
	for _, ev := range events {
		t := strings.TrimSpace(ev.Title)
		if t == "" {
			continue
		}
		if ev.Type == "context-options" && strings.Contains(t, " › ") {
			fromContext = t
			continue
		}
		if strings.Contains(t, ".spec.ts") && strings.Contains(t, " › ") {
			fromPath = t
		}
	}
	if fromContext != "" {
		return lastTitleSegment(fromContext)
	}
	return lastTitleSegment(fromPath)
}

func lastTitleSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, " › ")
	return strings.TrimSpace(parts[len(parts)-1])
}

func Fingerprint(steps []crystal.Step) string {
	h := sha256.New()
	for _, s := range steps {
		fmt.Fprintf(h, "%s\t%s\t%s\t%s\n", s.Op, s.Selector, s.URL, s.Value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func pairActions(events []event) ([]action, error) {
	pending := map[string]event{}
	var order []string
	after := map[string]event{}
	for _, ev := range events {
		switch ev.Type {
		case "before":
			if ev.CallID == "" {
				continue
			}
			pending[ev.CallID] = ev
			order = append(order, ev.CallID)
		case "after":
			if ev.CallID != "" {
				after[ev.CallID] = ev
			}
		}
	}
	out := []action{}
	for _, id := range order {
		b := pending[id]
		a, ok := after[id]
		if !ok {
			return nil, fmt.Errorf("trace: missing after for %s", id)
		}
		failed := len(a.Error) > 0 && string(a.Error) != "null" && string(a.Error) != "{}"
		var p actParams
		if len(b.Params) > 0 && string(b.Params) != "null" {
			if err := json.Unmarshal(b.Params, &p); err != nil {
				return nil, fmt.Errorf("trace: params %s: %w", id, err)
			}
		}
		out = append(out, action{class: b.Class, method: b.Method, params: p, failed: failed})
	}
	return out, nil
}

func stepsFrom(acts []action, attr, baseURL string) ([]crystal.Step, error) {
	var steps []crystal.Step
	needWait := false
	mapped := 0
	for _, a := range acts {
		if !mappableClass(a.class) || skipMethod(a.method) {
			continue
		}
		if unsupportedMethod(a.method) {
			return nil, fmt.Errorf("trace: op %q not in IR v1", a.method)
		}
		switch a.method {
		case "goto":
			if a.failed {
				return nil, fmt.Errorf("trace: goto failed")
			}
			u := relativeURL(baseURL, a.params.URL)
			if u == "" {
				return nil, fmt.Errorf("trace: goto missing url")
			}
			steps = append(steps, crystal.Step{Op: "navigate", URL: u})
			needWait = true
			mapped++
		case "fill", "type":
			if a.failed {
				return nil, fmt.Errorf("trace: %s failed", a.method)
			}
			sel, err := CSSTestID(a.params.Selector, attr)
			if err != nil {
				return nil, err
			}
			if a.params.Value == "" {
				return nil, fmt.Errorf("trace: %s missing value", a.method)
			}
			if needWait {
				steps = append(steps, crystal.Step{Op: "wait", Selector: sel, TimeoutMS: 5000})
				needWait = false
			}
			steps = append(steps, crystal.Step{Op: "fill", Selector: sel, Value: a.params.Value})
			mapped++
		case "click":
			if a.failed {
				return nil, fmt.Errorf("trace: click failed")
			}
			sel, err := CSSTestID(a.params.Selector, attr)
			if err != nil {
				return nil, err
			}
			if needWait {
				steps = append(steps, crystal.Step{Op: "wait", Selector: sel, TimeoutMS: 5000})
				needWait = false
			}
			steps = append(steps, crystal.Step{Op: "click", Selector: sel})
			mapped++
		case "waitForSelector":
			if a.failed {
				return nil, fmt.Errorf("trace: waitForSelector failed")
			}
			sel, err := CSSTestID(a.params.Selector, attr)
			if err != nil {
				return nil, err
			}
			st := crystal.Step{Op: "wait", Selector: sel}
			if ms := timeoutMS(a.params.Timeout); ms > 0 {
				st.TimeoutMS = ms
			}
			steps = append(steps, st)
			needWait = false
			mapped++
		case "expect":
			if a.failed {
				return nil, fmt.Errorf("trace: expect failed")
			}
			if a.params.IsNot {
				return nil, fmt.Errorf("trace: negated expect not in IR v1")
			}
			expr := a.params.Expression
			sel, err := CSSTestID(a.params.Selector, attr)
			if err != nil {
				return nil, err
			}
			switch expr {
			case "to.have.text", "to.contain.text", "to.have.value":
				want := expectText(a.params)
				if want == "" {
					return nil, fmt.Errorf("trace: expect missing text")
				}
				wait := crystal.Step{Op: "wait", Selector: sel, TimeoutMS: 5000}
				if ms := timeoutMS(a.params.Timeout); ms > 0 {
					wait.TimeoutMS = ms
				}
				steps = append(steps, wait, crystal.Step{Op: "text", Selector: sel, Value: want})
				needWait = false
				mapped++
			case "to.be.visible":
				st := crystal.Step{Op: "wait", Selector: sel}
				if ms := timeoutMS(a.params.Timeout); ms > 0 {
					st.TimeoutMS = ms
				}
				steps = append(steps, st)
				needWait = false
				mapped++
			default:
				return nil, fmt.Errorf("trace: expect %q not in IR v1", expr)
			}
		default:
			// Test runner / unknown — ignore
		}
	}
	if mapped == 0 {
		return nil, fmt.Errorf("trace: no IR ops (need library Frame goto/fill/click/expect)")
	}
	return steps, nil
}

func mappableClass(c string) bool {
	switch c {
	case "Frame", "Page", "Locator":
		return true
	}
	return false
}

func skipMethod(m string) bool {
	switch m {
	case "newPage", "close", "__waitInfo__", "waitForURL", "waitForLoadState",
		"waitForTimeout", "waitForEvent", "waitForFunction", "bringToFront",
		"hook", "fixture", "pw:api", "tracingStart", "tracingStop", "tracingStartChunk",
		"tracingStopChunk", "setViewportSize", "addInitScript", "addLocatorHandler":
		return true
	}
	return false
}

func unsupportedMethod(m string) bool {
	switch m {
	case "hover", "check", "uncheck", "selectOption", "setInputFiles", "screenshot",
		"dragAndDrop", "tap", "dblclick":
		return true
	}
	return false
}

func timeoutMS(t float64) int {
	if t <= 0 {
		return 0
	}
	return int(t)
}

func expectText(p actParams) string {
	if len(p.ExpectedText) > 0 && p.ExpectedText[0].String != "" {
		return p.ExpectedText[0].String
	}
	return p.Expected
}

func relativeURL(base, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return strings.TrimPrefix(raw, "./")
	}
	root := strings.TrimRight(base, "/")
	if root != "" && (raw == root || strings.HasPrefix(raw, root+"/")) {
		rest := strings.TrimPrefix(raw, root)
		rest = strings.TrimPrefix(rest, "/")
		if rest == "" {
			return "."
		}
		return rest
	}
	i := strings.LastIndex(raw, "/")
	if i >= 0 && i+1 < len(raw) {
		return raw[i+1:]
	}
	return raw
}

func readEvents(path string) ([]event, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		var all []event
		ents, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".trace") {
				continue
			}
			ev, err := readTraceFile(filepath.Join(path, e.Name()))
			if err != nil {
				return nil, err
			}
			all = append(all, ev...)
		}
		if len(all) == 0 {
			return nil, fmt.Errorf("trace: no .trace files in %s", path)
		}
		return all, nil
	}
	if strings.HasSuffix(strings.ToLower(path), ".zip") {
		return readZip(path)
	}
	return readTraceFile(path)
}

func readZip(path string) ([]event, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("trace zip: %w", err)
	}
	defer zr.Close()
	var all []event
	found := false
	for _, f := range zr.File {
		name := f.Name
		if strings.Contains(name, "..") {
			continue
		}
		if !strings.HasSuffix(name, ".trace") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 16<<20))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		ev, err := parseNDJSON(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		all = append(all, ev...)
		found = true
	}
	if !found {
		return nil, fmt.Errorf("trace zip: no .trace entries")
	}
	return all, nil
}

func readTraceFile(path string) ([]event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseNDJSON(b)
}

func parseNDJSON(b []byte) ([]event, error) {
	var out []event
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	n := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("trace json line %d: %w", n+1, err)
		}
		out = append(out, ev)
		n++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

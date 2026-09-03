package crystal

import (
	"encoding/json"
	"fmt"
	"os"
)

const Version = 1

var ops = map[string]struct{}{
	"navigate": {},
	"wait":     {},
	"click":    {},
	"fill":     {},
	"text":     {},
	"eval":     {},
	"park":     {},
}

type Source struct {
	ASID        string `json:"as_id,omitempty"`
	Lang        string `json:"lang,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type Step struct {
	Op        string `json:"op"`
	Selector  string `json:"selector,omitempty"`
	Value     string `json:"value,omitempty"`
	URL       string `json:"url,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type Crystal struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Version int    `json:"version"`
	Source  Source `json:"source"`
	Steps   []Step `json:"steps"`
}

func Load(path string) (*Crystal, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Crystal
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Crystal) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("id: required")
	}
	if c.Kind != "cdp" && c.Kind != "cli" {
		return fmt.Errorf("kind: want cdp|cli, got %q", c.Kind)
	}
	if c.Version != Version {
		return fmt.Errorf("version: want %d, got %d", Version, c.Version)
	}
	if len(c.Steps) == 0 {
		return fmt.Errorf("steps: empty")
	}
	for i, s := range c.Steps {
		if _, ok := ops[s.Op]; !ok {
			return fmt.Errorf("steps[%d].op: unknown %q", i, s.Op)
		}
		switch s.Op {
		case "navigate", "park":
			if s.URL == "" {
				return fmt.Errorf("steps[%d]: %s needs url", i, s.Op)
			}
		case "wait", "click":
			if s.Selector == "" {
				return fmt.Errorf("steps[%d]: %s needs selector", i, s.Op)
			}
		case "fill", "text":
			if s.Selector == "" || s.Value == "" {
				return fmt.Errorf("steps[%d]: %s needs selector and value", i, s.Op)
			}
		case "eval":
			if s.Value == "" {
				return fmt.Errorf("steps[%d]: eval needs value", i)
			}
		}
	}
	return nil
}

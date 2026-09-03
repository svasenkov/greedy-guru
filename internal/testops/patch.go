package testops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"greedy.guru/greedy/internal/status"
)

const APIPrefix = "/api/rs"

type Client struct {
	Endpoint string
	Prefix   string
	Token    string
	HTTP     *http.Client
}

type ApplyResult struct {
	OK             bool          `json:"ok"`
	FromTestResult bool          `json:"from_test_result"`
	TestCaseID     int           `json:"test_case_id"`
	CrystalStatus  string        `json:"crystal_status"`
	FieldID        int           `json:"field_id,omitempty"`
	ValueID        int           `json:"value_id,omitempty"`
	Error          string        `json:"error,omitempty"`
	Patch          *status.Patch `json:"patch,omitempty"`
	WorkflowID     int           `json:"workflow_id,omitempty"`
}

func Apply(ctx context.Context, c Client, testCaseID int, crystalStatus string) ApplyResult {
	out := ApplyResult{
		FromTestResult: false,
		TestCaseID:     testCaseID,
		CrystalStatus:  crystalStatus,
		Patch:          status.PatchSpec(testCaseID, crystalStatus),
	}
	if testCaseID <= 0 {
		out.Error = "testops: testcase id required"
		return out
	}
	if crystalStatus != status.PendingReview && crystalStatus != status.Live && crystalStatus != status.None {
		out.Error = "testops: unknown crystal_status " + crystalStatus
		return out
	}
	if c.Endpoint == "" {
		out.Error = "testops: endpoint required"
		return out
	}
	if c.Prefix == "" {
		c.Prefix = APIPrefix
	}
	if c.HTTP == nil {
		c.HTTP = http.DefaultClient
	}
	tc, err := getJSON(ctx, c, fmt.Sprintf("/testcase/%d", testCaseID))
	if err != nil {
		out.Error = err.Error()
		return out
	}
	projectID := intFrom(tc["projectId"])
	fields, err := getList(ctx, c, "/cf", nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	fieldID := 0
	for _, f := range fields {
		if str(f["name"]) == status.FieldName {
			fieldID = intFrom(f["id"])
			break
		}
	}
	if fieldID == 0 {
		fields, err = getList(ctx, c, "/cf", map[string]string{"projectId": fmt.Sprintf("%d", projectID)})
		if err != nil {
			out.Error = err.Error()
			return out
		}
		for _, f := range fields {
			if str(f["name"]) == status.FieldName {
				fieldID = intFrom(f["id"])
				break
			}
		}
	}
	if fieldID == 0 {
		out.Error = "testops: custom field " + status.FieldName + " missing"
		return out
	}
	values, err := getList(ctx, c, "/cfv", map[string]string{"customFieldId": fmt.Sprintf("%d", fieldID)})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	valueID := 0
	for _, v := range values {
		if str(v["name"]) == crystalStatus {
			valueID = intFrom(v["id"])
			break
		}
	}
	if valueID == 0 && crystalStatus != status.None {
		out.Error = "testops: value " + crystalStatus + " missing"
		return out
	}
	existing, err := getList(ctx, c, fmt.Sprintf("/testcase/%d/cfv", testCaseID), nil)
	if err != nil {
		existing = nil
	}
	ids := mergeValueIDs(existing, fieldID, valueID, crystalStatus)
	var wrapped []any
	for _, id := range ids {
		wrapped = append(wrapped, map[string]any{"id": id})
	}
	body := map[string]any{"customFields": wrapped}
	raw, _ := json.Marshal(body)
	if bytes.Contains(raw, []byte("from_test_result")) {
		out.Error = "testops: from_test_result forbidden"
		return out
	}
	if err := patchJSON(ctx, c, fmt.Sprintf("/testcase/%d", testCaseID), body); err != nil {
		out.Error = err.Error()
		return out
	}
	out.OK = true
	out.FieldID = fieldID
	out.ValueID = valueID
	out.Patch.Body = body
	wfID, err := assignCrystalWorkflow(ctx, c, testCaseID)
	out.WorkflowID = wfID
	if err != nil {
		out.OK = false
		out.Error = err.Error()
	}
	return out
}

func mergeValueIDs(existing []map[string]any, fieldID, valueID int, name string) []int {
	var out []int
	for _, m := range existing {
		cf, _ := m["customField"].(map[string]any)
		if str(cf["name"]) == status.FieldName || intFrom(cf["id"]) == fieldID {
			continue
		}
		id := intFrom(m["id"])
		if id != 0 {
			out = append(out, id)
		}
	}
	if name == status.None || valueID == 0 {
		return out
	}
	return append(out, valueID)
}

func assignCrystalWorkflow(ctx context.Context, c Client, testCaseID int) (int, error) {
	wfs, err := getList(ctx, c, "/workflow", nil)
	if err != nil {
		return 0, nil
	}
	wfID := 0
	for _, wf := range wfs {
		if str(wf["name"]) == status.WorkflowName {
			wfID = intFrom(wf["id"])
			break
		}
	}
	if wfID == 0 {
		created, err := postJSON(ctx, c, "/workflow", map[string]any{
			"name": status.WorkflowName,
			"statuses": []any{
				map[string]any{"id": status.StatusActiveID},
				map[string]any{"id": status.StatusOutdatedID},
			},
		})
		if err != nil {
			return 0, fmt.Errorf("testops: create workflow: %w", err)
		}
		wfID = intFrom(created["id"])
	}
	if wfID == 0 {
		return 0, fmt.Errorf("testops: workflow %s missing", status.WorkflowName)
	}
	if err := patchJSON(ctx, c, fmt.Sprintf("/testcase/%d", testCaseID), map[string]any{
		"workflowId": wfID,
		"statusId":   status.StatusActiveID,
	}); err != nil {
		return wfID, fmt.Errorf("testops: assign workflow: %w", err)
	}
	return wfID, nil
}

func postJSON(ctx context.Context, c Client, path string, body any) (map[string]any, error) {
	b, err := do(ctx, c, http.MethodPost, path, nil, body)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func getJSON(ctx context.Context, c Client, path string) (map[string]any, error) {
	b, err := do(ctx, c, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func getList(ctx context.Context, c Client, path string, q map[string]string) ([]map[string]any, error) {
	b, err := do(ctx, c, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	switch t := payload.(type) {
	case []any:
		return asMaps(t), nil
	case map[string]any:
		if content, ok := t["content"].([]any); ok {
			return asMaps(content), nil
		}
	}
	return nil, fmt.Errorf("testops: unexpected list %s", path)
}

func patchJSON(ctx context.Context, c Client, path string, body any) error {
	_, err := do(ctx, c, http.MethodPatch, path, nil, body)
	return err
}

func do(ctx context.Context, c Client, method, path string, q map[string]string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	u := strings.TrimRight(c.Endpoint, "/") + c.Prefix + path
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	if q != nil {
		qs := req.URL.Query()
		for k, v := range q {
			qs.Set(k, v)
		}
		req.URL.RawQuery = qs.Encode()
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("testops: %s %s HTTP %d", method, path, res.StatusCode)
	}
	return b, nil
}

func asMaps(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func intFrom(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case int:
		return t
	}
	return 0
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

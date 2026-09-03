package testops_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"greedy.guru/greedy/internal/status"
	"greedy.guru/greedy/internal/testops"
)

func TestApplyPATCHNotFromTestResult(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/testcase/11/cfv"):
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{"id": 1, "name": "cm", "customField": map[string]any{"id": 4, "name": "Component"}},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/testcase/11"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 11, "projectId": 7})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cf"):
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{"id": 9, "name": status.FieldName},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cfv"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []any{
					map[string]any{"id": 21, "name": status.PendingReview},
					map[string]any{"id": 22, "name": status.Live},
				},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/testcase/11"):
			gotMethod, gotPath = r.Method, r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":11}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	res := testops.Apply(context.Background(), testops.Client{
		Endpoint: srv.URL,
		Token:    "t",
		HTTP:     srv.Client(),
	}, 11, status.PendingReview)
	if !res.OK || res.FromTestResult || res.Error != "" {
		t.Fatalf("%+v", res)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/api/rs/testcase/11") {
		t.Fatalf("path %s", gotPath)
	}
	if strings.Contains(gotBody, "from_test_result") {
		t.Fatal(gotBody)
	}
	if !strings.Contains(gotBody, `"id":21`) || !strings.Contains(gotBody, `"id":1`) {
		t.Fatal(gotBody)
	}
	if strings.Contains(gotBody, status.FieldName) {
		t.Fatalf("do not send nested customField (TestOps 400 if field not in project list): %s", gotBody)
	}
}

func TestApplyAssignsCrystalWorkflow(t *testing.T) {
	var patches []string
	var created bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/testcase/11/cfv"):
			_ = json.NewEncoder(w).Encode([]any{})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/testcase/11"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 11, "projectId": 7})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cf"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"id": 9, "name": status.FieldName}})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cfv"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []any{map[string]any{"id": 22, "name": status.Live}},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workflow"):
			_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workflow"):
			created = true
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), status.WorkflowName) || !strings.Contains(string(b), `{"id":-3}`) {
				t.Errorf("create body %s", b)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "name": status.WorkflowName})
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/testcase/11"):
			b, _ := io.ReadAll(r.Body)
			patches = append(patches, string(b))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":11}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	res := testops.Apply(context.Background(), testops.Client{
		Endpoint: srv.URL,
		Token:    "t",
		HTTP:     srv.Client(),
	}, 11, status.Live)
	if !res.OK || res.Error != "" || res.WorkflowID != 7 || !created {
		t.Fatalf("%+v created=%v", res, created)
	}
	if len(patches) != 2 {
		t.Fatalf("patches %d %v", len(patches), patches)
	}
	if !strings.Contains(patches[0], `"id":22`) {
		t.Fatalf("field patch %s", patches[0])
	}
	if !strings.Contains(patches[1], `"workflowId":7`) || !strings.Contains(patches[1], `"statusId":-3`) {
		t.Fatalf("workflow patch %s", patches[1])
	}
}

func TestApplyMissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/testcase/3") {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "projectId": 1})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	t.Cleanup(srv.Close)
	res := testops.Apply(context.Background(), testops.Client{Endpoint: srv.URL, HTTP: srv.Client()}, 3, status.Live)
	if res.OK || !strings.Contains(res.Error, status.FieldName) {
		t.Fatalf("%+v", res)
	}
}

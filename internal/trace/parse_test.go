package trace_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"greedy.guru/greedy/internal/trace"
)

func testdata(t *testing.T, elem ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	return filepath.Join(append([]string{root}, elem...)...)
}

func zipTrace(t *testing.T, tracePath string) string {
	t.Helper()
	raw, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "trace.zip")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("0-trace.trace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestToCrystalLoginZip(t *testing.T) {
	src := testdata(t, "trace", "login.trace")
	c, err := trace.ToCrystal(zipTrace(t, src), trace.Options{
		ID:   "login",
		ASID: "Пользователь может войти с валидными credentials",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "login" || c.Kind != "cdp" || c.Source.Lang != trace.Lang {
		t.Fatalf("%+v", c)
	}
	if c.Source.Fingerprint == "" || c.Source.Fingerprint == "example-not-live" {
		t.Fatal("fingerprint")
	}
	want := [][3]string{
		{"navigate", "", "login"},
		{"wait", "[data-testid=login-input]", ""},
		{"fill", "[data-testid=login-input]", "user1"},
		{"fill", "[data-testid=password-input]", "password1"},
		{"click", "[data-testid=submit-button]", ""},
		{"wait", "[data-testid=welcome-message]", ""},
		{"text", "[data-testid=welcome-message]", "Welcome, user1!"},
	}
	if len(c.Steps) != len(want) {
		t.Fatalf("steps %d: %+v", len(c.Steps), c.Steps)
	}
	for i, w := range want {
		s := c.Steps[i]
		if s.Op != w[0] || s.Selector != w[1] {
			t.Fatalf("step %d %+v want %v", i, s, w)
		}
		if w[0] == "navigate" && s.URL != w[2] {
			t.Fatalf("url %q", s.URL)
		}
		if (w[0] == "fill" || w[0] == "text") && s.Value != w[2] {
			t.Fatalf("value %q", s.Value)
		}
	}
	if c.Source.Fingerprint != trace.Fingerprint(c.Steps) {
		t.Fatal("fingerprint drift")
	}
}

func TestToCrystalRawTraceFile(t *testing.T) {
	c, err := trace.ToCrystal(testdata(t, "trace", "login.trace"), trace.Options{ID: "x", ASID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "x" {
		t.Fatal(c.ID)
	}
}

func TestToCrystalCommittedZip(t *testing.T) {
	c, err := trace.ToCrystal(testdata(t, "trace", "login.zip"), trace.Options{
		ID:   "login",
		ASID: "Пользователь может войти с валидными credentials",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Steps[0].Op != "navigate" || c.Steps[0].URL != "login" {
		t.Fatalf("%+v", c.Steps[0])
	}
}

func TestToCrystalRequiresID(t *testing.T) {
	_, err := trace.ToCrystal(testdata(t, "trace", "login.trace"), trace.Options{
		ASID: "Пользователь может войти с валидными credentials",
	})
	if err == nil {
		t.Fatal("want --id")
	}
}

func TestRejectRoleSelector(t *testing.T) {
	_, err := trace.ToCrystal(testdata(t, "trace", "role.trace"), trace.Options{ID: "x", ASID: "role"})
	if err == nil {
		t.Fatal("role selector must fail")
	}
}

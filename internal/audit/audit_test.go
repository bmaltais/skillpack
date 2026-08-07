package audit_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bmaltais/skillpack/internal/audit"
)

// captureStderr redirects os.Stderr to a pipe, calls f, then returns what was written.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	f()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestSuccess_emitsValidJSON(t *testing.T) {
	out := captureStderr(t, func() {
		audit.Success(audit.EventSkillInstall, "my-repo/tools/debugger → claude-code")
	})

	out = strings.TrimSpace(out)
	if out == "" {
		t.Fatal("expected a log line on stderr, got nothing")
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nraw: %q", err, out)
	}

	for _, field := range []string{"timestamp", "event", "detail", "outcome"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("missing required field %q in audit record", field)
		}
	}
	if got := rec["event"]; got != audit.EventSkillInstall {
		t.Errorf("event: got %q, want %q", got, audit.EventSkillInstall)
	}
	if got := rec["outcome"]; got != "success" {
		t.Errorf("outcome: got %q, want %q", got, "success")
	}
	if _, ok := rec["error"]; ok {
		t.Error("error field should be absent on success")
	}
}

func TestFailure_includesError(t *testing.T) {
	out := captureStderr(t, func() {
		audit.Failure(audit.EventSkillRemove, "my-repo/tools/debugger", os.ErrNotExist)
	})

	out = strings.TrimSpace(out)
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nraw: %q", err, out)
	}

	if got := rec["outcome"]; got != "failure" {
		t.Errorf("outcome: got %q, want %q", got, "failure")
	}
	errField, ok := rec["error"]
	if !ok || errField == "" {
		t.Error("error field should be present and non-empty on failure")
	}
}

func TestLog_timestampIsRFC3339(t *testing.T) {
	out := captureStderr(t, func() {
		audit.Success(audit.EventSkillPublish, "my-repo/tools/debugger")
	})

	out = strings.TrimSpace(out)
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nraw: %q", err, out)
	}

	ts, ok := rec["timestamp"].(string)
	if !ok || ts == "" {
		t.Fatal("timestamp field missing or not a string")
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("timestamp %q is not RFC 3339: %v", ts, err)
	}
}

func TestEventConstants(t *testing.T) {
	for _, name := range []string{
		audit.EventSkillInstall,
		audit.EventSkillRemove,
		audit.EventSkillPublish,
	} {
		if name == "" {
			t.Errorf("event constant must not be empty")
		}
		if !strings.Contains(name, ".") {
			t.Errorf("event constant %q should use dotted namespace", name)
		}
	}
}

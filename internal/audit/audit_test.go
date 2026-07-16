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

	// AU-3 required fields: date/time, event type, subject identity, object, outcome.
	for _, field := range []string{"timestamp", "event", "actor", "detail", "outcome"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("missing required AU-3 field %q in audit record", field)
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

// TestAU3_actorFieldPresent verifies the AU-3 "subject identity" requirement:
// every record must carry a non-empty actor in USER@hostname form.
func TestAU3_actorFieldPresent(t *testing.T) {
	out := captureStderr(t, func() {
		audit.Success(audit.EventSkillInstall, "my-repo/tools/debugger → claude-code")
	})

	out = strings.TrimSpace(out)
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nraw: %q", err, out)
	}

	actor, ok := rec["actor"].(string)
	if !ok || actor == "" {
		t.Fatal("actor field missing or empty — AU-3 requires subject identity in every record")
	}
	if !strings.Contains(actor, "@") {
		t.Errorf("actor %q should be in USER@hostname form", actor)
	}
}

func TestEventConstants(t *testing.T) {
	for _, name := range []string{
		audit.EventSkillInstall,
		audit.EventSkillRemove,
		audit.EventSkillPublish,
		audit.EventSkillUpdate,
		audit.EventConfigCredentialSet,
	} {
		if name == "" {
			t.Errorf("event constant must not be empty")
		}
		if !strings.Contains(name, ".") {
			t.Errorf("event constant %q should use dotted namespace", name)
		}
	}
}

// TestAU12_generationAtLifecyclePoints verifies that Log() actually writes a
// record (i.e. audit generation fires) for each defined lifecycle event name.
// This is the AU-12 "configured generation" check — if the mechanism silently
// drops records the test fails.
func TestAU12_generationAtLifecyclePoints(t *testing.T) {
	events := []string{
		audit.EventSkillInstall,
		audit.EventSkillRemove,
		audit.EventSkillPublish,
		audit.EventSkillUpdate,
		audit.EventConfigCredentialSet,
	}
	for _, evt := range events {
		evt := evt
		t.Run(evt, func(t *testing.T) {
			out := captureStderr(t, func() {
				audit.Success(evt, "test-detail")
			})
			out = strings.TrimSpace(out)
			if out == "" {
				t.Fatalf("AU-12: no audit record emitted for event %q", evt)
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(out), &rec); err != nil {
				t.Fatalf("AU-12: record for %q is not valid JSON: %v", evt, err)
			}
			if rec["event"] != evt {
				t.Errorf("AU-12: event field got %q, want %q", rec["event"], evt)
			}
		})
	}
}

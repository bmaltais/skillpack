// Package audit provides structured audit event logging for skillpack CLI operations.
// Auditable events are written as newline-delimited JSON to stderr so they can be
// captured by a log aggregator or redirected independently of normal output.
//
// Auditable events defined for this system:
//   - skill.install   – a skill was installed into an agent's skill directory
//   - skill.remove    – a skill was removed from an agent's skill directory
//   - skill.publish   – local skill edits were pushed to a remote repo
//
// Each record contains:
//   - timestamp  (RFC 3339 UTC)
//   - event      (dotted name string)
//   - detail     (human-readable target / description)
//   - outcome    ("success" | "failure")
//   - error      (error message, omitted on success)
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Event names for key skillpack lifecycle operations.
const (
	EventSkillInstall = "skill.install"
	EventSkillRemove  = "skill.remove"
	EventSkillPublish = "skill.publish"
)

// record is the JSON structure written to stderr for each audit event.
type record struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Detail    string `json:"detail"`
	Outcome   string `json:"outcome"`
	Error     string `json:"error,omitempty"`
}

// Log writes a structured audit event to stderr.
// outcome must be "success" or "failure"; errMsg is included only on failure.
// Errors writing the log line are silently discarded — audit logging must not
// interfere with normal CLI operation.
func Log(event, detail, outcome, errMsg string) {
	r := record{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Event:     event,
		Detail:    detail,
		Outcome:   outcome,
		Error:     errMsg,
	}
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s\n", b)
}

// Success logs a successful audit event.
func Success(event, detail string) {
	Log(event, detail, "success", "")
}

// Failure logs a failed audit event with an error description.
func Failure(event, detail string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	Log(event, detail, "failure", msg)
}

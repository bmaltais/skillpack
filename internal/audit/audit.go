// Package audit provides structured audit event logging for skillpack CLI operations.
// Auditable events are written as newline-delimited JSON to stderr so they can be
// captured by a log aggregator or redirected independently of normal output.
//
// Auditable events defined for this system:
//   - skill.install   – a skill was installed into an agent's skill directory
//   - skill.remove    – a skill was removed from an agent's skill directory
//   - skill.publish   – local skill edits were pushed to a remote repo
//
// Each record contains (AU-3 required fields):
//   - timestamp  (RFC 3339 UTC)
//   - event      (dotted name string — type of auditable event)
//   - actor      (USER@hostname — identity of the subject performing the action)
//   - detail     (human-readable target / object of the action)
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
// Fields satisfy ITSG-33 AU-3 required content: date/time (Timestamp),
// event type (Event), subject identity (Actor), object (Detail), and
// outcome (Outcome).
type record struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail"`
	Outcome   string `json:"outcome"`
	Error     string `json:"error,omitempty"`
}

// actor returns a best-effort "USER@hostname" string for the current process.
// Falls back gracefully if either lookup fails.
func actor() string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows fallback
	}
	if user == "" {
		user = "unknown"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return user + "@" + host
}

// Log writes a structured audit event to stderr.
// outcome must be "success" or "failure"; errMsg is included only on failure.
// Errors writing the log line are silently discarded — audit logging must not
// interfere with normal CLI operation.
func Log(event, detail, outcome, errMsg string) {
	r := record{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Event:     event,
		Actor:     actor(),
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

package repo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

// IsGHOToken reports whether token is a GitHub OAuth token (gho_ prefix).
// These tokens can expire or be revoked after inactivity, producing
// "Invalid username or token" errors that are hard to diagnose.
func IsGHOToken(token string) bool {
	return strings.HasPrefix(token, "gho_")
}

// FormatAuthHint returns an improved error message for auth failures when the
// stored credential is a gho_ token. The hint tells the user exactly how to
// refresh the token and where to put it. For non-gho_ tokens it returns an
// empty string so callers can keep the original git error.
//
// isPush controls whether we say "push" or "pull/fetch" — matches the verb the
// user saw in their sync output.
func FormatAuthHint(repoName, token string, err error, isPush bool) string {
	if !IsGHOToken(token) || err == nil {
		return ""
	}

	action := "pull"
	if isPush {
		action = "push"
	}

	// Strip the raw git transport error for cleaner output.
	msg := strings.TrimSpace(err.Error())
	// Some go-git errors embed the verb already ("authentication required: ...").
	// Normalise to a consistent pattern.
	if !strings.Contains(strings.ToLower(msg), "authentication") && !strings.Contains(strings.ToLower(msg), "authorization") {
		msg = "authentication required" + msg
	}

	return fmt.Sprintf(
		"%s failed for %q: %s\n"+
			"hint: your token appears to be an expired GitHub OAuth token\n"+
			"hint: run: gh auth token -h github.com -u <user>\n"+
			"hint: then update credentials.%s in ~/.skillpack/config.yaml",
		action, repoName, strings.TrimPrefix(msg, action+" failed for"),
		repoName,
	)
}

// IsTransportAuthError reports whether err is an authentication/authorisation
// failure from the go-git transport layer. It uses errors.Is to support
// wrapped errors.
func IsTransportAuthError(err error) bool {
	return errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed)
}
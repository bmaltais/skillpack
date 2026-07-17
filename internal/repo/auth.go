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
	if !IsGHOToken(token) || err == nil || !IsTransportAuthError(err) {
		return ""
	}

	action := "pull"
	if isPush {
		action = "push"
	}

	return fmt.Sprintf(
		"%s failed for %q: %s\n"+
			"hint: your token appears to be an expired GitHub OAuth token\n"+
			"hint: run: gh auth token -h github.com -u <user>\n"+
			"hint: then update credentials.%s in ~/.skillpack/config.yaml",
		action, repoName, strings.TrimSpace(err.Error()), repoName,
	)
}

// IsTransportAuthError reports whether err is an authentication/authorisation
// failure from the go-git transport layer. It uses errors.Is to support
// wrapped errors.
func IsTransportAuthError(err error) bool {
	return errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed)
}
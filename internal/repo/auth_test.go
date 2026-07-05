package repo

// Internal-package tests for auth helpers and Update fallback logic.
// These tests exercise code paths that are not reachable from the external
// repo_test package because they depend on unexported functions.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestIsTransportAuthError_RecognisesAuthenticationRequired(t *testing.T) {
	if !IsTransportAuthError(transport.ErrAuthenticationRequired) {
		t.Error("expected IsTransportAuthError(ErrAuthenticationRequired) == true")
	}
}

func TestIsTransportAuthError_RecognisesAuthorizationFailed(t *testing.T) {
	if !IsTransportAuthError(transport.ErrAuthorizationFailed) {
		t.Error("expected IsTransportAuthError(ErrAuthorizationFailed) == true")
	}
}

func TestIsTransportAuthError_RecognisesWrappedErrors(t *testing.T) {
	properlyWrapped := fmt.Errorf("outer: %w", transport.ErrAuthenticationRequired)
	if !IsTransportAuthError(properlyWrapped) {
		t.Error("expected IsTransportAuthError to return true for properly wrapped ErrAuthenticationRequired")
	}
}

func TestIsTransportAuthError_ReturnsFalseForOtherErrors(t *testing.T) {
	for _, err := range []error{
		transport.ErrRepositoryNotFound,
		transport.ErrEmptyRemoteRepository,
		errors.New("connection refused"),
	} {
		if IsTransportAuthError(err) {
			t.Errorf("IsTransportAuthError(%v) = true, want false", err)
		}
	}
}

func TestIsGHOToken(t *testing.T) {
	if !IsGHOToken("gho_abc123") {
		t.Error("expected IsGHOToken('gho_abc123') == true")
	}
	if !IsGHOToken("gho_") {
		t.Error("expected IsGHOToken('gho_') == true")
	}
	if IsGHOToken("ghp_abc123") {
		t.Error("expected IsGHOToken('ghp_abc123') == false (PAT, not OAuth)")
	}
	if IsGHOToken("") {
		t.Error("expected IsGHOToken('') == false")
	}
	if IsGHOToken("some-random-string") {
		t.Error("expected IsGHOToken('some-random-string') == false")
	}
}

func TestFormatAuthHint_GhoTokenPush(t *testing.T) {
	err := fmt.Errorf("authentication required: Invalid username or token")
	hint := FormatAuthHint("my-repo", "gho_abc123", err, true)
	if hint == "" {
		t.Fatal("expected non-empty hint for gho_ token push failure")
	}
	if !strings.Contains(hint, "push failed") {
		t.Errorf("expected hint to mention 'push failed', got: %s", hint)
	}
	if !strings.Contains(hint, "expired GitHub OAuth token") {
		t.Errorf("expected hint to mention 'expired GitHub OAuth token', got: %s", hint)
	}
	if !strings.Contains(hint, "gh auth token") {
		t.Errorf("expected hint to mention 'gh auth token', got: %s", hint)
	}
	if !strings.Contains(hint, "credentials.my-repo") {
		t.Errorf("expected hint to mention 'credentials.my-repo', got: %s", hint)
	}
}

func TestFormatAuthHint_GhoTokenPull(t *testing.T) {
	err := fmt.Errorf("authentication required")
	hint := FormatAuthHint("my-repo", "gho_abc123", err, false)
	if hint == "" {
		t.Fatal("expected non-empty hint for gho_ token pull failure")
	}
	if !strings.Contains(hint, "pull") {
		t.Errorf("expected hint to mention 'pull', got: %s", hint)
	}
}

func TestFormatAuthHint_NonGhoToken(t *testing.T) {
	err := fmt.Errorf("authentication required: bad password")
	hint := FormatAuthHint("my-repo", "ghp_abc123", err, true)
	if hint != "" {
		t.Errorf("expected empty hint for non-gho_ token, got: %s", hint)
	}
}

func TestFormatAuthHint_NilError(t *testing.T) {
	hint := FormatAuthHint("my-repo", "gho_abc123", nil, true)
	if hint != "" {
		t.Errorf("expected empty hint for nil error, got: %s", hint)
	}
}

// Ensure the internal isTransportAuthError delegates to the exported helper.
func TestIsTransportAuthError_InternalWrapper(t *testing.T) {
	if !isTransportAuthError(transport.ErrAuthenticationRequired) {
		t.Error("expected isTransportAuthError to delegate to IsTransportAuthError")
	}
	if isTransportAuthError(errors.New("unrelated")) {
		t.Error("expected isTransportAuthError to return false for unrelated errors")
	}
}
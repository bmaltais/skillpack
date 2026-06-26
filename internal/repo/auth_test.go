package repo

// Internal-package tests for auth helpers and Update fallback logic.
// These tests exercise code paths that are not reachable from the external
// repo_test package because they depend on unexported functions.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestIsAuthError_RecognisesAuthenticationRequired(t *testing.T) {
	if !isAuthError(transport.ErrAuthenticationRequired) {
		t.Error("expected isAuthError(ErrAuthenticationRequired) == true")
	}
}

func TestIsAuthError_RecognisesAuthorizationFailed(t *testing.T) {
	if !isAuthError(transport.ErrAuthorizationFailed) {
		t.Error("expected isAuthError(ErrAuthorizationFailed) == true")
	}
}

func TestIsAuthError_RecognisesWrappedErrors(t *testing.T) {
	wrapped := errors.New("outer: " + transport.ErrAuthenticationRequired.Error())
	// errors.Is traversal does not match non-wrapped errors; confirm isAuthError
	// returns false for plain string-matching errors (i.e. not sentinel-wrapped).
	if isAuthError(wrapped) {
		t.Error("plain string error should not match sentinel; isAuthError should return false")
	}

	// A properly wrapped sentinel should match.
	properlywrapped := fmt.Errorf("outer: %w", transport.ErrAuthenticationRequired)
	if !isAuthError(properlywrapped) {
		t.Error("expected isAuthError to return true for properly wrapped ErrAuthenticationRequired")
	}
}

func TestIsAuthError_ReturnsFalseForOtherErrors(t *testing.T) {
	for _, err := range []error{
		transport.ErrRepositoryNotFound,
		transport.ErrEmptyRemoteRepository,
		errors.New("connection refused"),
	} {
		if isAuthError(err) {
			t.Errorf("isAuthError(%v) = true, want false", err)
		}
	}
}

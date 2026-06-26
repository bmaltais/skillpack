package main

import (
	"strings"
	"testing"
)

func TestValidateRelinkFlags_MutualExclusion(t *testing.T) {
	err := validateRelinkFlags(1, "upstream/debugger", true)
	if err == nil {
		t.Fatal("expected error for --set-upstream + --clear-upstream, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %v", err)
	}
}

func TestValidateRelinkFlags_PositionalPlusSetUpstream(t *testing.T) {
	err := validateRelinkFlags(2, "upstream/debugger", false)
	if err == nil {
		t.Fatal("expected error for positional new-addr + --set-upstream, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("expected 'cannot be combined' in error, got: %v", err)
	}
}

func TestValidateRelinkFlags_PositionalPlusClearUpstream(t *testing.T) {
	err := validateRelinkFlags(2, "", true)
	if err == nil {
		t.Fatal("expected error for positional new-addr + --clear-upstream, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("expected 'cannot be combined' in error, got: %v", err)
	}
}

func TestValidateRelinkFlags_ValidCases(t *testing.T) {
	// Plain stale-addr repair (two positionals, no upstream flags).
	if err := validateRelinkFlags(2, "", false); err != nil {
		t.Errorf("two positionals with no flags should be valid, got: %v", err)
	}
	// --set-upstream alone with one positional.
	if err := validateRelinkFlags(1, "upstream/debugger", false); err != nil {
		t.Errorf("--set-upstream alone should be valid, got: %v", err)
	}
	// --clear-upstream alone with one positional.
	if err := validateRelinkFlags(1, "", true); err != nil {
		t.Errorf("--clear-upstream alone should be valid, got: %v", err)
	}
}

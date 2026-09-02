package main

import (
	"testing"

	"github.com/bmaltais/skillpack/internal/skill"
)

func TestPrintDuplicateSets_NoDuplicates_DoesNotPanic(t *testing.T) {
	printDuplicateSets(nil)
}

func TestPrintDuplicateSets_WithSets_DoesNotPanic(t *testing.T) {
	printDuplicateSets([]skill.DuplicateSet{
		{
			Basename: "triage",
			Members:  []string{"repo-a/triage", "repo-b/triage"},
			Pairs: []skill.DuplicatePair{
				{
					A:          "repo-a/triage",
					B:          "repo-b/triage",
					Confidence: skill.ConfidenceDiverged,
					LinkStatus: skill.LinkStatusUnlinked,
				},
			},
		},
	})
}

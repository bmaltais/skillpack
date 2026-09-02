package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bmaltais/skillpack/internal/repo"
	"github.com/bmaltais/skillpack/internal/skill"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Report skills duplicated across registered repos",
	Long: `Scan every registered repo's cache and report Duplicate Sets — skills that
appear to live in more than one registered repo.

Each set lists its member skill addresses, plus per-pair labels:
  confidence   identical (content hash matches) or diverged (content differs)
  link status  linked (fork) (tracked via "skillpack fork") or unlinked

doctor is strictly read-only: it never modifies files, state, or a remote.
Use "skillpack fork" separately to register provenance for a duplicate you
want skillpack to track.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		app := AppFromCtx(cmd.Context())
		if app == nil {
			return fmt.Errorf("configuration not available")
		}

		skills, err := repo.DiscoverAllSkills(app.St)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}

		sets, err := skill.DetectDuplicateSets(skills)
		if err != nil {
			return fmt.Errorf("detecting duplicate sets: %w", err)
		}

		printDuplicateSets(sets)
		return nil
	},
}

// printDuplicateSets renders doctor's report, grouped by Duplicate Set.
func printDuplicateSets(sets []skill.DuplicateSet) {
	if len(sets) == 0 {
		fmt.Println("No duplicates found.")
		return
	}

	for i, set := range sets {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s (%d members)\n", bold(set.Basename), len(set.Members))
		for _, m := range set.Members {
			fmt.Printf("  - %s\n", m)
		}
		for _, p := range set.Pairs {
			confidence := p.Confidence
			if confidence == skill.ConfidenceDiverged {
				confidence = yellow(confidence)
			}
			linkStatus := p.LinkStatus
			if linkStatus == skill.LinkStatusUnlinked {
				linkStatus = yellow(linkStatus)
			}
			fmt.Printf("  %s <-> %s: %s, %s\n", p.A, p.B, confidence, linkStatus)
		}
	}
}

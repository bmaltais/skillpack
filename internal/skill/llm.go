package skill

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmaltais/skillpack/internal/state"
)

// LLMResolver sends a prompt to an LLM and returns the response text.
type LLMResolver func(prompt string) (string, error)

// NewDefaultLLMResolver returns a resolver that invokes the named agent's CLI binary.
// Supported agents: claude-code (→ claude --print --no-markdown).
// For other agents the agent name itself is tried as a binary.
func NewDefaultLLMResolver(agentName string) (LLMResolver, error) {
	bin, args := agentCLIArgs(agentName)
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("agent %q: binary %q not found in PATH — install the agent CLI to use --llm", agentName, bin)
	}
	return func(prompt string) (string, error) {
		cmdArgs := make([]string, len(args)+1)
		copy(cmdArgs, args)
		cmdArgs[len(args)] = prompt
		out, err := exec.Command(bin, cmdArgs...).Output() //nolint:gosec
		if err != nil {
			return "", fmt.Errorf("LLM call (%s) failed: %w", bin, err)
		}
		return string(out), nil
	}, nil
}

// agentCLIArgs maps an agent name to a (binary, args) pair for a prompt call.
func agentCLIArgs(agentName string) (string, []string) {
	switch agentName {
	case "claude-code":
		return "claude", []string{"--print", "--no-markdown"}
	default:
		return agentName, []string{}
	}
}

// LLMResolveConflicts scans the installed skill directory for files containing
// conflict markers, sends each conflicted file to the resolver, and writes the
// clean result back.
//
// Returns an error if the resolver returns output that still contains conflict
// markers, or if the resolver itself errors.
// Returns nil if no conflicted files are found.
func LLMResolveConflicts(addr, agentName string, resolver LLMResolver, st *state.State) error {
	agents, ok := st.InstalledSkills[addr]
	if !ok {
		return fmt.Errorf("skill %q is not installed", addr)
	}
	rec, ok := agents[agentName]
	if !ok {
		return fmt.Errorf("skill %q is not installed for agent %q", addr, agentName)
	}

	filesOnDisk := listFilesOnDisk(rec.LocalPath)
	for relPath, content := range filesOnDisk {
		if !strings.Contains(content, "<<<<<<<") {
			continue
		}

		prompt := buildLLMPrompt(addr, relPath, content)
		resolved, err := resolver(prompt)
		if err != nil {
			return fmt.Errorf("LLM resolution for %s: %w", relPath, err)
		}

		if strings.Contains(resolved, "<<<<<<<") {
			return fmt.Errorf(
				"LLM resolution for %s still contains conflict markers — manual review required; file not overwritten",
				relPath,
			)
		}

		targetPath := filepath.Join(rec.LocalPath, filepath.FromSlash(relPath))
		if err := writeStringToFile(targetPath, resolved); err != nil {
			return fmt.Errorf("writing resolved file %s: %w", relPath, err)
		}
	}
	return nil
}

func buildLLMPrompt(addr, relPath, content string) string {
	return fmt.Sprintf(
		"Resolve the merge conflict in this skill file and return only the resolved file content with no preamble or explanation.\n\nSkill: %s\nFile: %s\n\n%s",
		addr, relPath, content,
	)
}

// Package auth handles pre-TUI authentication validation and re-auth prompts.
package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tttao/dbx-dash/internal/config"
)

// Validator is the subset of WorkspaceClientFactory used for auth checks.
type Validator interface {
	ValidateAuth(ctx context.Context, p config.WorkspaceProfile) error
	EvictCache(name string)
}

// Run checks auth for every visible workspace before the TUI starts.
// For each workspace whose auth fails the user is prompted interactively:
//   - yes → run `databricks auth login` and retry once
//   - no  → mark workspace as disabled for this session
//
// Returns the names of workspaces the user chose to skip.
func Run(ctx context.Context, cfg *config.AppConfig, factory Validator) ([]string, error) {
	var disabled []string

	for _, ws := range cfg.VisibleWorkspaces() {
		if err := factory.ValidateAuth(ctx, ws); err == nil {
			continue // auth OK
		}

		fmt.Printf("\n⚠  workspace %q: auth check failed\n", ws.Name)

		if promptYN(fmt.Sprintf("   Re-authenticate now? [y/N] ")) {
			reauth(ws)
			factory.EvictCache(ws.Name) // force fresh client with updated credentials
			if factory.ValidateAuth(ctx, ws) == nil {
				fmt.Printf("   ✓  workspace %q authenticated successfully\n", ws.Name)
				continue
			}
			fmt.Printf("   ✗  workspace %q still unavailable after re-auth\n", ws.Name)
		}

		fmt.Printf("   → workspace %q will be skipped this session\n", ws.Name)
		disabled = append(disabled, ws.Name)
	}

	if len(disabled) > 0 {
		fmt.Println()
	}
	return disabled, nil
}

// promptYN prints the prompt and reads a single y/Y response; anything else is No.
func promptYN(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}

// reauth shells out to `databricks auth login` for the given workspace profile.
// stdin/stdout/stderr are inherited so the OAuth browser flow works normally.
func reauth(ws config.WorkspaceProfile) {
	args := []string{"auth", "login", "--host", ws.Host}
	if ws.Name != "" {
		args = append(args, "--profile", ws.Name)
	}
	cmd := exec.Command("databricks", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("   databricks auth login: %v\n", err)
	}
}

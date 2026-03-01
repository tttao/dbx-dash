package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tttao/dbx-dash/internal/auth"
	"github.com/tttao/dbx-dash/internal/cache"
	"github.com/tttao/dbx-dash/internal/config"
	"github.com/tttao/dbx-dash/internal/databricks"
	dbsdk "github.com/tttao/dbx-dash/internal/databricks/sdk"
	"github.com/tttao/dbx-dash/internal/tui"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "dbx-dash",
		Short:   "Terminal dashboard for Databricks workspaces",
		Version: version,
	}

	var (
		flagWorkspace  string
		flagWorkspaces string
		flagRefresh    int
	)

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Launch the dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Apply CLI filters.
			if flagWorkspace != "" {
				cfg.ActiveWorkspaces = []string{flagWorkspace}
			} else if flagWorkspaces != "" {
				cfg.ActiveWorkspaces = splitComma(flagWorkspaces)
			}
			if flagRefresh > 0 {
				cfg.Refresh.DefaultInterval = flagRefresh
			}

			visible := cfg.VisibleWorkspaces()
			if len(visible) == 0 {
				return fmt.Errorf("no workspaces found in ~/.databrickscfg")
			}

			factory := dbsdk.NewWorkspaceClientFactory()

			disabled, err := auth.Run(context.Background(), cfg, factory)
			if err != nil {
				return fmt.Errorf("auth check: %w", err)
			}
			disabledSet := make(map[string]struct{}, len(disabled))
			for _, n := range disabled {
				disabledSet[n] = struct{}{}
			}

			providers := make(map[string]*databricks.WorkspaceProviders, len(visible))
			for _, ws := range visible {
				if _, skip := disabledSet[ws.Name]; skip {
					continue
				}
				p, err := factory.GetProviders(ws)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: workspace %q: %v\n", ws.Name, err)
					continue
				}
				providers[ws.Name] = p
			}

			// Open local cache (best-effort; nil repo = cache disabled).
			var cacheRepo *cache.Repository
			if db, err := cache.Open("~/.dbx-dash/cache.db"); err == nil {
				cacheRepo = cache.NewRepository(db)
				defer db.Close()
			}

			return tui.Run(cfg, providers, disabled, cacheRepo)
		},
	}
	runCmd.Flags().StringVarP(&flagWorkspace, "workspace", "w", "", "show only this workspace")
	runCmd.Flags().StringVar(&flagWorkspaces, "workspaces", "", "comma-separated list of workspaces to show")
	runCmd.Flags().IntVarP(&flagRefresh, "refresh", "r", 0, "refresh interval in seconds (overrides config)")

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration utilities",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured workspaces",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Workspaces) == 0 {
				fmt.Println("No workspaces found. Add profiles to ~/.databrickscfg.")
				return nil
			}
			fmt.Printf("%-20s %-50s %s\n", "NAME", "HOST", "AUTH")
			fmt.Println(strings.Repeat("─", 80))
			for _, ws := range cfg.Workspaces {
				fmt.Printf("%-20s %-50s %s\n", ws.Name, ws.Host, ws.AuthType)
			}
			return nil
		},
	}

	checkCmd := &cobra.Command{
		Use:   "check WORKSPACE",
		Short: "Test connectivity to a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			ws := cfg.GetWorkspace(name)
			if ws == nil {
				return fmt.Errorf("workspace %q not found in ~/.databrickscfg", name)
			}
			factory := dbsdk.NewWorkspaceClientFactory()
			p, err := factory.GetProviders(*ws)
			if err != nil {
				return fmt.Errorf("init client: %w", err)
			}
			jobs, err := p.Jobs.ListJobs(cmd.Context(), 1)
			if err != nil {
				return fmt.Errorf("connectivity check failed: %w", err)
			}
			fmt.Printf("OK  workspace=%s  host=%s  jobs_visible=%d\n", ws.Name, ws.Host, len(jobs))
			return nil
		},
	}

	configCmd.AddCommand(listCmd, checkCmd)
	root.AddCommand(runCmd, configCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

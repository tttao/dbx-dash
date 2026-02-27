package config

import "strings"

// WorkspaceProfile represents a single Databricks workspace connection.
type WorkspaceProfile struct {
	Name            string
	Host            string
	AuthType        string // "pat" | "oauth-m2m"
	Token           string
	ClientID        string
	ClientSecret    string
	DisplayName     string // overlay from ~/.dbx-dash/config.toml
	RefreshInterval int    // seconds, default 30
	Source          string // "databrickscfg" | "dbx_dash_config"
}

// Label returns the display label for the workspace.
func (p WorkspaceProfile) Label() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Name
}

// RefreshConfig holds global refresh settings.
type RefreshConfig struct {
	DefaultInterval    int  // seconds
	AlertOnFailure     bool
}

// AppConfig is the top-level application configuration.
type AppConfig struct {
	Workspaces       []WorkspaceProfile
	Refresh          RefreshConfig
	DBPath           string
	ActiveWorkspaces []string // filter; nil = all
}

// GetWorkspace returns the profile with the given name, or nil.
func (c *AppConfig) GetWorkspace(name string) *WorkspaceProfile {
	for i := range c.Workspaces {
		if c.Workspaces[i].Name == name {
			return &c.Workspaces[i]
		}
	}
	return nil
}

// VisibleWorkspaces returns workspaces shown in multi-workspace views.
func (c *AppConfig) VisibleWorkspaces() []WorkspaceProfile {
	if len(c.ActiveWorkspaces) == 0 {
		return c.Workspaces
	}
	set := make(map[string]struct{}, len(c.ActiveWorkspaces))
	for _, n := range c.ActiveWorkspaces {
		set[strings.ToLower(n)] = struct{}{}
	}
	var out []WorkspaceProfile
	for _, ws := range c.Workspaces {
		if _, ok := set[strings.ToLower(ws.Name)]; ok {
			out = append(out, ws)
		}
	}
	return out
}

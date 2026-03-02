// Package config loads workspace profiles from ~/.databrickscfg (INI) and
// optional overrides from ~/.dbx-dash/config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/ini.v1"
)

const (
	defaultInterval = 30
	defaultDBPath   = "~/.dbx-dash/cache.db"
)

// Load merges ~/.databrickscfg and ~/.dbx-dash/config.toml into an AppConfig.
func Load() (*AppConfig, error) {
	cfg := &AppConfig{
		Refresh: RefreshConfig{
			DefaultInterval: defaultInterval,
			AlertOnFailure:  true,
		},
		DBPath:      defaultDBPath,
		JobsAgeDays: 1,
	}

	if err := loadDatabricksCfg(cfg); err != nil {
		return nil, err
	}
	// Overlay with dbx-dash config (optional — ignore if missing)
	_ = loadDbxDashConfig(cfg)

	return cfg, nil
}

// loadDatabricksCfg reads ~/.databrickscfg and populates workspace profiles.
func loadDatabricksCfg(cfg *AppConfig) error {
	path := filepath.Join(homeDir(), ".databrickscfg")
	iniFile, err := ini.LoadSources(ini.LoadOptions{
		AllowBooleanKeys:        true,
		IgnoreInlineComment:     true,
		UnescapeValueDoubleQuotes: true,
	}, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no config file is OK
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// DEFAULT section values act as fallback.
	defaults := iniFile.Section(ini.DefaultSection)

	for _, section := range iniFile.Sections() {
		name := section.Name()
		if name == ini.DefaultSection {
			continue
		}

		host := keyVal(section, defaults, "host")
		if host == "" {
			continue // skip sections without a host
		}

		p := WorkspaceProfile{
			Name:            name,
			Host:            strings.TrimRight(host, "/"),
			AuthType:        keyValDefault(section, defaults, "auth_type", "pat"),
			Token:           keyVal(section, defaults, "token"),
			ClientID:        keyVal(section, defaults, "client_id"),
			ClientSecret:    keyVal(section, defaults, "client_secret"),
			RefreshInterval: defaultInterval,
			Source:          "databrickscfg",
		}
		cfg.Workspaces = append(cfg.Workspaces, p)
	}
	return nil
}

// tomlOverlay mirrors the TOML schema of ~/.dbx-dash/config.toml.
type tomlOverlay struct {
	Workspace []struct {
		Name            string `toml:"name"`
		DisplayName     string `toml:"display_name"`
		RefreshInterval int    `toml:"refresh_interval"`
	} `toml:"workspace"`
	Refresh struct {
		DefaultInterval int  `toml:"default_interval"`
		AlertOnFailure  bool `toml:"alert_on_failure"`
	} `toml:"refresh"`
	DBPath      string `toml:"db_path"`
	JobsAgeDays int    `toml:"jobs_age_days"`
}

// loadDbxDashConfig applies optional overrides from ~/.dbx-dash/config.toml.
func loadDbxDashConfig(cfg *AppConfig) error {
	path := filepath.Join(homeDir(), ".dbx-dash", "config.toml")
	var overlay tomlOverlay
	if _, err := toml.DecodeFile(path, &overlay); err != nil {
		return err // caller ignores this error
	}

	if overlay.Refresh.DefaultInterval > 0 {
		cfg.Refresh.DefaultInterval = overlay.Refresh.DefaultInterval
	}
	cfg.Refresh.AlertOnFailure = overlay.Refresh.AlertOnFailure
	if overlay.DBPath != "" {
		cfg.DBPath = overlay.DBPath
	}
	if overlay.JobsAgeDays >= 0 {
		cfg.JobsAgeDays = overlay.JobsAgeDays
	}

	// Build a lookup map for existing workspaces by name.
	idx := make(map[string]int, len(cfg.Workspaces))
	for i, ws := range cfg.Workspaces {
		idx[ws.Name] = i
	}

	for _, ow := range overlay.Workspace {
		if i, ok := idx[ow.Name]; ok {
			if ow.DisplayName != "" {
				cfg.Workspaces[i].DisplayName = ow.DisplayName
			}
			if ow.RefreshInterval > 0 {
				cfg.Workspaces[i].RefreshInterval = ow.RefreshInterval
			}
		}
		// New workspace defined entirely in dbx-dash config — skip if no host.
	}
	return nil
}

// homeDir returns the user's home directory.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "~"
}

func keyVal(section, defaults *ini.Section, key string) string {
	if section.HasKey(key) {
		return section.Key(key).String()
	}
	if defaults.HasKey(key) {
		return defaults.Key(key).String()
	}
	return ""
}

func keyValDefault(section, defaults *ini.Section, key, fallback string) string {
	v := keyVal(section, defaults, key)
	if v == "" {
		return fallback
	}
	return v
}

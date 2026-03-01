package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/cache"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// WorkspaceSummary holds health metrics for one workspace.
type WorkspaceSummary struct {
	Name           string
	RunningJobs    int
	FailedJobs     int
	ActiveClusters int
	Warehouses     int
	Pipelines      int
	Loading        bool
	Disabled       bool // auth was skipped for this session
	FromCache      bool // data served from local cache
	Err            error
}

// DashboardLoadedMsg is sent when all workspace summaries have been fetched.
type DashboardLoadedMsg struct {
	Summaries []WorkspaceSummary
}

// LoadDashboardCmd starts concurrent fetches for all workspaces.
func LoadDashboardCmd(ctx context.Context, workspaces map[string]*databricks.WorkspaceProviders, repo *cache.Repository) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(workspaces))
	for name, providers := range workspaces {
		n, p := name, providers
		cmds = append(cmds, func() tea.Msg {
			return fetchSummary(ctx, n, p, repo)
		})
	}
	return tea.Batch(cmds...)
}

// workspaceSummaryMsg carries a single workspace summary result.
type workspaceSummaryMsg WorkspaceSummary

func fetchSummary(ctx context.Context, name string, p *databricks.WorkspaceProviders, repo *cache.Repository) tea.Msg {
	s := WorkspaceSummary{Name: name, Loading: false}

	jobs, err := p.Jobs.ListJobs(ctx, 100)
	if err != nil {
		// Fallback: use cached jobs for the summary counters.
		if repo != nil {
			if cachedJobs, cachedRuns, cErr := repo.GetJobsAsDomain(name); cErr == nil && len(cachedJobs) > 0 {
				for _, j := range cachedJobs {
					if r, ok := cachedRuns[j.JobID]; ok {
						switch r.State {
						case "RUNNING":
							s.RunningJobs++
						case "TERMINATED":
							if r.ResultState == "FAILED" {
								s.FailedJobs++
							}
						}
					}
				}
				s.FromCache = true
			} else {
				s.Err = err
				return workspaceSummaryMsg(s)
			}
		} else {
			s.Err = err
			return workspaceSummaryMsg(s)
		}
	} else {
		for _, j := range jobs {
			runs, _ := p.Jobs.ListRecentRuns(ctx, j.JobID, 1)
			if len(runs) > 0 {
				switch runs[0].State {
				case "RUNNING":
					s.RunningJobs++
				case "TERMINATED":
					if runs[0].ResultState == "FAILED" {
						s.FailedJobs++
					}
				}
				if repo != nil {
					_ = repo.UpsertJob(name, j, &runs[0])
				}
			} else if repo != nil {
				_ = repo.UpsertJob(name, j, nil)
			}
		}
	}

	clusters, err := p.Clusters.ListClusters(ctx)
	if err == nil {
		for _, c := range clusters {
			if c.State == "RUNNING" {
				s.ActiveClusters++
			}
		}
		if repo != nil {
			for _, c := range clusters {
				_ = repo.UpsertCluster(name, c)
			}
		}
	} else if repo != nil {
		if cached, cErr := repo.GetClustersAsDomain(name); cErr == nil {
			for _, c := range cached {
				if c.State == "RUNNING" {
					s.ActiveClusters++
				}
			}
			s.FromCache = true
		}
	}

	warehouses, err := p.Warehouses.ListWarehouses(ctx)
	if err == nil {
		s.Warehouses = len(warehouses)
	}

	pipelines, err := p.Pipelines.ListPipelines(ctx)
	if err == nil {
		s.Pipelines = len(pipelines)
	}

	return workspaceSummaryMsg(s)
}

// DashboardModel is the Bubble Tea model for the dashboard screen.
type DashboardModel struct {
	summaries map[string]WorkspaceSummary
	order     []string // workspace name order
	width     int
}

// NewDashboardModel creates a dashboard model with workspace names pre-loaded.
// disabledNames lists workspaces whose auth was skipped; they are shown but not polled.
func NewDashboardModel(workspaceNames []string, disabledNames []string) DashboardModel {
	disabled := make(map[string]struct{}, len(disabledNames))
	for _, n := range disabledNames {
		disabled[n] = struct{}{}
	}
	summaries := make(map[string]WorkspaceSummary, len(workspaceNames))
	for _, n := range workspaceNames {
		if _, ok := disabled[n]; ok {
			summaries[n] = WorkspaceSummary{Name: n, Disabled: true}
		} else {
			summaries[n] = WorkspaceSummary{Name: n, Loading: true}
		}
	}
	return DashboardModel{
		summaries: summaries,
		order:     workspaceNames,
	}
}

func (m DashboardModel) SetWidth(w int) DashboardModel {
	m.width = w
	return m
}

// Update handles dashboard-specific messages.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch v := msg.(type) {
	case workspaceSummaryMsg:
		s := WorkspaceSummary(v)
		m.summaries[s.Name] = s
		return m, func() tea.Msg { return DashboardLoadedMsg{} }
	}
	return m, nil
}

func (m DashboardModel) View() string {
	var b strings.Builder
	for _, name := range m.order {
		s, ok := m.summaries[name]
		if !ok {
			continue
		}
		b.WriteString(renderWorkspaceSummary(s, m.width))
		b.WriteString("\n")
	}
	return b.String()
}

func renderWorkspaceSummary(s WorkspaceSummary, _ int) string {
	label := s.Name
	if s.FromCache {
		label += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" - Cached")
	} else if !s.Loading && !s.Disabled && s.Err == nil {
		label += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" - Live")
	}
	title := lipgloss.NewStyle().Bold(true).Render("  " + label)
	if s.Disabled {
		skipped := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ⊘ auth skipped — workspace unavailable this session")
		return title + "\n" + skipped + "\n"
	}
	if s.Loading {
		return title + "\n  loading…\n"
	}
	if s.Err != nil {
		return title + "\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: "+s.Err.Error()) + "\n"
	}

	running := renderCard("Running Jobs", fmt.Sprintf("%d", s.RunningJobs), "42")
	failed := renderCard("Failed Jobs", fmt.Sprintf("%d", s.FailedJobs), "196")
	clusters := renderCard("Active Clusters", fmt.Sprintf("%d", s.ActiveClusters), "14")
	warehouses := renderCard("Warehouses", fmt.Sprintf("%d", s.Warehouses), "240")
	pipelines := renderCard("Pipelines", fmt.Sprintf("%d", s.Pipelines), "240")

	row := lipgloss.JoinHorizontal(lipgloss.Top, running, " ", failed, " ", clusters, " ", warehouses, " ", pipelines)
	return title + "\n" + row + "\n"
}

func renderCard(label, value, color string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(16)

	val := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(value)
	lbl := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(label)
	return style.Render(lbl + "\n" + val)
}

package screens

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/you/dbx-dash/internal/databricks"
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
	Err            error
}

// DashboardLoadedMsg is sent when all workspace summaries have been fetched.
type DashboardLoadedMsg struct {
	Summaries []WorkspaceSummary
}

// LoadDashboardCmd starts concurrent fetches for all workspaces.
func LoadDashboardCmd(ctx context.Context, workspaces map[string]*databricks.WorkspaceProviders) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(workspaces))
	for name, providers := range workspaces {
		n, p := name, providers
		cmds = append(cmds, func() tea.Msg {
			return fetchSummary(ctx, n, p)
		})
	}
	return tea.Batch(cmds...)
}

// workspaceSummaryMsg carries a single workspace summary result.
type workspaceSummaryMsg WorkspaceSummary

func fetchSummary(ctx context.Context, name string, p *databricks.WorkspaceProviders) tea.Msg {
	s := WorkspaceSummary{Name: name, Loading: false}

	jobs, err := p.Jobs.ListJobs(ctx, 100)
	if err != nil {
		s.Err = err
		return workspaceSummaryMsg(s)
	}
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
		}
	}

	clusters, err := p.Clusters.ListClusters(ctx)
	if err == nil {
		for _, c := range clusters {
			if c.State == "RUNNING" {
				s.ActiveClusters++
			}
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
func NewDashboardModel(workspaceNames []string) DashboardModel {
	summaries := make(map[string]WorkspaceSummary, len(workspaceNames))
	for _, n := range workspaceNames {
		summaries[n] = WorkspaceSummary{Name: n, Loading: true}
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
	title := lipgloss.NewStyle().Bold(true).Render("  " + s.Name)
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

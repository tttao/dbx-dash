package screens

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/cache"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// ClustersLoadedMsg carries cluster data for a workspace.
type ClustersLoadedMsg struct {
	Workspace string
	Clusters  []databricks.Cluster
	FromCache bool
	Err       error
}

// LoadClustersCmd fetches clusters for a workspace.
// Falls back to the local cache on error; writes through to cache on success.
func LoadClustersCmd(ctx context.Context, ws string, p *databricks.WorkspaceProviders, repo *cache.Repository) tea.Cmd {
	return func() tea.Msg {
		clusters, err := p.Clusters.ListClusters(ctx)
		if err != nil {
			if repo != nil {
				if cached, cErr := repo.GetClustersAsDomain(ws); cErr == nil && len(cached) > 0 {
					return ClustersLoadedMsg{Workspace: ws, Clusters: cached, FromCache: true}
				}
			}
			return ClustersLoadedMsg{Workspace: ws, Err: err}
		}
		// Write-through: update cache.
		if repo != nil {
			for _, c := range clusters {
				_ = repo.UpsertCluster(ws, c)
			}
		}
		return ClustersLoadedMsg{Workspace: ws, Clusters: clusters, FromCache: false}
	}
}

type clusterEntry struct {
	cluster   databricks.Cluster
	workspace string
}

// ClustersModel is the Bubble Tea model for the clusters screen.
type ClustersModel struct {
	table   table.Model
	entries []clusterEntry
	width   int
	height  int
}

func NewClustersModel() ClustersModel {
	cols := []table.Column{
		{Title: "Cluster", Width: 30},
		{Title: "State", Width: 14},
		{Title: "Workers", Width: 10},
		{Title: "Node Type", Width: 18},
		{Title: "Source", Width: 12},
		{Title: "Workspace", Width: 14},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("57"))
	t.SetStyles(s)
	return ClustersModel{table: t}
}

func (m ClustersModel) SetSize(w, h int) ClustersModel {
	m.width = w
	m.height = h
	m.table.SetHeight(h - 6)
	return m
}

func (m ClustersModel) Update(msg tea.Msg) (ClustersModel, tea.Cmd) {
	switch v := msg.(type) {
	case ClustersLoadedMsg:
		if v.Err == nil {
			for _, c := range v.Clusters {
				m.entries = append(m.entries, clusterEntry{cluster: c, workspace: v.Workspace})
			}
			m.table.SetRows(m.buildRows())
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *ClustersModel) buildRows() []table.Row {
	rows := make([]table.Row, 0, len(m.entries))
	for _, e := range m.entries {
		workers := fmt.Sprintf("%d", e.cluster.NumWorkers)
		if e.cluster.AutoscaleMin > 0 {
			workers = fmt.Sprintf("%d-%d", e.cluster.AutoscaleMin, e.cluster.AutoscaleMax)
		}
		rows = append(rows, table.Row{
			e.cluster.Name, e.cluster.State, workers,
			e.cluster.NodeTypeID, e.cluster.Source, e.workspace,
		})
	}
	return rows
}

func (m ClustersModel) View() string {
	return lipgloss.NewStyle().Width(m.width).Render(m.table.View())
}

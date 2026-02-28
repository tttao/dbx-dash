package screens

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// WarehousesLoadedMsg carries SQL warehouse data for a workspace.
type WarehousesLoadedMsg struct {
	Workspace  string
	Warehouses []databricks.SqlWarehouse
	Err        error
}

// LoadWarehousesCmd fetches SQL warehouses for a workspace.
func LoadWarehousesCmd(ctx context.Context, ws string, p *databricks.WorkspaceProviders) tea.Cmd {
	return func() tea.Msg {
		warehouses, err := p.Warehouses.ListWarehouses(ctx)
		return WarehousesLoadedMsg{Workspace: ws, Warehouses: warehouses, Err: err}
	}
}

type warehouseEntry struct {
	warehouse databricks.SqlWarehouse
	workspace string
}

// WarehousesModel is the Bubble Tea model for the SQL warehouses screen.
type WarehousesModel struct {
	table   table.Model
	entries []warehouseEntry
	width   int
	height  int
}

func NewWarehousesModel() WarehousesModel {
	cols := []table.Column{
		{Title: "Warehouse", Width: 30},
		{Title: "State", Width: 12},
		{Title: "Size", Width: 12},
		{Title: "Clusters", Width: 10},
		{Title: "Sessions", Width: 10},
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
	return WarehousesModel{table: t}
}

func (m WarehousesModel) SetSize(w, h int) WarehousesModel {
	m.width = w
	m.height = h
	m.table.SetHeight(h - 6)
	return m
}

func (m WarehousesModel) Update(msg tea.Msg) (WarehousesModel, tea.Cmd) {
	switch v := msg.(type) {
	case WarehousesLoadedMsg:
		if v.Err == nil {
			for _, w := range v.Warehouses {
				m.entries = append(m.entries, warehouseEntry{warehouse: w, workspace: v.Workspace})
			}
			m.table.SetRows(m.buildRows())
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *WarehousesModel) buildRows() []table.Row {
	rows := make([]table.Row, 0, len(m.entries))
	for _, e := range m.entries {
		clusters := fmt.Sprintf("%d-%d", e.warehouse.MinClusters, e.warehouse.MaxClusters)
		sessions := fmt.Sprintf("%d", e.warehouse.ActiveSessions)
		rows = append(rows, table.Row{
			e.warehouse.Name, e.warehouse.State, e.warehouse.ClusterSize,
			clusters, sessions, e.workspace,
		})
	}
	return rows
}

func (m WarehousesModel) View() string {
	return lipgloss.NewStyle().Width(m.width).Render(m.table.View())
}

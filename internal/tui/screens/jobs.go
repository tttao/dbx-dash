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

// SelectedJob is returned by JobsModel.SelectedRow() so callers outside this
// package can inspect the selected job and its most recent run.
type SelectedJob struct {
	Job       databricks.Job
	Run       *databricks.JobRun
	Workspace string
}

// jobRow is the internal display row.
type jobRow struct {
	job       databricks.Job
	run       *databricks.JobRun
	workspace string
}

// JobsLoadedMsg carries the jobs list for a workspace.
type JobsLoadedMsg struct {
	Workspace string
	Jobs      []jobRow
	FromCache bool
	Err       error
}

// LoadJobsCmd fetches jobs and their most recent run for a workspace.
// If the live API fails, falls back to the local cache (if repo is non-nil).
// On success, writes results to the cache.
func LoadJobsCmd(ctx context.Context, ws string, p *databricks.WorkspaceProviders, repo *cache.Repository) tea.Cmd {
	return func() tea.Msg {
		jobs, err := p.Jobs.ListJobs(ctx, 100)
		if err != nil {
			// Try cache fallback.
			if repo != nil {
				cachedJobs, cachedRuns, cErr := repo.GetJobsAsDomain(ws)
				if cErr == nil && len(cachedJobs) > 0 {
					rows := make([]jobRow, 0, len(cachedJobs))
					for _, j := range cachedJobs {
						row := jobRow{job: j, workspace: ws}
						if r, ok := cachedRuns[j.JobID]; ok {
							row.run = r
						}
						rows = append(rows, row)
					}
					return JobsLoadedMsg{Workspace: ws, Jobs: rows, FromCache: true}
				}
			}
			return JobsLoadedMsg{Workspace: ws, Err: err}
		}
		rows := make([]jobRow, 0, len(jobs))
		for _, j := range jobs {
			row := jobRow{job: j, workspace: ws}
			runs, _ := p.Jobs.ListRecentRuns(ctx, j.JobID, 1)
			if len(runs) > 0 {
				r := runs[0]
				row.run = &r
				// Write-through: update cache with latest run data.
				if repo != nil {
					_ = repo.UpsertJob(ws, j, &r)
				}
			} else if repo != nil {
				_ = repo.UpsertJob(ws, j, nil)
			}
			rows = append(rows, row)
		}
		return JobsLoadedMsg{Workspace: ws, Jobs: rows, FromCache: false}
	}
}

// JobsModel is the Bubble Tea model for the jobs screen.
type JobsModel struct {
	table     table.Model
	rows   []jobRow
	width  int
	height    int
}

func NewJobsModel() JobsModel {
	cols := []table.Column{
		{Title: "Job", Width: 35},
		{Title: "State", Width: 14},
		{Title: "Result", Width: 12},
		{Title: "Duration", Width: 10},
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
	return JobsModel{table: t}
}

func (m JobsModel) SetSize(w, h int) JobsModel {
	m.width = w
	m.height = h
	m.table.SetHeight(h - 6)
	return m
}

func (m JobsModel) Update(msg tea.Msg) (JobsModel, tea.Cmd) {
	switch v := msg.(type) {
	case JobsLoadedMsg:
		if v.Err == nil {
			m.rows = append(m.rows, v.Jobs...)
			m.table.SetRows(m.buildRows())
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *JobsModel) buildRows() []table.Row {
	rows := make([]table.Row, 0, len(m.rows))
	for _, r := range m.rows {
		state, result, dur := "–", "–", "–"
		if r.run != nil {
			state = r.run.State
			result = r.run.ResultState
			if r.run.DurationMs > 0 {
				dur = fmt.Sprintf("%.0fs", float64(r.run.DurationMs)/1000)
			}
		}
		rows = append(rows, table.Row{r.job.Name, state, result, dur, r.workspace})
	}
	return rows
}

// SelectedRow returns the selected job entry, or nil if nothing is selected.
func (m JobsModel) SelectedRow() *SelectedJob {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.rows) {
		return nil
	}
	r := m.rows[idx]
	return &SelectedJob{Job: r.job, Run: r.run, Workspace: r.workspace}
}

func (m JobsModel) View() string {
	return lipgloss.NewStyle().Width(m.width).Render(m.table.View())
}

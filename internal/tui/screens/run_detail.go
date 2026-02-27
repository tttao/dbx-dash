package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/you/dbx-dash/internal/databricks"
)

// RunOutputLoadedMsg carries run output for the detail screen.
type RunOutputLoadedMsg struct {
	Output *databricks.RunOutput
	Err    error
}

// LoadRunOutputCmd fetches output for a specific run.
func LoadRunOutputCmd(ctx context.Context, runID int64, p databricks.JobsProvider) tea.Cmd {
	return func() tea.Msg {
		out, err := p.GetRunOutput(ctx, runID)
		return RunOutputLoadedMsg{Output: out, Err: err}
	}
}

// RunDetailModel is the Bubble Tea model for the run detail screen.
type RunDetailModel struct {
	viewport viewport.Model
	output   *databricks.RunOutput
	runID    int64
	loading  bool
	err      error
	width    int
	height   int
}

func NewRunDetailModel(runID int64) RunDetailModel {
	vp := viewport.New(80, 20)
	return RunDetailModel{
		viewport: vp,
		runID:    runID,
		loading:  true,
	}
}

func (m RunDetailModel) SetSize(w, h int) RunDetailModel {
	m.width = w
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h - 8
	return m
}

func (m RunDetailModel) Update(msg tea.Msg) (RunDetailModel, tea.Cmd) {
	switch v := msg.(type) {
	case RunOutputLoadedMsg:
		m.loading = false
		m.err = v.Err
		m.output = v.Output
		if v.Output != nil {
			m.viewport.SetContent(buildRunContent(v.Output))
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m RunDetailModel) View() string {
	if m.loading {
		return "  Loading run output…\n"
	}
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("  Error: "+m.err.Error()) + "\n"
	}
	if m.output == nil {
		return "  No output available.\n"
	}

	header := buildRunHeader(m.output)
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ↑/↓ scroll  esc/b back")
	return header + "\n" + m.viewport.View() + "\n" + help
}

func buildRunHeader(o *databricks.RunOutput) string {
	var b strings.Builder
	if o.Metadata != nil {
		r := o.Metadata
		b.WriteString(fmt.Sprintf("  Run #%d  State: %s", r.RunID, r.State))
		if r.ResultState != "" {
			b.WriteString("  Result: " + r.ResultState)
		}
		if !r.StartTime.IsZero() {
			b.WriteString("  Started: " + r.StartTime.Format("15:04:05"))
		}
		if r.RunPageURL != "" {
			b.WriteString("  URL: " + r.RunPageURL)
		}
	}
	return lipgloss.NewStyle().Bold(true).Render(b.String())
}

func buildRunContent(o *databricks.RunOutput) string {
	var parts []string
	if o.Error != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: "+o.Error))
	}
	if o.ErrorTrace != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(o.ErrorTrace))
	}
	if o.Logs != "" {
		parts = append(parts, o.Logs)
	}
	if len(parts) == 0 {
		return "(no output)"
	}
	return strings.Join(parts, "\n")
}

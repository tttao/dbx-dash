// Package tui implements the Bubble Tea terminal dashboard.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/cache"
	"github.com/tttao/dbx-dash/internal/config"
	"github.com/tttao/dbx-dash/internal/databricks"
	"github.com/tttao/dbx-dash/internal/tui/screens"
)

type screenID int

const (
	screenDashboard screenID = iota
	screenJobs
	screenClusters
	screenWarehouses
	screenIdentity
	screenRunDetail
)

// tickMsg drives auto-refresh.
type tickMsg time.Time

// Model is the root Bubble Tea model.
type Model struct {
	cfg        *config.AppConfig
	providers  map[string]*databricks.WorkspaceProviders
	cacheRepo  *cache.Repository       // nil when cache is unavailable
	wsDataMode map[string]string        // workspace name -> "live" | "cached"
	screen     screenID
	wsIdx      int
	dashboard  screens.DashboardModel
	jobs       screens.JobsModel
	clusters   screens.ClustersModel
	warehouses screens.WarehousesModel
	identity   screens.IdentityModel
	runDetail  screens.RunDetailModel
	spinner       spinner.Model
	loading       bool
	lastRefreshed time.Time
	width         int
	height        int
	interval   time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewModel initialises the root model.
func NewModel(cfg *config.AppConfig, providers map[string]*databricks.WorkspaceProviders, disabled []string, cacheRepo *cache.Repository) Model {
	ctx, cancel := context.WithCancel(context.Background())
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	wsNames := make([]string, 0, len(cfg.VisibleWorkspaces()))
	for _, ws := range cfg.VisibleWorkspaces() {
		wsNames = append(wsNames, ws.Name)
	}

	interval := time.Duration(cfg.Refresh.DefaultInterval) * time.Second

	return Model{
		cfg:        cfg,
		providers:  providers,
		cacheRepo:  cacheRepo,
		wsDataMode: make(map[string]string),
		screen:     screenDashboard,
		dashboard:  screens.NewDashboardModel(wsNames, disabled),
		jobs:       screens.NewJobsModel(),
		clusters:   screens.NewClustersModel(),
		warehouses: screens.NewWarehousesModel(),
		identity:   screens.NewIdentityModel(),
		spinner:    sp,
		loading:    true,
		interval:   interval,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Init kicks off the first data load and starts the refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadDashboard(),
		tickAfter(m.interval),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.jobs = m.jobs.SetSize(msg.Width, msg.Height)
		m.clusters = m.clusters.SetSize(msg.Width, msg.Height)
		m.warehouses = m.warehouses.SetSize(msg.Width, msg.Height)
		m.identity = m.identity.SetSize(msg.Width, msg.Height)
		m.runDetail = m.runDetail.SetSize(msg.Width, msg.Height)
		m.dashboard = m.dashboard.SetWidth(msg.Width)

	case tickMsg:
		return m, tea.Batch(m.loadDashboard(), tickAfter(m.interval))

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case screens.DashboardLoadedMsg:
		m.lastRefreshed = time.Now()

	case screens.JobsLoadedMsg:
		if msg.FromCache {
			m.wsDataMode[msg.Workspace] = "cached"
		} else if msg.Err == nil {
			m.wsDataMode[msg.Workspace] = "live"
		}

	case screens.ClustersLoadedMsg:
		if msg.FromCache {
			m.wsDataMode[msg.Workspace] = "cached"
		} else if msg.Err == nil && m.wsDataMode[msg.Workspace] != "cached" {
			m.wsDataMode[msg.Workspace] = "live"
		}

	case screens.LoadUserDetailRequestMsg:
		p := m.providers[msg.Workspace]
		if p != nil {
			return m, screens.LoadUserDetailCmd(m.ctx, msg.UserID, p.Identity)
		}
		return m, nil

	case screens.LoadSPDetailRequestMsg:
		p := m.providers[msg.Workspace]
		if p != nil {
			return m, screens.LoadSPDetailCmd(m.ctx, msg.SPID, p.Identity)
		}
		return m, nil

	case tea.KeyMsg:
		// Global nav keys are suppressed when the identity search/popup is active.
		identitySearchFocused := m.screen == screenIdentity && (m.identity.SearchFocused() || m.identity.PopupVisible())
		if !identitySearchFocused {
			switch {
			case key.Matches(msg, keys.Quit):
				m.cancel()
				return m, tea.Quit
			case key.Matches(msg, keys.Dashboard):
				m.screen = screenDashboard
			case key.Matches(msg, keys.Jobs):
				m.screen = screenJobs
				return m, m.loadJobs()
			case key.Matches(msg, keys.Clusters):
				m.screen = screenClusters
				return m, m.loadClusters()
			case key.Matches(msg, keys.Warehouses):
				m.screen = screenWarehouses
				return m, m.loadWarehouses()
			case key.Matches(msg, keys.Identity):
				m.screen = screenIdentity
				return m, m.loadIdentity()
			case key.Matches(msg, keys.Refresh):
				return m, m.refreshCurrentScreen()
			case key.Matches(msg, keys.NextWS):
				ws := m.cfg.VisibleWorkspaces()
				if len(ws) > 0 {
					m.wsIdx = (m.wsIdx + 1) % len(ws)
				}
			case key.Matches(msg, keys.Back):
				if m.screen == screenRunDetail {
					m.screen = screenJobs
				}
			case key.Matches(msg, keys.Select):
				if m.screen == screenJobs {
					if row := m.jobs.SelectedRow(); row != nil && row.Run != nil {
						m.screen = screenRunDetail
						m.runDetail = screens.NewRunDetailModel(row.Run.RunID)
						m.runDetail = m.runDetail.SetSize(m.width, m.height)
						p := m.providers[row.Workspace]
						if p != nil {
							return m, screens.LoadRunOutputCmd(m.ctx, row.Run.RunID, p.Jobs)
						}
					}
				}
			}
		}
	}

	// Delegate to sub-models.
	var cmd tea.Cmd
	switch m.screen {
	case screenDashboard:
		m.dashboard, cmd = m.dashboard.Update(msg)
	case screenJobs:
		m.jobs, cmd = m.jobs.Update(msg)
	case screenClusters:
		m.clusters, cmd = m.clusters.Update(msg)
	case screenWarehouses:
		m.warehouses, cmd = m.warehouses.Update(msg)
	case screenIdentity:
		m.identity, cmd = m.identity.Update(msg)
	case screenRunDetail:
		m.runDetail, cmd = m.runDetail.Update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	header := m.renderHeader()
	var body string
	switch m.screen {
	case screenDashboard:
		body = m.dashboard.View()
	case screenJobs:
		body = m.jobs.View()
	case screenClusters:
		body = m.clusters.View()
	case screenWarehouses:
		body = m.warehouses.View()
	case screenIdentity:
		body = m.identity.View()
	case screenRunDetail:
		body = m.runDetail.View()
	}
	help := m.renderHelp()
	return header + "\n" + body + "\n" + help
}

// renderHeader renders the top status bar.
func (m Model) renderHeader() string {
	ws := m.currentWorkspaceName()
	wsBar := styleWorkspaceBar.Render(fmt.Sprintf("◀  %s  ▶", ws))
	var status string
	if m.lastRefreshed.IsZero() {
		status = m.spinner.View() + " loading…"
	} else {
		status = "last refreshed: " + m.lastRefreshed.Format("15:04:05")
	}
	right := styleStatusBar.Render(status)
	title := styleTitle.Render("dbx-dash")
	left := title + "  " + wsBar
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderHelp renders the bottom keybinding bar.
func (m Model) renderHelp() string {
	hints := []string{"1 Dashboard", "2 Jobs", "3 Clusters", "4 Warehouses", "5 Identity", "r Refresh", "w Workspace", "q Quit"}
	return styleHelp.Width(m.width).Render(strings.Join(hints, "  "))
}

func (m Model) currentWorkspaceName() string {
	ws := m.cfg.VisibleWorkspaces()
	if len(ws) == 0 {
		return "–"
	}
	name := ws[m.wsIdx%len(ws)].Label()
	switch m.wsDataMode[ws[m.wsIdx%len(ws)].Name] {
	case "cached":
		name += styleDataModeCached.Render(" - Cached")
	case "live":
		name += styleDataModeLive.Render(" - Live")
	}
	return name
}

// loadDashboard returns a Cmd that fetches summaries for all workspaces.
func (m Model) loadDashboard() tea.Cmd {
	return screens.LoadDashboardCmd(m.ctx, m.providers, m.cacheRepo)
}

func (m Model) loadJobs() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.providers))
	for ws, p := range m.providers {
		cmds = append(cmds, screens.LoadJobsCmd(m.ctx, ws, p, m.cacheRepo))
	}
	return tea.Batch(cmds...)
}

func (m Model) loadClusters() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.providers))
	for ws, p := range m.providers {
		cmds = append(cmds, screens.LoadClustersCmd(m.ctx, ws, p, m.cacheRepo))
	}
	return tea.Batch(cmds...)
}

func (m Model) loadWarehouses() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.providers))
	for ws, p := range m.providers {
		cmds = append(cmds, screens.LoadWarehousesCmd(m.ctx, ws, p))
	}
	return tea.Batch(cmds...)
}

func (m Model) loadIdentity() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.providers))
	for ws, p := range m.providers {
		cmds = append(cmds, screens.LoadIdentityCmd(m.ctx, ws, p))
	}
	return tea.Batch(cmds...)
}

func (m Model) refreshCurrentScreen() tea.Cmd {
	switch m.screen {
	case screenDashboard:
		return m.loadDashboard()
	case screenJobs:
		return m.loadJobs()
	case screenClusters:
		return m.loadClusters()
	case screenWarehouses:
		return m.loadWarehouses()
	case screenIdentity:
		return m.loadIdentity()
	}
	return nil
}

func tickAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Run starts the Bubble Tea program.
func Run(cfg *config.AppConfig, providers map[string]*databricks.WorkspaceProviders, disabled []string, cacheRepo *cache.Repository) error {
	m := NewModel(cfg, providers, disabled, cacheRepo)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

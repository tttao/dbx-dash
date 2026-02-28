package screens

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// IdentityLoadedMsg carries all identity data for one workspace.
type IdentityLoadedMsg struct {
	Workspace         string
	Groups            []databricks.Group
	Users             []databricks.IdentityUser
	ServicePrincipals []databricks.WorkspaceServicePrincipal
	Err               error
}

// LoadUserDetailRequestMsg signals that the identity screen wants to load user details.
// app.go catches this and fires LoadUserDetailCmd with the correct provider.
type LoadUserDetailRequestMsg struct {
	Workspace string
	UserID    string
}

// LoadSPDetailRequestMsg signals that the identity screen wants to load SP details.
type LoadSPDetailRequestMsg struct {
	Workspace string
	SPID      string
}

// UserDetailLoadedMsg carries the result of GetUser + GetCatalogPermissions.
type UserDetailLoadedMsg struct {
	Detail *databricks.UserDetail
	Err    error
}

// SPDetailLoadedMsg carries the result of GetServicePrincipal + GetCatalogPermissions.
type SPDetailLoadedMsg struct {
	Detail *databricks.SPDetail
	Err    error
}

// LoadIdentityCmd fetches groups, users, and service principals for a workspace.
func LoadIdentityCmd(ctx context.Context, ws string, p *databricks.WorkspaceProviders) tea.Cmd {
	return func() tea.Msg {
		groups, err := p.Identity.ListGroups(ctx)
		if err != nil {
			return IdentityLoadedMsg{Workspace: ws, Err: err}
		}
		users, err := p.Identity.ListUsers(ctx)
		if err != nil {
			return IdentityLoadedMsg{Workspace: ws, Err: err}
		}
		sps, err := p.Identity.ListServicePrincipals(ctx)
		if err != nil {
			return IdentityLoadedMsg{Workspace: ws, Err: err}
		}
		return IdentityLoadedMsg{
			Workspace:         ws,
			Groups:            groups,
			Users:             users,
			ServicePrincipals: sps,
		}
	}
}

// LoadUserDetailCmd fetches full SCIM details and Unity Catalog permissions for a user.
func LoadUserDetailCmd(ctx context.Context, userID string, p databricks.IdentityProvider) tea.Cmd {
	return func() tea.Msg {
		detail, err := p.GetUser(ctx, userID)
		if err != nil {
			return UserDetailLoadedMsg{Err: err}
		}
		if detail != nil {
			catPerms, _ := p.GetCatalogPermissions(ctx, detail.UserName)
			detail.CatalogPermissions = catPerms
		}
		return UserDetailLoadedMsg{Detail: detail}
	}
}

// LoadSPDetailCmd fetches full SCIM details, Unity Catalog permissions, and workspace ACL for a service principal.
func LoadSPDetailCmd(ctx context.Context, spID string, p databricks.IdentityProvider) tea.Cmd {
	return func() tea.Msg {
		detail, err := p.GetServicePrincipal(ctx, spID)
		if err != nil {
			return SPDetailLoadedMsg{Err: err}
		}
		if detail != nil {
			// Unity Catalog identifies SPs by their applicationId string.
			catPerms, _ := p.GetCatalogPermissions(ctx, detail.ApplicationID)
			detail.CatalogPermissions = catPerms
			// Workspace-level permission ACL (soft-fail if API unsupported).
			detail.AccessControl, _ = p.GetSPPermissions(ctx, spID)
		}
		return SPDetailLoadedMsg{Detail: detail}
	}
}

// wsIdentityData holds identity data for a single workspace.
type wsIdentityData struct {
	name   string
	groups []databricks.Group
	users  []databricks.IdentityUser
	sps    []databricks.WorkspaceServicePrincipal
	err    error
}

// treeItem is one rendered line in the identity tree.
type treeItem struct {
	line        string
	isWorkspace bool   // workspace root row
	wsName      string // always set (workspace for user/SP: their workspace)
	isGroup     bool   // group header row
	groupKey    string // "wsName:groupID", set when isGroup == true
	isUser      bool   // user leaf row
	isSP        bool   // service principal leaf row
	entityID    string // SCIM ID for user or SP (used for transitive group lookup)
}

// popupContent holds pre-rendered text for the detail popup.
type popupContent struct {
	title string
	body  string
}

// IdentityModel is the Bubble Tea model for the identity screen.
type IdentityModel struct {
	workspaces []wsIdentityData // ordered list, one entry per workspace
	wsIndex    map[string]int   // workspace name -> index in workspaces slice

	expanded map[string]bool // key -> expanded
	search   textinput.Model
	viewport viewport.Model
	items    []treeItem
	cursor   int
	loading  bool
	err      error
	width    int
	height   int

	treeView bool // main view: false = list (default), true = group hierarchy tree

	popup           *popupContent
	popupLoading    bool
	popupWsName     string // workspace of entity shown in popup (for transitive lookup)
	popupEntityID   string // stored for refresh
	popupEntityType string // "user" | "sp"
	popupGroupTree  bool   // popup group section: false = flat, true = ancestry tree
	popupUserDetail *databricks.UserDetail
	popupSPDetail   *databricks.SPDetail
}

func NewIdentityModel() IdentityModel {
	ti := textinput.New()
	ti.Placeholder = "filter groups, users, service principals…"
	ti.CharLimit = 80

	vp := viewport.New(80, 20)

	return IdentityModel{
		wsIndex:  make(map[string]int),
		expanded: make(map[string]bool),
		search:   ti,
		viewport: vp,
	}
}

// SearchFocused returns true when the search input has keyboard focus.
func (m IdentityModel) SearchFocused() bool {
	return m.search.Focused()
}

// PopupVisible returns true when the detail popup is open or loading.
func (m IdentityModel) PopupVisible() bool {
	return m.popup != nil || m.popupLoading
}

func (m IdentityModel) SetSize(w, h int) IdentityModel {
	m.width = w
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h - 6
	m.search.Width = w - 12
	m = m.rebuild()
	return m
}

func (m IdentityModel) Update(msg tea.Msg) (IdentityModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch v := msg.(type) {
	case IdentityLoadedMsg:
		m.loading = false
		data := wsIdentityData{
			name: v.Workspace,
			err:  v.Err,
		}
		if v.Err == nil {
			data.groups = sortedGroups(v.Groups)
			data.users = sortedUsers(v.Users)
			data.sps = sortedSPs(v.ServicePrincipals)
		}
		if idx, ok := m.wsIndex[v.Workspace]; ok {
			m.workspaces[idx] = data
		} else {
			m.wsIndex[v.Workspace] = len(m.workspaces)
			m.workspaces = append(m.workspaces, data)
			m.expanded["ws:"+v.Workspace] = true
		}
		m = m.rebuild()

	case UserDetailLoadedMsg:
		m.popupLoading = false
		if v.Err != nil {
			m.popup = &popupContent{title: "Error", body: styleIdentityErr.Render(v.Err.Error())}
			m.popupUserDetail = nil
		} else if v.Detail != nil {
			m.popupUserDetail = v.Detail
			m.popupSPDetail = nil
			m = m.rerenderPopup()
		}

	case SPDetailLoadedMsg:
		m.popupLoading = false
		if v.Err != nil {
			m.popup = &popupContent{title: "Error", body: styleIdentityErr.Render(v.Err.Error())}
			m.popupSPDetail = nil
		} else if v.Detail != nil {
			m.popupSPDetail = v.Detail
			m.popupUserDetail = nil
			m = m.rerenderPopup()
		}

	case tea.KeyMsg:
		// Esc: close popup first, then clear search.
		if v.String() == "esc" {
			if m.popup != nil || m.popupLoading {
				m.popup = nil
				m.popupLoading = false
				return m, nil
			}
			if m.search.Focused() {
				m.search.Blur()
				m.search.SetValue("")
				m = m.rebuild()
				return m, nil
			}
		}

		// While popup is showing, handle t/r and close keys.
		if m.popup != nil || m.popupLoading {
			switch v.String() {
			case "t":
				if !m.popupLoading {
					m.popupGroupTree = !m.popupGroupTree
					m = m.rerenderPopup()
				}
			case "r":
				if !m.popupLoading && m.popupEntityID != "" {
					m.popupLoading = true
					m.popup = nil
					m.popupUserDetail = nil
					m.popupSPDetail = nil
					wsName, entityID, entityType := m.popupWsName, m.popupEntityID, m.popupEntityType
					if entityType == "user" {
						return m, func() tea.Msg {
							return LoadUserDetailRequestMsg{Workspace: wsName, UserID: entityID}
						}
					}
					return m, func() tea.Msg {
						return LoadSPDetailRequestMsg{Workspace: wsName, SPID: entityID}
					}
				}
			case "esc", "enter", " ", "q":
				m.popup = nil
				m.popupLoading = false
				m.popupUserDetail = nil
				m.popupSPDetail = nil
				m.popupEntityID = ""
				m.popupEntityType = ""
			}
			return m, nil
		}

		switch v.String() {
		case "/":
			if !m.search.Focused() {
				m.search.Focus()
				return m, textinput.Blink
			}
		case "up", "k":
			if !m.search.Focused() {
				if m.cursor > 0 {
					m.cursor--
					m = m.ensureCursorVisible()
				}
				return m, nil
			}
		case "down", "j":
			if !m.search.Focused() {
				if m.cursor < len(m.items)-1 {
					m.cursor++
					m = m.ensureCursorVisible()
				}
				return m, nil
			}
		case "enter", " ":
			if !m.search.Focused() && m.cursor < len(m.items) {
				item := m.items[m.cursor]
				switch {
				case item.isWorkspace:
					key := "ws:" + item.wsName
					m.expanded[key] = !m.expanded[key]
					m = m.rebuild()
				case item.isGroup:
					m.expanded[item.groupKey] = !m.expanded[item.groupKey]
					m = m.rebuild()
				case item.isUser:
					m.popupLoading = true
					m.popupWsName = item.wsName
					m.popupEntityID = item.entityID
					m.popupEntityType = "user"
					m.popupGroupTree = false
					return m, func() tea.Msg {
						return LoadUserDetailRequestMsg{Workspace: item.wsName, UserID: item.entityID}
					}
				case item.isSP:
					m.popupLoading = true
					m.popupWsName = item.wsName
					m.popupEntityID = item.entityID
					m.popupEntityType = "sp"
					m.popupGroupTree = false
					return m, func() tea.Msg {
						return LoadSPDetailRequestMsg{Workspace: item.wsName, SPID: item.entityID}
					}
				}
				return m, nil
			}
		// Toggle list/tree view.
		case "t":
			if !m.search.Focused() {
				m.treeView = !m.treeView
				m = m.rebuild()
				return m, nil
			}
		}
	}

	// Forward key events to search input when focused.
	if m.search.Focused() {
		var cmd tea.Cmd
		prev := m.search.Value()
		m.search, cmd = m.search.Update(msg)
		if m.search.Value() != prev {
			m.cursor = 0
			m = m.rebuild()
		}
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// wsGroups returns the groups slice for the workspace currently shown in the popup.
func (m IdentityModel) wsGroups() []databricks.Group {
	if m.popupWsName == "" {
		return nil
	}
	idx, ok := m.wsIndex[m.popupWsName]
	if !ok {
		return nil
	}
	return m.workspaces[idx].groups
}

// wsData returns the full wsIdentityData for the workspace currently shown in the popup.
func (m IdentityModel) wsData() *wsIdentityData {
	if m.popupWsName == "" {
		return nil
	}
	idx, ok := m.wsIndex[m.popupWsName]
	if !ok {
		return nil
	}
	return &m.workspaces[idx]
}

// computeUserSPAccess returns SPs reachable by userID via shared group membership.
// Zero extra API calls — uses already-loaded workspace group/SP data.
func computeUserSPAccess(userID string, ws wsIdentityData) []databricks.SPUserAccess {
	if userID == "" || len(ws.sps) == 0 || len(ws.groups) == 0 {
		return nil
	}

	// Build member→groups map and SP set.
	memberToGroupIDs := make(map[string][]string, len(ws.groups))
	groupNameByID := make(map[string]string, len(ws.groups))
	for _, g := range ws.groups {
		groupNameByID[g.ID] = g.DisplayName
		for _, m := range g.Members {
			memberToGroupIDs[m.ID] = append(memberToGroupIDs[m.ID], g.ID)
		}
	}
	spByID := make(map[string]databricks.WorkspaceServicePrincipal, len(ws.sps))
	for _, sp := range ws.sps {
		spByID[sp.ID] = sp
	}

	// BFS: collect all group IDs the user belongs to (direct + transitive).
	userGroupIDs := make(map[string]bool)
	queue := append([]string{}, memberToGroupIDs[userID]...)
	for _, gid := range queue {
		userGroupIDs[gid] = true
	}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, parentGID := range memberToGroupIDs[curr] {
			if !userGroupIDs[parentGID] {
				userGroupIDs[parentGID] = true
				queue = append(queue, parentGID)
			}
		}
	}

	// For each SP, find groups that contain both the user and the SP.
	spAccess := make(map[string]*databricks.SPUserAccess)
	for _, g := range ws.groups {
		if !userGroupIDs[g.ID] {
			continue
		}
		for _, m := range g.Members {
			sp, isSP := spByID[m.ID]
			if !isSP {
				continue
			}
			if acc, exists := spAccess[sp.ID]; exists {
				acc.ViaGroups = append(acc.ViaGroups, groupNameByID[g.ID])
			} else {
				spAccess[sp.ID] = &databricks.SPUserAccess{
					SPID:        sp.ID,
					DisplayName: sp.DisplayName,
					ViaGroups:   []string{groupNameByID[g.ID]},
				}
			}
		}
	}

	result := make([]databricks.SPUserAccess, 0, len(spAccess))
	for _, acc := range spAccess {
		sort.Strings(acc.ViaGroups)
		result = append(result, *acc)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].DisplayName) < strings.ToLower(result[j].DisplayName)
	})
	return result
}

func (m IdentityModel) rebuild() IdentityModel {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if m.treeView {
		m.items = buildGroupTreeWorkspaceItems(m.workspaces, m.expanded, q)
	} else {
		m.items = buildWorkspaceItems(m.workspaces, m.expanded, q)
	}
	lines := make([]string, len(m.items))
	for i, item := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = styleIdentityCursor.Render(">") + " "
		}
		lines[i] = prefix + item.line
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	return m
}

func (m IdentityModel) ensureCursorVisible() IdentityModel {
	if m.cursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.cursor)
	} else if m.cursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.cursor - m.viewport.Height + 1)
	}
	m = m.rebuild()
	return m
}

func (m IdentityModel) View() string {
	if m.loading {
		return "  Loading identity data…\n"
	}
	if m.err != nil {
		return styleIdentityErr.Render("  Error: "+m.err.Error()) + "\n"
	}

	var searchLine string
	if m.search.Focused() {
		searchLine = styleIdentitySearchActive.Render(" / ") + " " + m.search.View()
	} else {
		searchLine = styleIdentitySearchHint.Render(" / ") + " " + m.search.View()
	}

	var totalGroups, totalUsers, totalSPs int
	for _, ws := range m.workspaces {
		totalGroups += len(ws.groups)
		totalUsers += len(ws.users)
		totalSPs += len(ws.sps)
	}
	stats := styleIdentityMuted.Render(fmt.Sprintf(
		"  %d workspaces  %d groups  %d users  %d service principals",
		len(m.workspaces), totalGroups, totalUsers, totalSPs,
	))

	viewMode := styleIdentityMuted.Render("[list]")
	if m.treeView {
		viewMode = styleIdentitySection.Render("[tree]")
	}

	if m.popupLoading || m.popup != nil {
		popupView := m.renderPopup()
		help := styleIdentityMuted.Render("  t groups   r refresh   esc / enter  close")
		return searchLine + "\n" + stats + "  " + viewMode + "\n" + popupView + "\n" + help
	}

	help := styleIdentityMuted.Render("  / search   ↑↓ navigate   enter expand/details   t toggle view   esc clear")
	return searchLine + "\n" + stats + "  " + viewMode + "\n" + m.viewport.View() + "\n" + help
}

func (m IdentityModel) renderPopup() string {
	popupWidth := 68
	if m.width > 0 && popupWidth > m.width-4 {
		popupWidth = m.width - 4
	}

	var inner string
	if m.popupLoading {
		inner = styleIdentityMuted.Render("Loading details…")
	} else if m.popup != nil {
		title := styleIdentityWSName.Render(m.popup.title)
		inner = title + "\n\n" + m.popup.body
	}

	box := stylePopupBox.Width(popupWidth).Render(inner)

	leftPad := (m.width - lipgloss.Width(box)) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", leftPad) + line
	}
	return strings.Join(lines, "\n")
}

// transitiveGroupLookup computes direct and indirect (transitive) group memberships
// for an entity (user or SP) identified by its SCIM ID, using the workspace groups.
func transitiveGroupLookup(entityID string, allGroups []databricks.Group) (direct, indirect []string) {
	if entityID == "" || len(allGroups) == 0 {
		return
	}

	// Build lookup structures.
	groupNameByID := make(map[string]string, len(allGroups))
	memberToGroupIDs := make(map[string][]string)
	for _, g := range allGroups {
		groupNameByID[g.ID] = g.DisplayName
		for _, m := range g.Members {
			memberToGroupIDs[m.ID] = append(memberToGroupIDs[m.ID], g.ID)
		}
	}

	// BFS starting from entityID.
	visited := make(map[string]bool)
	directGroupIDs := memberToGroupIDs[entityID]
	for _, gid := range directGroupIDs {
		if !visited[gid] {
			visited[gid] = true
			direct = append(direct, groupNameByID[gid])
		}
	}

	// Expand transitively: find groups that contain the direct groups, etc.
	queue := append([]string{}, directGroupIDs...)
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, parentGID := range memberToGroupIDs[curr] {
			if !visited[parentGID] {
				visited[parentGID] = true
				indirect = append(indirect, groupNameByID[parentGID])
				queue = append(queue, parentGID)
			}
		}
	}

	sort.Strings(direct)
	sort.Strings(indirect)
	return
}

// renderUserDetailPopup formats user detail for the popup.
// groupSection is a pre-rendered string for the group membership section.
func renderUserDetailPopup(u *databricks.UserDetail, groupSection string) string {
	var b strings.Builder

	status := "active"
	if !u.Active {
		status = styleIdentityMuted.Render("inactive")
	}

	b.WriteString(popupRow("Username", u.UserName))
	if u.DisplayName != "" && u.DisplayName != u.UserName {
		b.WriteString(popupRow("Name", u.DisplayName))
	}
	b.WriteString(popupRow("SCIM ID", u.ID))
	b.WriteString(popupRow("Status", status))
	b.WriteString("\n")

	b.WriteString(popupSection("WORKSPACE ENTITLEMENTS", u.Entitlements, "(none)"))
	b.WriteString("\n")
	b.WriteString(popupSection("ROLES", u.Roles, "(none)"))
	b.WriteString("\n")

	b.WriteString(groupSection)
	b.WriteString("\n")

	b.WriteString(renderCatalogPerms(u.CatalogPermissions))
	b.WriteString("\n")
	b.WriteString(renderUserSPAccess(u.SPAccess))
	return b.String()
}

// renderSPDetailPopup formats service principal detail for the popup.
// groupSection is a pre-rendered string for the group membership section.
func renderSPDetailPopup(sp *databricks.SPDetail, groupSection string) string {
	var b strings.Builder

	status := "active"
	if !sp.Active {
		status = styleIdentityMuted.Render("inactive")
	}

	b.WriteString(popupRow("Name", sp.DisplayName))
	if sp.ApplicationID != "" {
		b.WriteString(popupRow("App ID", sp.ApplicationID))
	}
	b.WriteString(popupRow("SCIM ID", sp.ID))
	b.WriteString(popupRow("Status", status))
	b.WriteString("\n")

	b.WriteString(popupSection("WORKSPACE ENTITLEMENTS", sp.Entitlements, "(none)"))
	b.WriteString("\n")
	b.WriteString(popupSection("ROLES", sp.Roles, "(none)"))
	b.WriteString("\n")

	b.WriteString(groupSection)
	b.WriteString("\n")

	b.WriteString(renderCatalogPerms(sp.CatalogPermissions))
	b.WriteString("\n")
	b.WriteString(renderSPWhoHasAccess(sp.AccessControl))
	return b.String()
}

func popupGroupSection(direct, indirect []string) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render("GROUP MEMBERSHIPS") + "\n")
	if len(direct) == 0 && len(indirect) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render("(no group memberships)") + "\n")
		return b.String()
	}
	for _, g := range direct {
		b.WriteString("  " + styleIdentityUser.Render("●") + " " + g + "\n")
	}
	if len(indirect) > 0 {
		b.WriteString("  " + styleIdentityMuted.Render("via transitive membership:") + "\n")
		for _, g := range indirect {
			b.WriteString("  " + styleIdentityMuted.Render("◌") + " " + styleIdentityMuted.Render(g) + "\n")
		}
	}
	return b.String()
}

func renderCatalogPerms(perms []databricks.CatalogPermission) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render("UNITY CATALOG PERMISSIONS") + "\n")
	switch {
	case perms == nil:
		b.WriteString("  " + styleIdentityMuted.Render("(Unity Catalog not available)") + "\n")
	case len(perms) == 0:
		b.WriteString("  " + styleIdentityMuted.Render("(no catalog grants)") + "\n")
	default:
		for _, p := range perms {
			b.WriteString("  " + styleIdentitySP.Render(p.CatalogName) + "\n")
			for _, priv := range p.Privileges {
				b.WriteString("    " + styleIdentityUser.Render("●") + " " + priv + "\n")
			}
		}
	}
	return b.String()
}

func renderSPWhoHasAccess(acl []databricks.SPAccessEntry) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render("WHO HAS ACCESS") + "\n")
	if acl == nil {
		b.WriteString("  " + styleIdentityMuted.Render("(workspace permissions not available)") + "\n")
		return b.String()
	}
	if len(acl) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render("(no explicit workspace permissions)") + "\n")
		return b.String()
	}
	for _, e := range acl {
		level := styleIdentityMuted.Render("[" + strings.ToLower(e.Level) + "]")
		if e.UserName != "" {
			b.WriteString("  " + styleIdentityUser.Render("●") + " " + e.UserName + "  " + level + "\n")
		} else if e.GroupName != "" {
			b.WriteString("  " + styleIdentityUser.Render("●") + " " + styleIdentityGroupName.Render(e.GroupName) + "  " + level + styleIdentityMuted.Render(" (group)") + "\n")
		}
	}
	return b.String()
}

func renderUserSPAccess(access []databricks.SPUserAccess) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render("SERVICE PRINCIPAL ACCESS") + "\n")
	if len(access) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render("(no service principal co-memberships)") + "\n")
		return b.String()
	}
	for _, a := range access {
		b.WriteString("  " + styleIdentityUser.Render("●") + " " + styleIdentitySP.Render(a.DisplayName) + "\n")
		b.WriteString("      " + styleIdentityMuted.Render("via: "+strings.Join(a.ViaGroups, ", ")) + "\n")
	}
	return b.String()
}

func popupRow(label, value string) string {
	return styleIdentityMuted.Render(fmt.Sprintf("%-12s", label+":")) + "  " + value + "\n"
}

func popupSection(title string, items []string, empty string) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render(title) + "\n")
	if len(items) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render(empty) + "\n")
	} else {
		for _, item := range items {
			b.WriteString("  " + styleIdentityUser.Render("●") + " " + item + "\n")
		}
	}
	return b.String()
}

// buildWorkspaceItems produces the flat tree item list, with each workspace as a root node.
func buildWorkspaceItems(
	workspaces []wsIdentityData,
	expanded map[string]bool,
	query string,
) []treeItem {
	var items []treeItem

	for _, ws := range workspaces {
		wsKey := "ws:" + ws.name
		isExpanded := expanded[wsKey]

		arrow := "►"
		if isExpanded {
			arrow = "▼"
		}
		wsLabel := styleIdentityWSArrow.Render(arrow) + " " +
			styleIdentityWSName.Render(ws.name)
		if ws.err != nil {
			wsLabel += "  " + styleIdentityErr.Render("(error: "+ws.err.Error()+")")
		}
		items = append(items, treeItem{
			line:        wsLabel,
			isWorkspace: true,
			wsName:      ws.name,
		})

		if !isExpanded {
			continue
		}

		// Build ID-based lookups: SCIM $type/$ref are unreliable for any member kind.
		groupByID := make(map[string]databricks.Group, len(ws.groups))
		for _, g := range ws.groups {
			groupByID[g.ID] = g
		}
		userByID := make(map[string]databricks.IdentityUser, len(ws.users))
		for _, u := range ws.users {
			userByID[u.ID] = u
		}
		spByID := make(map[string]databricks.WorkspaceServicePrincipal, len(ws.sps))
		for _, sp := range ws.sps {
			spByID[sp.ID] = sp
		}

		// Groups: all groups shown flat (tree view handles hierarchy).
		matchedGroups := filteredGroups(ws.groups, query)
		if len(matchedGroups) > 0 {
			items = append(items, treeItem{line: styleIdentitySection.Render("    GROUPS")})
			for _, g := range matchedGroups {
				groupKey := ws.name + ":" + g.ID
				grpExpanded := expanded[groupKey]
				arrow := "►"
				if grpExpanded {
					arrow = "▼"
				}
				label := "    " + styleIdentityGroupArrow.Render(arrow) + " " +
					styleIdentityGroupName.Render(g.DisplayName) + " " +
					styleIdentityMuted.Render(fmt.Sprintf("(%d)", len(g.Members)))
				items = append(items, treeItem{line: label, isGroup: true, groupKey: groupKey})
				if grpExpanded {
					for i, mem := range g.Members {
						displayName := mem.DisplayName
						memberType := mem.Type
						if child, ok := groupByID[mem.ID]; ok {
							memberType = "Group"
							if displayName == "" {
								displayName = child.DisplayName
							}
						} else if u, ok := userByID[mem.ID]; ok {
							memberType = "User"
							if displayName == "" {
								displayName = u.UserName
							}
						} else if sp, ok := spByID[mem.ID]; ok {
							memberType = "ServicePrincipal"
							if displayName == "" {
								displayName = sp.DisplayName
							}
						}
						if query != "" && !strings.Contains(strings.ToLower(displayName), query) {
							continue
						}
						isLast := i == len(g.Members)-1
						branch := "├"
						if isLast {
							branch = "└"
						}
						line := "      " + styleIdentityMuted.Render(branch+" ") +
							memberNameStyle(memberType).Render(displayName) + "  " +
							memberTypeTag(memberType)
						it := treeItem{line: line}
						switch memberType {
						case "User":
							it.isUser, it.entityID, it.wsName = true, mem.ID, ws.name
						case "ServicePrincipal":
							it.isSP, it.entityID, it.wsName = true, mem.ID, ws.name
						}
						items = append(items, it)
					}
				}
			}
		}

		// Users.
		matchedUsers := filteredUsers(ws.users, query)
		if len(matchedUsers) > 0 {
			items = append(items, treeItem{
				line: styleIdentitySection.Render(fmt.Sprintf("    USERS (%d)", len(matchedUsers))),
			})
			for _, u := range matchedUsers {
				name := u.UserName
				if u.DisplayName != "" && u.DisplayName != u.UserName {
					name = u.DisplayName + "  " + styleIdentityMuted.Render("<"+u.UserName+">")
				}
				inactive := ""
				if !u.Active {
					inactive = "  " + styleIdentityMuted.Render("[inactive]")
				}
				items = append(items, treeItem{
					line:     "      " + styleIdentityUser.Render(name) + inactive,
					isUser:   true,
					entityID: u.ID,
					wsName:   ws.name,
				})
			}
		}

		// Service principals.
		matchedSPs := filteredSPs(ws.sps, query)
		if len(matchedSPs) > 0 {
			items = append(items, treeItem{
				line: styleIdentitySection.Render(fmt.Sprintf("    SERVICE PRINCIPALS (%d)", len(matchedSPs))),
			})
			for _, sp := range matchedSPs {
				inactive := ""
				if !sp.Active {
					inactive = "  " + styleIdentityMuted.Render("[inactive]")
				}
				appID := ""
				if sp.ApplicationID != "" {
					appID = "  " + styleIdentityMuted.Render("app:"+sp.ApplicationID)
				}
				items = append(items, treeItem{
					line:     "      " + styleIdentitySP.Render(sp.DisplayName) + appID + inactive,
					isSP:     true,
					entityID: sp.ID,
					wsName:   ws.name,
				})
			}
		}

		if isExpanded && len(matchedGroups) == 0 && len(matchedUsers) == 0 && len(matchedSPs) == 0 && query != "" {
			items = append(items, treeItem{
				line: styleIdentityMuted.Render(`    no results for "` + query + `"`),
			})
		}
	}

	return items
}

func filteredGroups(groups []databricks.Group, query string) []databricks.Group {
	if query == "" {
		return groups
	}
	var out []databricks.Group
	for _, g := range groups {
		if groupMatches(g, query) {
			out = append(out, g)
		}
	}
	return out
}

func filteredUsers(users []databricks.IdentityUser, query string) []databricks.IdentityUser {
	if query == "" {
		return users
	}
	var out []databricks.IdentityUser
	for _, u := range users {
		if strings.Contains(strings.ToLower(u.UserName), query) ||
			strings.Contains(strings.ToLower(u.DisplayName), query) {
			out = append(out, u)
		}
	}
	return out
}

func filteredSPs(sps []databricks.WorkspaceServicePrincipal, query string) []databricks.WorkspaceServicePrincipal {
	if query == "" {
		return sps
	}
	var out []databricks.WorkspaceServicePrincipal
	for _, sp := range sps {
		if strings.Contains(strings.ToLower(sp.DisplayName), query) ||
			strings.Contains(strings.ToLower(sp.ApplicationID), query) {
			out = append(out, sp)
		}
	}
	return out
}

func groupMatches(g databricks.Group, query string) bool {
	if strings.Contains(strings.ToLower(g.DisplayName), query) {
		return true
	}
	for _, m := range g.Members {
		if strings.Contains(strings.ToLower(m.DisplayName), query) {
			return true
		}
	}
	return false
}

func sortedGroups(gs []databricks.Group) []databricks.Group {
	sort.Slice(gs, func(i, j int) bool {
		return strings.ToLower(gs[i].DisplayName) < strings.ToLower(gs[j].DisplayName)
	})
	return gs
}

func sortedUsers(us []databricks.IdentityUser) []databricks.IdentityUser {
	sort.Slice(us, func(i, j int) bool {
		return strings.ToLower(us[i].UserName) < strings.ToLower(us[j].UserName)
	})
	return us
}

func sortedSPs(sps []databricks.WorkspaceServicePrincipal) []databricks.WorkspaceServicePrincipal {
	sort.Slice(sps, func(i, j int) bool {
		return strings.ToLower(sps[i].DisplayName) < strings.ToLower(sps[j].DisplayName)
	})
	return sps
}

func memberTypeTag(t string) string {
	switch t {
	case "User":
		return styleIdentityTagUser.Render("[user]")
	case "ServicePrincipal":
		return styleIdentityTagSP.Render("[sp]")
	case "Group":
		return styleIdentityTagGroup.Render("[group]")
	default:
		return styleIdentityMuted.Render("[" + strings.ToLower(t) + "]")
	}
}

func memberNameStyle(t string) lipgloss.Style {
	switch t {
	case "User":
		return styleIdentityUser
	case "ServicePrincipal":
		return styleIdentitySP
	case "Group":
		return styleIdentityGroupName
	default:
		return styleIdentityMuted
	}
}

var (
	styleIdentityWSArrow      = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	styleIdentityWSName       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	styleIdentitySection      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleIdentityGroupName    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleIdentityGroupArrow   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleIdentityUser         = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleIdentitySP           = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	styleIdentityMuted        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleIdentityCursor       = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	styleIdentityErr          = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleIdentitySearchActive = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	styleIdentitySearchHint   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleIdentityTagUser      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleIdentityTagSP        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleIdentityTagGroup     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stylePopupBox             = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("33")).
					Padding(1, 2)
)

// ---------------------------------------------------------------------------
// rerenderPopup re-builds popup content from stored raw detail.
// Called when data arrives or when the user toggles the group tree view.
// ---------------------------------------------------------------------------

func (m IdentityModel) rerenderPopup() IdentityModel {
	if m.popupUserDetail != nil {
		u := m.popupUserDetail
		direct, indirect := transitiveGroupLookup(u.ID, m.wsGroups())
		if len(direct) == 0 && len(u.Groups) > 0 {
			direct = u.Groups
		}
		var groupSection string
		if m.popupGroupTree {
			groupSection = renderGroupAncestryTree(u.ID, m.wsGroups())
		} else {
			groupSection = popupGroupSection(direct, indirect)
		}
		if wsD := m.wsData(); wsD != nil {
			u.SPAccess = computeUserSPAccess(u.ID, *wsD)
		}
		m.popup = &popupContent{
			title: "User: " + u.UserName,
			body:  renderUserDetailPopup(u, groupSection),
		}
	} else if m.popupSPDetail != nil {
		sp := m.popupSPDetail
		direct, indirect := transitiveGroupLookup(sp.ID, m.wsGroups())
		if len(direct) == 0 && len(sp.Groups) > 0 {
			direct = sp.Groups
		}
		var groupSection string
		if m.popupGroupTree {
			groupSection = renderGroupAncestryTree(sp.ID, m.wsGroups())
		} else {
			groupSection = popupGroupSection(direct, indirect)
		}
		m.popup = &popupContent{
			title: "Service principal: " + sp.DisplayName,
			body:  renderSPDetailPopup(sp, groupSection),
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Main tree view: group hierarchy (groups at root, members as children)
// ---------------------------------------------------------------------------

func buildGroupTreeWorkspaceItems(workspaces []wsIdentityData, expanded map[string]bool, query string) []treeItem {
	var items []treeItem
	for _, ws := range workspaces {
		wsKey := "ws:" + ws.name
		isExpanded := expanded[wsKey]
		arrow := "►"
		if isExpanded {
			arrow = "▼"
		}
		wsLabel := styleIdentityWSArrow.Render(arrow) + " " + styleIdentityWSName.Render(ws.name)
		if ws.err != nil {
			wsLabel += "  " + styleIdentityErr.Render("(error: "+ws.err.Error()+")")
		}
		items = append(items, treeItem{line: wsLabel, isWorkspace: true, wsName: ws.name})
		if !isExpanded {
			continue
		}
		items = append(items, buildGroupTreeItems(ws, expanded, query)...)
	}
	return items
}

func buildGroupTreeItems(ws wsIdentityData, expanded map[string]bool, query string) []treeItem {
	if ws.err != nil {
		return nil
	}

	groupByID := make(map[string]databricks.Group, len(ws.groups))
	for _, g := range ws.groups {
		groupByID[g.ID] = g
	}
	userByID := make(map[string]databricks.IdentityUser, len(ws.users))
	for _, u := range ws.users {
		userByID[u.ID] = u
	}
	spByID := make(map[string]databricks.WorkspaceServicePrincipal, len(ws.sps))
	for _, sp := range ws.sps {
		spByID[sp.ID] = sp
	}

	// Use ID-based lookup: SCIM $type/$ref are unreliable for group members.
	childGroupIDs := make(map[string]bool)
	for _, g := range ws.groups {
		for _, m := range g.Members {
			if _, isGroup := groupByID[m.ID]; isGroup {
				childGroupIDs[m.ID] = true
			}
		}
	}

	var rootGroups []databricks.Group
	for _, g := range ws.groups {
		if !childGroupIDs[g.ID] {
			rootGroups = append(rootGroups, g)
		}
	}
	sort.Slice(rootGroups, func(i, j int) bool {
		return strings.ToLower(rootGroups[i].DisplayName) < strings.ToLower(rootGroups[j].DisplayName)
	})

	inGroup := make(map[string]bool)
	for _, g := range ws.groups {
		for _, m := range g.Members {
			if _, isGroup := groupByID[m.ID]; !isGroup {
				inGroup[m.ID] = true
			}
		}
	}

	var items []treeItem
	for _, g := range rootGroups {
		if query != "" && !groupMatchesDeep(g, groupByID, query, map[string]bool{}) {
			continue
		}
		items = append(items, renderGroupTreeNode(ws.name, g, groupByID, userByID, spByID, expanded, query, "    ", map[string]bool{})...)
	}

	var unaffUsers []databricks.IdentityUser
	for _, u := range ws.users {
		if !inGroup[u.ID] {
			if query == "" || strings.Contains(strings.ToLower(u.UserName), query) || strings.Contains(strings.ToLower(u.DisplayName), query) {
				unaffUsers = append(unaffUsers, u)
			}
		}
	}
	var unaffSPs []databricks.WorkspaceServicePrincipal
	for _, sp := range ws.sps {
		if !inGroup[sp.ID] {
			if query == "" || strings.Contains(strings.ToLower(sp.DisplayName), query) || strings.Contains(strings.ToLower(sp.ApplicationID), query) {
				unaffSPs = append(unaffSPs, sp)
			}
		}
	}

	if len(unaffUsers)+len(unaffSPs) > 0 {
		items = append(items, treeItem{line: styleIdentityMuted.Render("    (unaffiliated)")})
		for _, u := range unaffUsers {
			name := u.UserName
			if u.DisplayName != "" && u.DisplayName != u.UserName {
				name = u.DisplayName + "  " + styleIdentityMuted.Render("<"+u.UserName+">")
			}
			inactive := ""
			if !u.Active {
				inactive = "  " + styleIdentityMuted.Render("[inactive]")
			}
			items = append(items, treeItem{
				line: "      " + styleIdentityUser.Render(name) + inactive,
				isUser: true, entityID: u.ID, wsName: ws.name,
			})
		}
		for _, sp := range unaffSPs {
			inactive := ""
			if !sp.Active {
				inactive = "  " + styleIdentityMuted.Render("[inactive]")
			}
			appID := ""
			if sp.ApplicationID != "" {
				appID = "  " + styleIdentityMuted.Render("app:"+sp.ApplicationID)
			}
			items = append(items, treeItem{
				line: "      " + styleIdentitySP.Render(sp.DisplayName) + appID + inactive,
				isSP: true, entityID: sp.ID, wsName: ws.name,
			})
		}
	}
	return items
}

func renderGroupTreeNode(
	wsName string,
	g databricks.Group,
	groupByID map[string]databricks.Group,
	userByID map[string]databricks.IdentityUser,
	spByID map[string]databricks.WorkspaceServicePrincipal,
	expanded map[string]bool,
	query string,
	baseIndent string,
	visited map[string]bool,
) []treeItem {
	if visited[g.ID] {
		return []treeItem{{
			line: baseIndent + styleIdentityMuted.Render("↻ "+g.DisplayName+" (cyclic ref)"),
		}}
	}
	visited[g.ID] = true
	defer delete(visited, g.ID)

	groupKey := wsName + ":" + g.ID
	isExpanded := expanded[groupKey]
	arrow := "►"
	if isExpanded {
		arrow = "▼"
	}
	label := baseIndent + styleIdentityGroupArrow.Render(arrow) + " " +
		styleIdentityGroupName.Render(g.DisplayName) + " " +
		styleIdentityMuted.Render(fmt.Sprintf("(%d)", len(g.Members)))

	items := []treeItem{{line: label, isGroup: true, groupKey: groupKey, wsName: wsName}}
	if !isExpanded {
		return items
	}

	childIndent := baseIndent + "  "
	for _, m := range g.Members {
		// Resolve actual type via ID lookup: SCIM $type/$ref are unreliable.
		resolvedType := m.Type
		if _, ok := groupByID[m.ID]; ok {
			resolvedType = "Group"
		} else if _, ok := userByID[m.ID]; ok {
			resolvedType = "User"
		} else if _, ok := spByID[m.ID]; ok {
			resolvedType = "ServicePrincipal"
		}

		switch resolvedType {
		case "Group":
			child := groupByID[m.ID]
			if query != "" && !groupMatchesDeep(child, groupByID, query, map[string]bool{}) {
				continue
			}
			items = append(items, renderGroupTreeNode(wsName, child, groupByID, userByID, spByID, expanded, query, childIndent, visited)...)
		case "User":
			displayName := m.DisplayName
			if displayName == "" {
				if u, ok := userByID[m.ID]; ok {
					displayName = u.UserName
				}
			}
			if query != "" && !strings.Contains(strings.ToLower(displayName), query) {
				continue
			}
			items = append(items, treeItem{
				line:     childIndent + styleIdentityUser.Render(displayName) + "  " + memberTypeTag(resolvedType),
				isUser:   true,
				entityID: m.ID,
				wsName:   wsName,
			})
		case "ServicePrincipal":
			displayName := m.DisplayName
			if displayName == "" {
				if sp, ok := spByID[m.ID]; ok {
					displayName = sp.DisplayName
				}
			}
			if query != "" && !strings.Contains(strings.ToLower(displayName), query) {
				continue
			}
			items = append(items, treeItem{
				line:     childIndent + styleIdentitySP.Render(displayName) + "  " + memberTypeTag(resolvedType),
				isSP:     true,
				entityID: m.ID,
				wsName:   wsName,
			})
		}
	}
	return items
}

func groupMatchesDeep(g databricks.Group, groupByID map[string]databricks.Group, query string, visited map[string]bool) bool {
	if visited[g.ID] {
		return false
	}
	visited[g.ID] = true
	if strings.Contains(strings.ToLower(g.DisplayName), query) {
		return true
	}
	for _, m := range g.Members {
		if strings.Contains(strings.ToLower(m.DisplayName), query) {
			return true
		}
		if child, ok := groupByID[m.ID]; ok {
			if groupMatchesDeep(child, groupByID, query, visited) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Popup group ancestry tree view
// ---------------------------------------------------------------------------

func renderGroupAncestryTree(entityID string, allGroups []databricks.Group) string {
	var b strings.Builder
	b.WriteString(styleIdentitySection.Render("GROUP MEMBERSHIPS") + "  " + styleIdentityMuted.Render("(tree)") + "\n")

	if entityID == "" || len(allGroups) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render("(no group memberships)") + "\n")
		return b.String()
	}

	groupNameByID := make(map[string]string, len(allGroups))
	memberToParents := make(map[string][]string)
	for _, g := range allGroups {
		groupNameByID[g.ID] = g.DisplayName
		for _, m := range g.Members {
			memberToParents[m.ID] = append(memberToParents[m.ID], g.ID)
		}
	}

	directIDs := memberToParents[entityID]
	if len(directIDs) == 0 {
		b.WriteString("  " + styleIdentityMuted.Render("(no group memberships)") + "\n")
		return b.String()
	}

	sort.Slice(directIDs, func(i, j int) bool {
		return groupNameByID[directIDs[i]] < groupNameByID[directIDs[j]]
	})

	for _, gid := range directIDs {
		b.WriteString("  " + styleIdentityUser.Render("●") + " " + groupNameByID[gid] + "\n")
		chain := buildAncestorChain(gid, memberToParents, groupNameByID, map[string]bool{gid: true})
		for i, ancestor := range chain {
			indent := strings.Repeat("  ", i+2)
			b.WriteString(indent + styleIdentityMuted.Render("└── "+ancestor) + "\n")
		}
	}
	return b.String()
}

func buildAncestorChain(groupID string, memberToParents map[string][]string, groupNameByID map[string]string, visited map[string]bool) []string {
	parents := memberToParents[groupID]
	if len(parents) == 0 {
		return nil
	}
	firstParent := parents[0]
	if visited[firstParent] {
		return []string{groupNameByID[firstParent] + " (cyclic)"}
	}
	visited[firstParent] = true
	chain := []string{groupNameByID[firstParent]}
	chain = append(chain, buildAncestorChain(firstParent, memberToParents, groupNameByID, visited)...)
	return chain
}

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
	"github.com/you/dbx-dash/internal/databricks"
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

// LoadSPDetailCmd fetches full SCIM details and Unity Catalog permissions for a service principal.
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

	popup        *popupContent
	popupLoading bool
	popupWsName  string // workspace of entity shown in popup (for transitive lookup)
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
		} else if v.Detail != nil {
			direct, indirect := transitiveGroupLookup(v.Detail.ID, m.wsGroups())
			// Fall back to SCIM groups if workspace groups haven't loaded yet.
			if len(direct) == 0 && len(v.Detail.Groups) > 0 {
				direct = v.Detail.Groups
			}
			m.popup = &popupContent{
				title: "User: " + v.Detail.UserName,
				body:  renderUserDetailPopup(v.Detail, direct, indirect),
			}
		}

	case SPDetailLoadedMsg:
		m.popupLoading = false
		if v.Err != nil {
			m.popup = &popupContent{title: "Error", body: styleIdentityErr.Render(v.Err.Error())}
		} else if v.Detail != nil {
			direct, indirect := transitiveGroupLookup(v.Detail.ID, m.wsGroups())
			if len(direct) == 0 && len(v.Detail.Groups) > 0 {
				direct = v.Detail.Groups
			}
			m.popup = &popupContent{
				title: "Service principal: " + v.Detail.DisplayName,
				body:  renderSPDetailPopup(v.Detail, direct, indirect),
			}
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

		// While popup is showing, any key closes it.
		if m.popup != nil || m.popupLoading {
			switch v.String() {
			case "esc", "enter", " ", "q":
				m.popup = nil
				m.popupLoading = false
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
					return m, func() tea.Msg {
						return LoadUserDetailRequestMsg{Workspace: item.wsName, UserID: item.entityID}
					}
				case item.isSP:
					m.popupLoading = true
					m.popupWsName = item.wsName
					return m, func() tea.Msg {
						return LoadSPDetailRequestMsg{Workspace: item.wsName, SPID: item.entityID}
					}
				}
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

func (m IdentityModel) rebuild() IdentityModel {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.items = buildWorkspaceItems(m.workspaces, m.expanded, q)
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

	if m.popupLoading || m.popup != nil {
		popupView := m.renderPopup()
		help := styleIdentityMuted.Render("  esc / enter  close")
		return searchLine + "\n" + stats + "\n" + popupView + "\n" + help
	}

	help := styleIdentityMuted.Render("  / search   ↑↓ navigate   enter expand / view details   esc clear")
	return searchLine + "\n" + stats + "\n" + m.viewport.View() + "\n" + help
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

// renderUserDetailPopup formats user detail for the popup, including transitive groups.
func renderUserDetailPopup(u *databricks.UserDetail, direct, indirect []string) string {
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

	b.WriteString(popupGroupSection(direct, indirect))
	b.WriteString("\n")

	b.WriteString(renderCatalogPerms(u.CatalogPermissions))
	return b.String()
}

// renderSPDetailPopup formats service principal detail for the popup.
func renderSPDetailPopup(sp *databricks.SPDetail, direct, indirect []string) string {
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

	b.WriteString(popupGroupSection(direct, indirect))
	b.WriteString("\n")

	b.WriteString(renderCatalogPerms(sp.CatalogPermissions))
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

		// Groups.
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
				items = append(items, treeItem{
					line:     label,
					isGroup:  true,
					groupKey: groupKey,
				})
				if grpExpanded {
					for i, mem := range g.Members {
						if query != "" && !strings.Contains(strings.ToLower(mem.DisplayName), query) {
							continue
						}
						isLast := i == len(g.Members)-1
						branch := "├"
						if isLast {
							branch = "└"
						}
						line := "      " + styleIdentityMuted.Render(branch+" ") +
							memberNameStyle(mem.Type).Render(mem.DisplayName) + "  " +
							memberTypeTag(mem.Type)
						items = append(items, treeItem{line: line})
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

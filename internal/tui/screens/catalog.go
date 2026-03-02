package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// ---------------------------------------------------------------------------
// Request / load messages
// ---------------------------------------------------------------------------

// LoadSchemasRequestMsg asks the app to fire LoadSchemasCmd.
type LoadSchemasRequestMsg struct {
	Workspace   string
	CatalogName string
}

// LoadTablesRequestMsg asks the app to fire LoadTablesCmd.
type LoadTablesRequestMsg struct {
	Workspace   string
	CatalogName string
	SchemaName  string
}

// CatalogObjectDetailRequestMsg asks the app to fire LoadCatalogObjectDetailCmd.
type CatalogObjectDetailRequestMsg struct {
	Workspace string
	Kind      string // "catalog", "schema", "table"
	FullName  string
}

// ---------------------------------------------------------------------------
// Loaded messages
// ---------------------------------------------------------------------------

// CatalogsLoadedMsg is sent when the catalog list for a workspace is loaded.
type CatalogsLoadedMsg struct {
	Workspace string
	Catalogs  []databricks.CatalogInfo
	Err       error
}

// SchemasLoadedMsg is sent when schemas for a catalog are loaded.
type SchemasLoadedMsg struct {
	Workspace   string
	CatalogName string
	Schemas     []databricks.SchemaInfo
	Err         error
}

// TablesLoadedMsg is sent when tables for a schema are loaded.
type TablesLoadedMsg struct {
	Workspace   string
	CatalogName string
	SchemaName  string
	Tables      []databricks.TableInfo
	Err         error
}

// CatalogObjectDetailLoadedMsg is sent when detail for a UC object is loaded.
type CatalogObjectDetailLoadedMsg struct {
	Workspace string
	FullName  string
	Detail    *databricks.ObjectDetail
	Err       error
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// LoadCatalogsCmd fetches the catalog list for a workspace.
func LoadCatalogsCmd(ctx context.Context, ws string, p databricks.CatalogProvider) tea.Cmd {
	return func() tea.Msg {
		cats, err := p.ListCatalogs(ctx)
		return CatalogsLoadedMsg{Workspace: ws, Catalogs: cats, Err: err}
	}
}

// LoadSchemasCmd fetches schemas for a catalog.
func LoadSchemasCmd(ctx context.Context, ws, catalogName string, p databricks.CatalogProvider) tea.Cmd {
	return func() tea.Msg {
		schemas, err := p.ListSchemas(ctx, catalogName)
		return SchemasLoadedMsg{Workspace: ws, CatalogName: catalogName, Schemas: schemas, Err: err}
	}
}

// LoadTablesCmd fetches tables for a schema.
func LoadTablesCmd(ctx context.Context, ws, catalogName, schemaName string, p databricks.CatalogProvider) tea.Cmd {
	return func() tea.Msg {
		tables, err := p.ListTables(ctx, catalogName, schemaName)
		return TablesLoadedMsg{Workspace: ws, CatalogName: catalogName, SchemaName: schemaName, Tables: tables, Err: err}
	}
}

// LoadCatalogObjectDetailCmd fetches full detail for a UC object.
func LoadCatalogObjectDetailCmd(ctx context.Context, ws, kind, fullName string, p databricks.CatalogProvider) tea.Cmd {
	return func() tea.Msg {
		detail, err := p.GetObjectDetail(ctx, kind, fullName)
		return CatalogObjectDetailLoadedMsg{Workspace: ws, FullName: fullName, Detail: detail, Err: err}
	}
}

// ---------------------------------------------------------------------------
// Tree item
// ---------------------------------------------------------------------------

type catalogItemKind int

const (
	kindCatalog catalogItemKind = iota
	kindSchema
	kindTable
)

type catalogItem struct {
	kind        catalogItemKind
	catalogName string
	schemaName  string
	tableName   string
	tableType   string
	depth       int
	isLast      bool // last sibling (for branch drawing)
}

func (it catalogItem) key() string {
	switch it.kind {
	case kindCatalog:
		return it.catalogName
	case kindSchema:
		return it.catalogName + "." + it.schemaName
	default:
		return it.catalogName + "." + it.schemaName + "." + it.tableName
	}
}

func (it catalogItem) displayName() string {
	switch it.kind {
	case kindCatalog:
		return it.catalogName
	case kindSchema:
		return it.schemaName
	default:
		return it.tableName
	}
}

func (it catalogItem) objectKind() string {
	switch it.kind {
	case kindCatalog:
		return "catalog"
	case kindSchema:
		return "schema"
	default:
		return "table"
	}
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// CatalogModel is the Bubble Tea model for the Catalogs screen.
type CatalogModel struct {
	workspace string
	catalogs  []databricks.CatalogInfo
	schemas   map[string][]databricks.SchemaInfo // key: catalogName
	tables    map[string][]databricks.TableInfo  // key: "catalog.schema"
	expanded  map[string]bool
	loading   map[string]bool
	loadErr   map[string]string
	cursor    int
	items     []catalogItem // all expanded items (unfiltered)
	width     int
	height    int
	err       error // top-level error (e.g. UC unavailable)

	// pre-fetched group hierarchy (populated via IdentityLoadedMsg)
	groups []databricks.Group

	// search/filter
	search       textinput.Model
	searchActive bool

	// detail panel
	detailKey     string // fullName of currently loaded/loading detail
	detail        *databricks.ObjectDetail
	detailLoading bool
	detailErr     error
	detailVP      viewport.Model
}

func NewCatalogModel() CatalogModel {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80

	vp := viewport.New(40, 20)

	return CatalogModel{
		schemas:  make(map[string][]databricks.SchemaInfo),
		tables:   make(map[string][]databricks.TableInfo),
		expanded: make(map[string]bool),
		loading:  make(map[string]bool),
		loadErr:  make(map[string]string),
		search:   ti,
		detailVP: vp,
	}
}

func (m CatalogModel) SetSize(w, h int) CatalogModel {
	m.width = w
	m.height = h
	leftW, rightW, visH := m.panelDims()
	m.detailVP = viewport.New(rightW, visH)
	m.detailVP.SetContent(m.renderDetailContent())
	_ = leftW
	return m
}

// Reset clears data so a workspace switch starts fresh.
// groups may be nil if identity has not yet loaded for the workspace.
func (m CatalogModel) Reset(ws string, groups []databricks.Group) CatalogModel {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 80

	vp := viewport.New(m.detailVP.Width, m.detailVP.Height)

	return CatalogModel{
		workspace: ws,
		groups:    groups,
		schemas:   make(map[string][]databricks.SchemaInfo),
		tables:    make(map[string][]databricks.TableInfo),
		expanded:  make(map[string]bool),
		loading:   make(map[string]bool),
		loadErr:   make(map[string]string),
		width:     m.width,
		height:    m.height,
		search:    ti,
		detailVP:  vp,
	}
}

// SearchFocused returns true when the search input has keyboard focus.
// App uses this to suppress global nav keys.
func (m CatalogModel) SearchFocused() bool { return m.searchActive }

func (m CatalogModel) Update(msg tea.Msg) (CatalogModel, tea.Cmd) {
	switch v := msg.(type) {

	case CatalogsLoadedMsg:
		if v.Workspace != m.workspace {
			return m, nil
		}
		if v.Err != nil {
			m.err = v.Err
			return m, nil
		}
		m.catalogs = v.Catalogs
		m.err = nil
		m.rebuildItems()

	case SchemasLoadedMsg:
		if v.Workspace != m.workspace {
			return m, nil
		}
		delete(m.loading, v.CatalogName)
		if v.Err != nil {
			m.loadErr[v.CatalogName] = v.Err.Error()
		} else {
			m.schemas[v.CatalogName] = v.Schemas
			m.expanded[v.CatalogName] = true
		}
		m.rebuildItems()

	case TablesLoadedMsg:
		if v.Workspace != m.workspace {
			return m, nil
		}
		schemaKey := v.CatalogName + "." + v.SchemaName
		delete(m.loading, schemaKey)
		if v.Err != nil {
			m.loadErr[schemaKey] = v.Err.Error()
		} else {
			m.tables[schemaKey] = v.Tables
			m.expanded[schemaKey] = true
		}
		m.rebuildItems()

	case IdentityLoadedMsg:
		// Store groups for this workspace for client-side grant expansion.
		if v.Workspace == m.workspace && v.Err == nil {
			m.groups = v.Groups
			// Refresh detail panel if already showing detail.
			if m.detail != nil {
				m.detailVP.SetContent(m.renderDetailContent())
			}
		}

	case CatalogObjectDetailLoadedMsg:
		if v.Workspace != m.workspace || v.FullName != m.detailKey {
			return m, nil
		}
		m.detailLoading = false
		m.detailErr = v.Err
		m.detail = v.Detail
		content := m.renderDetailContent()
		m.detailVP.SetContent(content)
		m.detailVP.GotoTop()

	case tea.KeyMsg:
		// When search is active, forward to textinput; Esc clears, Enter confirms.
		if m.searchActive {
			switch v.Type {
			case tea.KeyEsc:
				m.searchActive = false
				m.search.Blur()
				m.search.SetValue("")
				m.cursor = 0
				return m, nil
			case tea.KeyEnter:
				m.searchActive = false
				m.search.Blur()
				m.cursor = 0
				return m, nil
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(v, key.NewBinding(key.WithKeys("up", "k"))):
			visible := m.visibleItems()
			if m.cursor > 0 {
				m.cursor--
			}
			_ = visible
		case key.Matches(v, key.NewBinding(key.WithKeys("down", "j"))):
			visible := m.visibleItems()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
		case key.Matches(v, key.NewBinding(key.WithKeys("enter", " "))):
			return m.toggleOrLoad()
		case key.Matches(v, key.NewBinding(key.WithKeys("right", "l"))):
			return m.requestDetail()
		case key.Matches(v, key.NewBinding(key.WithKeys("/"))):
			m.searchActive = true
			m.search.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

// toggleOrLoad expands or collapses the item under the cursor.
func (m CatalogModel) toggleOrLoad() (CatalogModel, tea.Cmd) {
	visible := m.visibleItems()
	if len(visible) == 0 || m.cursor >= len(visible) {
		return m, nil
	}
	it := visible[m.cursor]

	switch it.kind {
	case kindCatalog:
		nodeKey := it.catalogName
		if m.expanded[nodeKey] {
			m.expanded[nodeKey] = false
			m.rebuildItems()
			return m, nil
		}
		if _, ok := m.schemas[nodeKey]; ok {
			m.expanded[nodeKey] = true
			m.rebuildItems()
			return m, nil
		}
		if m.loading[nodeKey] {
			return m, nil
		}
		m.loading[nodeKey] = true
		m.rebuildItems()
		return m, func() tea.Msg {
			return LoadSchemasRequestMsg{Workspace: m.workspace, CatalogName: it.catalogName}
		}

	case kindSchema:
		nodeKey := it.catalogName + "." + it.schemaName
		if m.expanded[nodeKey] {
			m.expanded[nodeKey] = false
			m.rebuildItems()
			return m, nil
		}
		if _, ok := m.tables[nodeKey]; ok {
			m.expanded[nodeKey] = true
			m.rebuildItems()
			return m, nil
		}
		if m.loading[nodeKey] {
			return m, nil
		}
		m.loading[nodeKey] = true
		m.rebuildItems()
		return m, func() tea.Msg {
			return LoadTablesRequestMsg{Workspace: m.workspace, CatalogName: it.catalogName, SchemaName: it.schemaName}
		}
	}
	return m, nil
}

// requestDetail emits a CatalogObjectDetailRequestMsg for the current item.
func (m CatalogModel) requestDetail() (CatalogModel, tea.Cmd) {
	visible := m.visibleItems()
	if len(visible) == 0 || m.cursor >= len(visible) {
		return m, nil
	}
	it := visible[m.cursor]
	fullName := it.key()
	if m.detailKey == fullName && (m.detail != nil || m.detailLoading) {
		return m, nil // already loaded or loading
	}
	m.detailKey = fullName
	m.detailLoading = true
	m.detailErr = nil
	m.detail = nil
	m.detailVP.SetContent(styleCatalogLoading.Render("  loading…"))
	return m, func() tea.Msg {
		return CatalogObjectDetailRequestMsg{Workspace: m.workspace, Kind: it.objectKind(), FullName: fullName}
	}
}

// rebuildItems flattens the tree into m.items for rendering.
func (m *CatalogModel) rebuildItems() {
	items := make([]catalogItem, 0, len(m.catalogs)*4)
	for ci, cat := range m.catalogs {
		catItem := catalogItem{
			kind:        kindCatalog,
			catalogName: cat.Name,
			depth:       0,
			isLast:      ci == len(m.catalogs)-1,
		}
		items = append(items, catItem)

		catKey := cat.Name
		if !m.expanded[catKey] {
			continue
		}
		schemas := m.schemas[catKey]
		for si, schema := range schemas {
			schKey := catKey + "." + schema.Name
			schItem := catalogItem{
				kind:        kindSchema,
				catalogName: cat.Name,
				schemaName:  schema.Name,
				depth:       1,
				isLast:      si == len(schemas)-1,
			}
			items = append(items, schItem)

			if !m.expanded[schKey] {
				continue
			}
			tables := m.tables[schKey]
			for ti, tbl := range tables {
				items = append(items, catalogItem{
					kind:        kindTable,
					catalogName: cat.Name,
					schemaName:  schema.Name,
					tableName:   tbl.Name,
					tableType:   tbl.TableType,
					depth:       2,
					isLast:      ti == len(tables)-1,
				})
			}
		}
	}
	m.items = items
	// Clamp cursor to visible items.
	visible := m.filteredItems(m.items)
	if m.cursor >= len(visible) && len(visible) > 0 {
		m.cursor = len(visible) - 1
	}
}

// visibleItems returns the items that should be shown (applying filter).
func (m CatalogModel) visibleItems() []catalogItem {
	return m.filteredItems(m.items)
}

// filteredItems applies the search filter to a item list.
func (m CatalogModel) filteredItems(items []catalogItem) []catalogItem {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		return items
	}

	// Pass 1: find which catalog/schema keys have any descendant matching.
	matchCat := map[string]bool{}
	matchSch := map[string]bool{}
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.displayName()), q) {
			switch it.kind {
			case kindTable:
				matchSch[it.catalogName+"."+it.schemaName] = true
				matchCat[it.catalogName] = true
			case kindSchema:
				matchSch[it.catalogName+"."+it.schemaName] = true
				matchCat[it.catalogName] = true
			case kindCatalog:
				matchCat[it.catalogName] = true
			}
		}
	}

	// Pass 2: collect items that pass the filter.
	result := make([]catalogItem, 0, len(items))
	for _, it := range items {
		name := strings.ToLower(it.displayName())
		switch it.kind {
		case kindCatalog:
			if matchCat[it.catalogName] {
				result = append(result, it)
			}
		case kindSchema:
			schKey := it.catalogName + "." + it.schemaName
			if strings.Contains(name, q) || matchSch[schKey] {
				result = append(result, it)
			}
		case kindTable:
			if strings.Contains(name, q) {
				result = append(result, it)
			}
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Panel dimension helpers
// ---------------------------------------------------------------------------

func (m CatalogModel) panelDims() (leftW, rightW, visH int) {
	visH = m.height - 6
	if visH < 4 {
		visH = 4
	}
	leftW = m.width * 40 / 100
	if leftW < 20 {
		leftW = 20
	}
	rightW = m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	return
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

var (
	styleCatalogSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("57"))
	styleCatalogCatalog = lipgloss.NewStyle().Bold(true)
	styleCatalogSchema  = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	styleCatalogTable   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	styleCatalogType    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleCatalogLoading = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	styleCatalogError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleCatalogSection = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	styleCatalogLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleCatalogValue   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleCatalogGroup   = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	styleCatalogPriv    = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

func (m CatalogModel) View() string {
	if m.err != nil {
		msg := m.err.Error()
		if strings.Contains(strings.ToLower(msg), "unity catalog") ||
			strings.Contains(strings.ToLower(msg), "metastore") ||
			strings.Contains(strings.ToLower(msg), "does not have unity catalog") {
			return styleCatalogError.Render("  (Unity Catalog is not enabled for this workspace)\n")
		}
		return styleCatalogError.Render("  Error: "+msg) + "\n"
	}

	leftW, rightW, visH := m.panelDims()

	// ── left: search bar + tree ──────────────────────────────────────────────
	leftPanel := m.renderTree(leftW, visH)

	// ── right: detail panel ──────────────────────────────────────────────────
	m.detailVP.Width = rightW
	m.detailVP.Height = visH
	rightPanel := m.detailVP.View()

	// Pad right panel to full height if shorter.
	rightLines := strings.Count(rightPanel, "\n") + 1
	if rightLines < visH {
		rightPanel += strings.Repeat("\n", visH-rightLines)
	}

	// ── separator ───────────────────────────────────────────────────────────
	sepLines := make([]string, visH)
	for i := range sepLines {
		sepLines[i] = "│"
	}
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Join(sepLines, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Render(leftPanel),
		sep,
		lipgloss.NewStyle().Width(rightW).Render(rightPanel),
	)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
		Render("  ↑/↓ navigate  enter expand  → detail  / filter  enter confirm  esc clear")
	return body + "\n" + help
}

func (m CatalogModel) renderTree(width, visH int) string {
	var sb strings.Builder

	// Search bar.
	if m.searchActive {
		bar := "/ " + m.search.View()
		sb.WriteString(lipgloss.NewStyle().Width(width).Render(bar))
	} else {
		q := m.search.Value()
		if q != "" {
			bar := styleCatalogLabel.Render("/ ") + q + styleCatalogLabel.Render("  (esc clear)")
			sb.WriteString(lipgloss.NewStyle().Width(width).Render(bar))
		} else {
			sb.WriteString(lipgloss.NewStyle().Width(width).Render(styleCatalogLabel.Render("  / to filter")))
		}
	}
	sb.WriteByte('\n')

	if len(m.catalogs) == 0 {
		sb.WriteString(styleCatalogLoading.Render("  Loading catalogs…"))
		return sb.String()
	}

	visible := m.visibleItems()

	// Determine scroll window.
	start := m.cursor - visH/2
	if start < 0 {
		start = 0
	}
	if start+visH > len(visible) {
		start = len(visible) - visH
		if start < 0 {
			start = 0
		}
	}
	end := start + visH
	if end > len(visible) {
		end = len(visible)
	}

	for i := start; i < end; i++ {
		it := visible[i]
		line := m.renderItem(it, width)
		if i == m.cursor {
			line = styleCatalogSelected.Render(line)
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m CatalogModel) renderItem(it catalogItem, width int) string {
	indent := strings.Repeat("  ", it.depth)
	nodeKey := it.key()

	var prefix, name, suffix string

	switch it.kind {
	case kindCatalog:
		if m.loading[nodeKey] {
			prefix = "  ▶ "
			name = styleCatalogCatalog.Render(it.catalogName)
			suffix = styleCatalogLoading.Render(" [loading…]")
		} else if m.expanded[nodeKey] {
			prefix = "  ▼ "
			name = styleCatalogCatalog.Render(it.catalogName)
		} else {
			prefix = "  ▶ "
			name = styleCatalogCatalog.Render(it.catalogName)
		}
		if e, ok := m.loadErr[nodeKey]; ok {
			suffix = styleCatalogError.Render(" [" + e + "]")
		}

	case kindSchema:
		branch := "├─ "
		if it.isLast {
			branch = "└─ "
		}
		if m.loading[nodeKey] {
			prefix = indent + branch
			name = styleCatalogSchema.Render(it.schemaName)
			suffix = styleCatalogLoading.Render(" [loading…]")
		} else if m.expanded[nodeKey] {
			prefix = indent + branch + "▼ "
			name = styleCatalogSchema.Render(it.schemaName)
		} else {
			prefix = indent + branch
			name = styleCatalogSchema.Render(it.schemaName)
		}
		if e, ok := m.loadErr[nodeKey]; ok {
			suffix = styleCatalogError.Render(" [" + e + "]")
		}

	case kindTable:
		branch := "  ├─ "
		if it.isLast {
			branch = "  └─ "
		}
		prefix = indent + branch
		name = styleCatalogTable.Render(it.tableName)
		if it.tableType != "" && it.tableType != "TABLE" {
			suffix = "  " + styleCatalogType.Render(it.tableType)
		}
	}

	maxW := width - 2
	if maxW < 10 {
		maxW = 10
	}
	line := fmt.Sprintf("%-*s%s", maxW, prefix+name, suffix)
	return line
}

// ---------------------------------------------------------------------------
// Detail panel
// ---------------------------------------------------------------------------

// renderDetailContent builds the full text for the detail viewport.
func (m CatalogModel) renderDetailContent() string {
	if m.detailKey == "" {
		return styleCatalogLoading.Render("  Select an item and press → to view details")
	}
	if m.detailLoading {
		return styleCatalogLoading.Render("  Loading…")
	}
	if m.detailErr != nil {
		return styleCatalogError.Render("  Error: " + m.detailErr.Error())
	}
	if m.detail == nil {
		return styleCatalogLoading.Render("  No detail available")
	}
	return renderDetail(m.detail, m.groups)
}

func renderDetail(d *databricks.ObjectDetail, groups []databricks.Group) string {
	var sb strings.Builder

	// Title line.
	kindBadge := styleCatalogType.Render("[" + strings.ToUpper(d.Kind) + "]")
	if d.TableType != "" {
		kindBadge = styleCatalogType.Render("[" + d.TableType + "]")
	}
	sb.WriteString("  " + styleCatalogCatalog.Render(d.FullName) + "  " + kindBadge + "\n\n")

	// Metadata fields.
	field := func(label, val string) {
		if val == "" {
			return
		}
		sb.WriteString("  " + styleCatalogLabel.Render(fmt.Sprintf("%-12s", label)) +
			styleCatalogValue.Render(val) + "\n")
	}
	field("Owner:", d.Owner)
	field("Comment:", d.Comment)
	field("Format:", d.DataFormat)
	field("Storage:", d.StorageLocation)
	field("Root:", d.StorageRoot)
	if d.ViewDefinition != "" {
		sb.WriteString("  " + styleCatalogLabel.Render("View SQL:") + "\n")
		for _, line := range strings.Split(d.ViewDefinition, "\n") {
			sb.WriteString("    " + styleCatalogValue.Render(line) + "\n")
		}
	}
	if !d.CreatedAt.IsZero() {
		field("Created:", d.CreatedAt.Format("2006-01-02 15:04"))
	}
	if !d.UpdatedAt.IsZero() {
		field("Updated:", d.UpdatedAt.Format("2006-01-02 15:04"))
	}

	// Grants sections — expand group membership client-side.
	writeGrants := func(title string, grants []databricks.GrantEntry) {
		if grants == nil {
			return
		}
		expanded := expandGrants(grants, groups)
		sb.WriteString("\n  " + styleCatalogSection.Render("── "+title+" ") + "\n")
		if len(expanded) == 0 {
			sb.WriteString("  " + styleCatalogLoading.Render("  (no grants)") + "\n")
			return
		}
		for _, g := range expanded {
			label := principalLabel(g.Principal)
			if g.Via != "" {
				label += styleCatalogLabel.Render("  via " + g.Via)
			}
			privStr := strings.Join(g.Privileges, ", ")
			sb.WriteString(fmt.Sprintf("  %-38s %s\n",
				label,
				styleCatalogPriv.Render(privStr),
			))
		}
	}

	switch d.Kind {
	case "catalog":
		writeGrants("Catalog grants", d.DirectGrants)
	case "schema":
		writeGrants("Schema grants", d.DirectGrants)
		writeGrants("Catalog grants (inherited)", d.GrandpaGrants)
	case "table":
		writeGrants("Table grants", d.DirectGrants)
		writeGrants("Schema grants (inherited)", d.ParentGrants)
		writeGrants("Catalog grants (inherited)", d.GrandpaGrants)
	}

	return sb.String()
}

// expandGrants augments grants with the transitive members of any group principal.
// In Unity Catalog, privileges flow DOWNWARD: if a group has a grant, all its
// members (users, SPs, nested subgroups) inherit it. Parent groups do NOT
// inherit grants from their subgroups.
//
// Expansion uses the pre-fetched workspace group hierarchy; groups not present
// in the workspace SCIM list cannot be expanded.
// Depth is capped at 4 to handle deeply nested groups.
func expandGrants(grants []databricks.GrantEntry, groups []databricks.Group) []databricks.GrantEntry {
	if len(groups) == 0 {
		return grants
	}

	// Build forward lookup: displayName → group.
	byName := make(map[string]*databricks.Group, len(groups))
	for i := range groups {
		byName[groups[i].DisplayName] = &groups[i]
	}

	// Build ID → display name for members whose Display field is empty.
	idToName := make(map[string]string, len(groups))
	for _, g := range groups {
		idToName[g.ID] = g.DisplayName
	}

	memberName := func(m databricks.GroupMember) string {
		if m.DisplayName != "" {
			return m.DisplayName
		}
		return idToName[m.ID]
	}

	result := make([]databricks.GrantEntry, 0, len(grants)*3)
	result = append(result, grants...)
	seen := make(map[string]bool, len(grants))
	for _, g := range grants {
		seen[g.Principal] = true
	}

	// Downward: walk members of the granted group transitively.
	// via tracks the top-level group name for the Via label.
	var addMembers func(groupName, via string, privs []string, depth int)
	addMembers = func(groupName, via string, privs []string, depth int) {
		if depth > 4 {
			return
		}
		grp, ok := byName[groupName]
		if !ok {
			return
		}
		for _, m := range grp.Members {
			name := memberName(m)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, databricks.GrantEntry{
				Principal:  name,
				Privileges: privs,
				Via:        via,
			})
			if m.Type == "Group" {
				addMembers(name, via, privs, depth+1)
			}
		}
	}

	for _, g := range grants {
		if _, isGroup := byName[g.Principal]; isGroup {
			addMembers(g.Principal, g.Principal, g.Privileges, 0)
		}
	}
	return result
}

// principalLabel formats a principal with a "(group)" suffix when applicable.
func principalLabel(principal string) string {
	if strings.Contains(principal, "@") {
		return styleCatalogValue.Render(principal)
	}
	return styleCatalogGroup.Render(principal) + styleCatalogLabel.Render(" (group)")
}

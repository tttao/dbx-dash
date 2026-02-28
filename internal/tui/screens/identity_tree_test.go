package screens

import (
	"strings"
	"testing"

	"github.com/tttao/dbx-dash/internal/databricks"
)

// TestGroupTreeView_SubgroupMembersVisible tests the SCIM type-unreliability scenario:
// BusinessAnalysts → AnalyticsTeam (type "Unknown") → Alice (type "Unknown").
// All group members report type "Unknown" from the SCIM API.
// The tree view must still correctly:
//   - identify AnalyticsTeam as a child (not root) group
//   - render Alice inside AnalyticsTeam when expanded
func TestGroupTreeView_SubgroupMembersVisible(t *testing.T) {
	alice := databricks.IdentityUser{
		ID:       "user-alice",
		UserName: "alice@example.com",
		Active:   true,
	}
	analyticsTeam := databricks.Group{
		ID:          "group-analytics",
		DisplayName: "AnalyticsTeam",
		Members: []databricks.GroupMember{
			{ID: "user-alice", DisplayName: "", Type: "Unknown"}, // SCIM quirk
		},
	}
	businessAnalysts := databricks.Group{
		ID:          "group-ba",
		DisplayName: "BusinessAnalysts",
		Members: []databricks.GroupMember{
			{ID: "group-analytics", DisplayName: "", Type: "Unknown"}, // SCIM quirk
		},
	}

	ws := wsIdentityData{
		name:   "free1",
		groups: []databricks.Group{businessAnalysts, analyticsTeam},
		users:  []databricks.IdentityUser{alice},
	}

	// Expand both groups.
	expanded := map[string]bool{
		"ws:free1":                           true,
		"free1:" + businessAnalysts.ID:       true,
		"free1:" + analyticsTeam.ID:          true,
	}

	items := buildGroupTreeItems(ws, expanded, "")

	// Collect rendered lines for inspection.
	var lines []string
	for _, it := range items {
		lines = append(lines, it.line)
	}
	joined := strings.Join(lines, "\n")

	// AnalyticsTeam should appear as a child of BusinessAnalysts, not a separate root.
	// With the fix, there should be exactly 2 group header rows:
	//   BusinessAnalysts (root)
	//     AnalyticsTeam (child, nested inside BA when expanded)
	groupHeaders := 0
	for _, it := range items {
		if it.isGroup {
			groupHeaders++
		}
	}
	if groupHeaders != 2 {
		t.Errorf("expected 2 group header items, got %d\nlines:\n%s", groupHeaders, joined)
	}

	// Alice must appear somewhere in the output.
	aliceFound := false
	for _, it := range items {
		if it.isUser && it.entityID == "user-alice" {
			aliceFound = true
			break
		}
	}
	if !aliceFound {
		t.Errorf("alice not found in tree items\nlines:\n%s", joined)
	}

	// There should be no "(unaffiliated)" section because alice IS in a group.
	for _, it := range items {
		if strings.Contains(it.line, "unaffiliated") {
			t.Errorf("alice incorrectly appears in unaffiliated section\nlines:\n%s", joined)
			break
		}
	}
}

// TestGroupTreeView_RootGroupDetection verifies that a group with no parent
// appears as a root and a group that is a child of another does not.
func TestGroupTreeView_RootGroupDetection(t *testing.T) {
	child := databricks.Group{
		ID:          "group-child",
		DisplayName: "Child",
		Members:     nil,
	}
	parent := databricks.Group{
		ID:          "group-parent",
		DisplayName: "Parent",
		Members: []databricks.GroupMember{
			{ID: "group-child", Type: "Unknown"}, // unreliable type
		},
	}

	ws := wsIdentityData{
		name:   "test",
		groups: []databricks.Group{parent, child},
	}
	expanded := map[string]bool{
		"ws:test":              true,
		"test:group-parent":   true,
	}

	items := buildGroupTreeItems(ws, expanded, "")

	// Only parent should be a root-level group item; child appears nested.
	// Count group items at "root" indentation (4 spaces) vs nested (6 spaces).
	rootGroupCount := 0
	for _, it := range items {
		if it.isGroup {
			// Root groups are rendered with baseIndent "    " (4 spaces).
			// Nested groups get "    " + "  " = 6 spaces.
			if strings.HasPrefix(stripANSI(it.line), "    ") && !strings.HasPrefix(stripANSI(it.line), "      ") {
				rootGroupCount++
			}
		}
	}
	if rootGroupCount != 1 {
		var lines []string
		for _, it := range items {
			lines = append(lines, it.line)
		}
		t.Errorf("expected 1 root-level group, got %d\nlines:\n%s", rootGroupCount, strings.Join(lines, "\n"))
	}
}

// TestListView_UserMemberTypeTag verifies that user members inside expanded groups
// render as [user] (not [unknown]) when the SCIM type field is "Unknown".
func TestListView_UserMemberTypeTag(t *testing.T) {
	alice := databricks.IdentityUser{
		ID:       "user-alice",
		UserName: "alice@example.com",
		Active:   true,
	}
	grp := databricks.Group{
		ID:          "group-ba",
		DisplayName: "BusinessAnalysts",
		Members: []databricks.GroupMember{
			{ID: "user-alice", DisplayName: "alice@example.com", Type: "Unknown"},
		},
	}

	workspaces := []wsIdentityData{{
		name:   "test",
		groups: []databricks.Group{grp},
		users:  []databricks.IdentityUser{alice},
	}}
	expanded := map[string]bool{
		"ws:test":         true,
		"test:group-ba":   true,
	}

	items := buildWorkspaceItems(workspaces, expanded, "")

	// Find the member line and verify it has [user] tag, not [unknown].
	for _, it := range items {
		plain := stripANSI(it.line)
		if strings.Contains(plain, "alice") {
			if strings.Contains(plain, "[unknown]") {
				t.Errorf("alice rendered with [unknown] tag: %q", plain)
			}
			if !strings.Contains(plain, "[user]") {
				t.Errorf("alice missing [user] tag: %q", plain)
			}
			return
		}
	}
	t.Error("alice not found in list view items")
}

// stripANSI removes ANSI escape codes for plain-text comparisons.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

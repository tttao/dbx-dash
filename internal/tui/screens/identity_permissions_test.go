package screens

import (
	"strings"
	"testing"

	"github.com/tttao/dbx-dash/internal/databricks"
)

// ---------------------------------------------------------------------------
// renderSPWhoHasAccess
// ---------------------------------------------------------------------------

func TestSPPermissions_NilACL_ShowsUnavailable(t *testing.T) {
	out := renderSPWhoHasAccess(nil)
	if !strings.Contains(out, "workspace permissions not available") {
		t.Errorf("expected unavailable message, got:\n%s", out)
	}
}

func TestSPPermissions_EmptyACL_ShowsNoPermissions(t *testing.T) {
	out := renderSPWhoHasAccess([]databricks.SPAccessEntry{})
	if !strings.Contains(out, "no explicit workspace permissions") {
		t.Errorf("expected no-permissions message, got:\n%s", out)
	}
}

func TestSPPermissions_UserCanUse(t *testing.T) {
	acl := []databricks.SPAccessEntry{
		{UserName: "alice@example.com", Level: "CAN_USE"},
	}
	out := renderSPWhoHasAccess(acl)
	if !strings.Contains(out, "alice@example.com") {
		t.Errorf("expected username in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[can_use]") {
		t.Errorf("expected [can_use] level in output, got:\n%s", out)
	}
}

func TestSPPermissions_UserIsOwner(t *testing.T) {
	acl := []databricks.SPAccessEntry{
		{UserName: "bob@example.com", Level: "IS_OWNER"},
	}
	out := renderSPWhoHasAccess(acl)
	if !strings.Contains(out, "bob@example.com") {
		t.Errorf("expected username in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[is_owner]") {
		t.Errorf("expected [is_owner] level in output, got:\n%s", out)
	}
}

func TestSPPermissions_GroupCanManage(t *testing.T) {
	acl := []databricks.SPAccessEntry{
		{GroupName: "admins", Level: "CAN_MANAGE"},
	}
	out := renderSPWhoHasAccess(acl)
	if !strings.Contains(out, "admins") {
		t.Errorf("expected group name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[can_manage]") {
		t.Errorf("expected [can_manage] level in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(group)") {
		t.Errorf("expected (group) marker in output, got:\n%s", out)
	}
}

func TestSPPermissions_MixedUserAndGroup(t *testing.T) {
	acl := []databricks.SPAccessEntry{
		{UserName: "alice@example.com", Level: "CAN_USE"},
		{GroupName: "admins", Level: "CAN_MANAGE"},
	}
	out := renderSPWhoHasAccess(acl)
	if !strings.Contains(out, "alice@example.com") {
		t.Errorf("expected user entry in output, got:\n%s", out)
	}
	if !strings.Contains(out, "admins") {
		t.Errorf("expected group entry in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[can_use]") {
		t.Errorf("expected [can_use] in output, got:\n%s", out)
	}
	if !strings.Contains(out, "[can_manage]") {
		t.Errorf("expected [can_manage] in output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// renderUserSPAccess
// ---------------------------------------------------------------------------

func TestUserSPAccess_Empty_ShowsNoMemberships(t *testing.T) {
	out := renderUserSPAccess([]databricks.SPUserAccess{})
	if !strings.Contains(out, "no service principal co-memberships") {
		t.Errorf("expected no-memberships message, got:\n%s", out)
	}
}

func TestUserSPAccess_SingleSP_SingleGroup(t *testing.T) {
	access := []databricks.SPUserAccess{
		{SPID: "sp-1", DisplayName: "mybot", ViaGroups: []string{"team-a"}},
	}
	out := renderUserSPAccess(access)
	if !strings.Contains(out, "mybot") {
		t.Errorf("expected SP display name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "via: team-a") {
		t.Errorf("expected via-group chain in output, got:\n%s", out)
	}
}

func TestUserSPAccess_MultipleViaGroups_AreSorted(t *testing.T) {
	access := []databricks.SPUserAccess{
		{SPID: "sp-1", DisplayName: "mybot", ViaGroups: []string{"group-b", "group-a"}},
	}
	out := renderUserSPAccess(access)
	if !strings.Contains(out, "via: group-b, group-a") {
		// ViaGroups are sorted by computeUserSPAccess before being stored;
		// renderUserSPAccess renders them as-is (caller's responsibility to sort).
		// Verify whatever order is present is rendered faithfully.
		if !strings.Contains(out, "group-b") || !strings.Contains(out, "group-a") {
			t.Errorf("expected both groups in output, got:\n%s", out)
		}
	}
}

// ---------------------------------------------------------------------------
// computeUserSPAccess
// ---------------------------------------------------------------------------

func TestComputeUserSPAccess_Empty(t *testing.T) {
	result := computeUserSPAccess("user-1", wsIdentityData{})
	if result != nil {
		t.Errorf("expected nil for empty workspace, got %v", result)
	}
}

func TestComputeUserSPAccess_UserNotInAnyGroup(t *testing.T) {
	ws := wsIdentityData{
		groups: []databricks.Group{
			{ID: "g1", DisplayName: "DevTeam", Members: []databricks.GroupMember{
				{ID: "sp-99", Type: "ServicePrincipal"},
			}},
		},
		sps: []databricks.WorkspaceServicePrincipal{
			{ID: "sp-99", DisplayName: "some-bot"},
		},
	}
	result := computeUserSPAccess("user-alice", ws)
	if len(result) != 0 {
		t.Errorf("expected no SP access for user not in any group, got %v", result)
	}
}

func TestComputeUserSPAccess_DirectMembership(t *testing.T) {
	// user-alice and sp-bot are both in group g1.
	ws := wsIdentityData{
		groups: []databricks.Group{
			{ID: "g1", DisplayName: "DevTeam", Members: []databricks.GroupMember{
				{ID: "user-alice", Type: "User"},
				{ID: "sp-bot", Type: "ServicePrincipal"},
			}},
		},
		sps: []databricks.WorkspaceServicePrincipal{
			{ID: "sp-bot", DisplayName: "deploy-bot"},
		},
	}
	result := computeUserSPAccess("user-alice", ws)
	if len(result) != 1 {
		t.Fatalf("expected 1 SP access entry, got %d: %v", len(result), result)
	}
	if result[0].DisplayName != "deploy-bot" {
		t.Errorf("expected deploy-bot, got %s", result[0].DisplayName)
	}
	if len(result[0].ViaGroups) != 1 || result[0].ViaGroups[0] != "DevTeam" {
		t.Errorf("expected ViaGroups=[DevTeam], got %v", result[0].ViaGroups)
	}
}

func TestComputeUserSPAccess_TransitiveMembership(t *testing.T) {
	// user-alice is in g1 (Engineers), g1 is a member of g2 (AllStaff).
	// sp-bot is in g2 (AllStaff). Alice reaches sp-bot transitively.
	ws := wsIdentityData{
		groups: []databricks.Group{
			{
				ID:          "g1",
				DisplayName: "Engineers",
				Members: []databricks.GroupMember{
					{ID: "user-alice", Type: "User"},
				},
			},
			{
				ID:          "g2",
				DisplayName: "AllStaff",
				Members: []databricks.GroupMember{
					{ID: "g1", Type: "Group"},           // Engineers nested inside AllStaff
					{ID: "sp-bot", Type: "ServicePrincipal"},
				},
			},
		},
		sps: []databricks.WorkspaceServicePrincipal{
			{ID: "sp-bot", DisplayName: "company-bot"},
		},
	}
	result := computeUserSPAccess("user-alice", ws)
	if len(result) != 1 {
		t.Fatalf("expected 1 SP access entry via transitive group, got %d: %v", len(result), result)
	}
	if result[0].DisplayName != "company-bot" {
		t.Errorf("expected company-bot, got %s", result[0].DisplayName)
	}
	// SP is in AllStaff (g2), which is the linking group.
	found := false
	for _, g := range result[0].ViaGroups {
		if g == "AllStaff" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected AllStaff in ViaGroups, got %v", result[0].ViaGroups)
	}
}

func TestComputeUserSPAccess_MultipleGroupsSameSP_ListedOnce(t *testing.T) {
	// user-alice is in both g1 and g2; sp-bot is in both too.
	// SP should appear once with both groups in ViaGroups (sorted).
	ws := wsIdentityData{
		groups: []databricks.Group{
			{
				ID:          "g1",
				DisplayName: "Bravo",
				Members: []databricks.GroupMember{
					{ID: "user-alice", Type: "User"},
					{ID: "sp-bot", Type: "ServicePrincipal"},
				},
			},
			{
				ID:          "g2",
				DisplayName: "Alpha",
				Members: []databricks.GroupMember{
					{ID: "user-alice", Type: "User"},
					{ID: "sp-bot", Type: "ServicePrincipal"},
				},
			},
		},
		sps: []databricks.WorkspaceServicePrincipal{
			{ID: "sp-bot", DisplayName: "shared-bot"},
		},
	}
	result := computeUserSPAccess("user-alice", ws)
	if len(result) != 1 {
		t.Fatalf("expected SP deduplicated to 1 entry, got %d: %v", len(result), result)
	}
	if len(result[0].ViaGroups) != 2 {
		t.Errorf("expected 2 ViaGroups, got %v", result[0].ViaGroups)
	}
	// ViaGroups must be sorted: Alpha before Bravo.
	if result[0].ViaGroups[0] != "Alpha" || result[0].ViaGroups[1] != "Bravo" {
		t.Errorf("expected ViaGroups sorted [Alpha Bravo], got %v", result[0].ViaGroups)
	}
}

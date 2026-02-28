package sdk

import (
	"context"
	"fmt"
	"strings"

	dbsdk "github.com/databricks/databricks-sdk-go"
	dbcatalog "github.com/databricks/databricks-sdk-go/service/catalog"
	"github.com/databricks/databricks-sdk-go/service/iam"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// SDKIdentityProvider implements databricks.IdentityProvider using workspace-level
// SCIM APIs. Requires workspace admin access; no account admin needed.
type SDKIdentityProvider struct {
	client *dbsdk.WorkspaceClient
}

func (p *SDKIdentityProvider) ListGroups(ctx context.Context) ([]databricks.Group, error) {
	// Request only the fields we need to reduce payload size on large workspaces.
	all, err := p.client.Groups.ListAll(ctx, iam.ListGroupsRequest{
		Attributes: "id,displayName,members",
	})
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	out := make([]databricks.Group, 0, len(all))
	for _, g := range all {
		members := make([]databricks.GroupMember, 0, len(g.Members))
		for _, m := range g.Members {
			members = append(members, databricks.GroupMember{
				ID:          m.Value,
				DisplayName: m.Display,
				Type:        memberTypeFromRef(m.Ref, m.Type),
			})
		}
		out = append(out, databricks.Group{
			ID:          g.Id,
			DisplayName: g.DisplayName,
			Members:     members,
		})
	}
	return out, nil
}

func (p *SDKIdentityProvider) ListUsers(ctx context.Context) ([]databricks.IdentityUser, error) {
	all, err := p.client.Users.ListAll(ctx, iam.ListUsersRequest{
		Attributes: "id,userName,displayName,active",
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]databricks.IdentityUser, 0, len(all))
	for _, u := range all {
		out = append(out, databricks.IdentityUser{
			ID:          u.Id,
			UserName:    u.UserName,
			DisplayName: u.DisplayName,
			Active:      u.Active,
		})
	}
	return out, nil
}

func (p *SDKIdentityProvider) ListServicePrincipals(ctx context.Context) ([]databricks.WorkspaceServicePrincipal, error) {
	all, err := p.client.ServicePrincipals.ListAll(ctx, iam.ListServicePrincipalsRequest{
		Attributes: "id,applicationId,displayName,active",
	})
	if err != nil {
		return nil, fmt.Errorf("list service principals: %w", err)
	}
	out := make([]databricks.WorkspaceServicePrincipal, 0, len(all))
	for _, sp := range all {
		out = append(out, databricks.WorkspaceServicePrincipal{
			ID:            sp.Id,
			ApplicationID: sp.ApplicationId,
			DisplayName:   sp.DisplayName,
			Active:        sp.Active,
		})
	}
	return out, nil
}

func (p *SDKIdentityProvider) GetUser(ctx context.Context, userID string) (*databricks.UserDetail, error) {
	u, err := p.client.Users.Get(ctx, iam.GetUserRequest{Id: userID})
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", userID, err)
	}
	return &databricks.UserDetail{
		ID:           u.Id,
		UserName:     u.UserName,
		DisplayName:  u.DisplayName,
		Active:       u.Active,
		Entitlements: scimComplexValues(u.Entitlements),
		Groups:       scimComplexDisplays(u.Groups),
		Roles:        scimComplexValues(u.Roles),
	}, nil
}

func (p *SDKIdentityProvider) GetServicePrincipal(ctx context.Context, spID string) (*databricks.SPDetail, error) {
	sp, err := p.client.ServicePrincipals.Get(ctx, iam.GetServicePrincipalRequest{Id: spID})
	if err != nil {
		return nil, fmt.Errorf("get service principal %s: %w", spID, err)
	}
	return &databricks.SPDetail{
		ID:            sp.Id,
		ApplicationID: sp.ApplicationId,
		DisplayName:   sp.DisplayName,
		Active:        sp.Active,
		Entitlements:  scimComplexValues(sp.Entitlements),
		Groups:        scimComplexDisplays(sp.Groups),
		Roles:         scimComplexValues(sp.Roles),
	}, nil
}

// GetCatalogPermissions returns Unity Catalog grants for principalName at catalog level.
// principalName is the user's email or the SP's applicationId (as a string).
// Returns nil (not an error) when Unity Catalog is unavailable or inaccessible.
func (p *SDKIdentityProvider) GetCatalogPermissions(ctx context.Context, principalName string) ([]databricks.CatalogPermission, error) {
	if principalName == "" {
		return nil, nil
	}
	cats, err := p.client.Catalogs.ListAll(ctx, dbcatalog.ListCatalogsRequest{})
	if err != nil {
		// Unity Catalog is not enabled or caller lacks metastore access.
		return nil, nil
	}

	var result []databricks.CatalogPermission
	for _, cat := range cats {
		eff, err := p.client.Grants.GetEffective(ctx, dbcatalog.GetEffectiveRequest{
			FullName:      cat.FullName,
			SecurableType: dbcatalog.SecurableTypeCatalog,
			Principal:     principalName,
		})
		if err != nil {
			continue
		}
		if eff == nil {
			continue
		}
		seen := make(map[string]bool)
		var privs []string
		for _, assign := range eff.PrivilegeAssignments {
			for _, priv := range assign.Privileges {
				s := string(priv.Privilege)
				if s != "" && !seen[s] {
					seen[s] = true
					privs = append(privs, s)
				}
			}
		}
		if len(privs) > 0 {
			result = append(result, databricks.CatalogPermission{
				CatalogName: cat.FullName,
				Privileges:  privs,
			})
		}
	}
	return result, nil
}

// GetSPPermissions returns workspace-level ACL entries for a service principal.
// Soft-fails (returns nil, nil) if the Permissions API is unsupported.
func (p *SDKIdentityProvider) GetSPPermissions(ctx context.Context, spID string) ([]databricks.SPAccessEntry, error) {
	perms, err := p.client.Permissions.GetByRequestObjectTypeAndRequestObjectId(ctx, "servicePrincipals", spID)
	if err != nil {
		return nil, nil // soft-fail: unsupported object type or permission denied
	}
	var out []databricks.SPAccessEntry
	for _, acl := range perms.AccessControlList {
		for _, perm := range acl.AllPermissions {
			out = append(out, databricks.SPAccessEntry{
				UserName:  acl.UserName,
				GroupName: acl.GroupName,
				Level:     string(perm.PermissionLevel),
			})
		}
	}
	return out, nil
}

func scimComplexValues(vals []iam.ComplexValue) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v.Value != "" {
			out = append(out, v.Value)
		}
	}
	return out
}

func scimComplexDisplays(vals []iam.ComplexValue) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v.Display != "" {
			out = append(out, v.Display)
		}
	}
	return out
}

// memberTypeFromRef resolves the member type from the SCIM $ref URL or the
// explicit type field. The $ref typically looks like "../Users/123" or
// "../Groups/456" or "../ServicePrincipals/789".
func memberTypeFromRef(ref, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch {
	case strings.Contains(ref, "/Users/"):
		return "User"
	case strings.Contains(ref, "/Groups/"):
		return "Group"
	case strings.Contains(ref, "/ServicePrincipals/"):
		return "ServicePrincipal"
	default:
		return "Unknown"
	}
}

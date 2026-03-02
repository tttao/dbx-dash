package sdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbsdk "github.com/databricks/databricks-sdk-go"
	dbcatalog "github.com/databricks/databricks-sdk-go/service/catalog"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// SDKCatalogProvider implements databricks.CatalogProvider using Unity Catalog APIs.
type SDKCatalogProvider struct {
	client *dbsdk.WorkspaceClient
}

func (p *SDKCatalogProvider) ListCatalogs(ctx context.Context) ([]databricks.CatalogInfo, error) {
	all, err := p.client.Catalogs.ListAll(ctx, dbcatalog.ListCatalogsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list catalogs: %w", err)
	}
	out := make([]databricks.CatalogInfo, 0, len(all))
	for _, c := range all {
		out = append(out, databricks.CatalogInfo{
			Name:    c.Name,
			Comment: c.Comment,
			Owner:   c.Owner,
		})
	}
	return out, nil
}

func (p *SDKCatalogProvider) ListSchemas(ctx context.Context, catalogName string) ([]databricks.SchemaInfo, error) {
	all, err := p.client.Schemas.ListAll(ctx, dbcatalog.ListSchemasRequest{CatalogName: catalogName})
	if err != nil {
		return nil, fmt.Errorf("list schemas for %s: %w", catalogName, err)
	}
	out := make([]databricks.SchemaInfo, 0, len(all))
	for _, s := range all {
		out = append(out, databricks.SchemaInfo{
			FullName:    s.FullName,
			Name:        s.Name,
			CatalogName: s.CatalogName,
			Owner:       s.Owner,
		})
	}
	return out, nil
}

func (p *SDKCatalogProvider) ListTables(ctx context.Context, catalogName, schemaName string) ([]databricks.TableInfo, error) {
	all, err := p.client.Tables.ListAll(ctx, dbcatalog.ListTablesRequest{
		CatalogName: catalogName,
		SchemaName:  schemaName,
	})
	if err != nil {
		return nil, fmt.Errorf("list tables for %s.%s: %w", catalogName, schemaName, err)
	}
	out := make([]databricks.TableInfo, 0, len(all))
	for _, t := range all {
		out = append(out, databricks.TableInfo{
			FullName:    t.FullName,
			Name:        t.Name,
			SchemaName:  t.SchemaName,
			CatalogName: t.CatalogName,
			TableType:   string(t.TableType),
			Owner:       t.Owner,
		})
	}
	return out, nil
}

// GetObjectDetail fetches full metadata and permission hierarchy for a UC object.
func (p *SDKCatalogProvider) GetObjectDetail(ctx context.Context, kind, fullName string) (*databricks.ObjectDetail, error) {
	switch kind {
	case "catalog":
		return p.getCatalogDetail(ctx, fullName)
	case "schema":
		return p.getSchemaDetail(ctx, fullName)
	case "table":
		return p.getTableDetail(ctx, fullName)
	default:
		return nil, fmt.Errorf("unknown object kind: %s", kind)
	}
}

func (p *SDKCatalogProvider) getCatalogDetail(ctx context.Context, name string) (*databricks.ObjectDetail, error) {
	cat, err := p.client.Catalogs.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get catalog %s: %w", name, err)
	}
	d := &databricks.ObjectDetail{
		Kind:        "catalog",
		FullName:    name,
		Name:        cat.Name,
		Owner:       cat.Owner,
		Comment:     cat.Comment,
		StorageRoot: cat.StorageRoot,
		CreatedAt:   millisToTime(cat.CreatedAt),
		UpdatedAt:   millisToTime(cat.UpdatedAt),
	}
	d.DirectGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeCatalog, name)
	return d, nil
}

func (p *SDKCatalogProvider) getSchemaDetail(ctx context.Context, fullName string) (*databricks.ObjectDetail, error) {
	sch, err := p.client.Schemas.GetByFullName(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("get schema %s: %w", fullName, err)
	}
	catName := sch.CatalogName
	d := &databricks.ObjectDetail{
		Kind:            "schema",
		FullName:        fullName,
		Name:            sch.Name,
		Owner:           sch.Owner,
		Comment:         sch.Comment,
		StorageLocation: sch.StorageLocation,
		CreatedAt:       millisToTime(sch.CreatedAt),
		UpdatedAt:       millisToTime(sch.UpdatedAt),
	}
	d.DirectGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeSchema, fullName)
	d.GrandpaGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeCatalog, catName)
	return d, nil
}

func (p *SDKCatalogProvider) getTableDetail(ctx context.Context, fullName string) (*databricks.ObjectDetail, error) {
	tbl, err := p.client.Tables.GetByFullName(ctx, fullName)
	if err != nil {
		return nil, fmt.Errorf("get table %s: %w", fullName, err)
	}
	parts := strings.SplitN(fullName, ".", 3)
	catName := ""
	schFullName := ""
	if len(parts) == 3 {
		catName = parts[0]
		schFullName = parts[0] + "." + parts[1]
	}
	d := &databricks.ObjectDetail{
		Kind:            "table",
		FullName:        fullName,
		Name:            tbl.Name,
		Owner:           tbl.Owner,
		Comment:         tbl.Comment,
		StorageLocation: tbl.StorageLocation,
		DataFormat:      string(tbl.DataSourceFormat),
		TableType:       string(tbl.TableType),
		ViewDefinition:  tbl.ViewDefinition,
		CreatedAt:       millisToTime(tbl.CreatedAt),
		UpdatedAt:       millisToTime(tbl.UpdatedAt),
	}
	d.DirectGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeTable, fullName)
	if schFullName != "" {
		d.ParentGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeSchema, schFullName)
	}
	if catName != "" {
		d.GrandpaGrants, _ = p.fetchGrants(ctx, dbcatalog.SecurableTypeCatalog, catName)
	}
	return d, nil
}

// fetchGrants retrieves direct grants for a securable.
// Returns nil, nil on permission errors so callers can display gracefully.
// Transitive expansion (group membership) is done client-side using the
// pre-fetched workspace group hierarchy.
func (p *SDKCatalogProvider) fetchGrants(ctx context.Context, secType dbcatalog.SecurableType, fullName string) ([]databricks.GrantEntry, error) {
	perms, err := p.client.Grants.GetBySecurableTypeAndFullName(ctx, secType, fullName)
	if err != nil {
		return nil, err // non-fatal: caller ignores error
	}
	if perms == nil {
		return nil, nil
	}
	out := make([]databricks.GrantEntry, 0, len(perms.PrivilegeAssignments))
	for _, pa := range perms.PrivilegeAssignments {
		privs := make([]string, 0, len(pa.Privileges))
		for _, priv := range pa.Privileges {
			privs = append(privs, string(priv))
		}
		out = append(out, databricks.GrantEntry{Principal: pa.Principal, Privileges: privs})
	}
	return out, nil
}

func millisToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

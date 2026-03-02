// Package mock provides in-memory implementations of the databricks provider
// interfaces for use in unit tests.
package mock

import (
	"context"

	"github.com/tttao/dbx-dash/internal/databricks"
)

// JobsProvider is a mock implementation of databricks.JobsProvider.
type JobsProvider struct {
	Jobs map[int64]databricks.Job
	Runs map[int64][]databricks.JobRun   // key: jobID
	Outputs map[int64]*databricks.RunOutput // key: runID
	Err  error
}

func (m *JobsProvider) ListJobs(_ context.Context, _ int) ([]databricks.Job, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]databricks.Job, 0, len(m.Jobs))
	for _, j := range m.Jobs {
		out = append(out, j)
	}
	return out, nil
}

func (m *JobsProvider) GetJob(_ context.Context, jobID int64) (*databricks.Job, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	j, ok := m.Jobs[jobID]
	if !ok {
		return nil, nil
	}
	return &j, nil
}

func (m *JobsProvider) ListRecentRuns(_ context.Context, jobID int64, _ int) ([]databricks.JobRun, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Runs[jobID], nil
}

func (m *JobsProvider) GetRunOutput(_ context.Context, runID int64) (*databricks.RunOutput, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Outputs[runID], nil
}

// ClustersProvider is a mock implementation of databricks.ClustersProvider.
type ClustersProvider struct {
	Clusters map[string]databricks.Cluster
	Err      error
}

func (m *ClustersProvider) ListClusters(_ context.Context) ([]databricks.Cluster, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]databricks.Cluster, 0, len(m.Clusters))
	for _, c := range m.Clusters {
		out = append(out, c)
	}
	return out, nil
}

func (m *ClustersProvider) GetCluster(_ context.Context, clusterID string) (*databricks.Cluster, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	c, ok := m.Clusters[clusterID]
	if !ok {
		return nil, nil
	}
	return &c, nil
}

// WarehousesProvider is a mock implementation of databricks.WarehousesProvider.
type WarehousesProvider struct {
	Warehouses map[string]databricks.SqlWarehouse
	Err        error
}

func (m *WarehousesProvider) ListWarehouses(_ context.Context) ([]databricks.SqlWarehouse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]databricks.SqlWarehouse, 0, len(m.Warehouses))
	for _, w := range m.Warehouses {
		out = append(out, w)
	}
	return out, nil
}

func (m *WarehousesProvider) GetWarehouse(_ context.Context, warehouseID string) (*databricks.SqlWarehouse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	w, ok := m.Warehouses[warehouseID]
	if !ok {
		return nil, nil
	}
	return &w, nil
}

// PipelinesProvider is a mock implementation of databricks.PipelinesProvider.
type PipelinesProvider struct {
	Pipelines map[string]databricks.Pipeline
	Updates   map[string][]databricks.PipelineUpdate // key: pipelineID
	Err       error
}

func (m *PipelinesProvider) ListPipelines(_ context.Context) ([]databricks.Pipeline, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	out := make([]databricks.Pipeline, 0, len(m.Pipelines))
	for _, p := range m.Pipelines {
		out = append(out, p)
	}
	return out, nil
}

func (m *PipelinesProvider) GetPipeline(_ context.Context, pipelineID string) (*databricks.Pipeline, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	p, ok := m.Pipelines[pipelineID]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (m *PipelinesProvider) ListPipelineUpdates(_ context.Context, pipelineID string, _ int) ([]databricks.PipelineUpdate, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Updates[pipelineID], nil
}

// IdentityProvider is a mock implementation of databricks.IdentityProvider.
type IdentityProvider struct {
	Groups            []databricks.Group
	Users             []databricks.IdentityUser
	ServicePrincipals []databricks.WorkspaceServicePrincipal
	UserDetails       map[string]*databricks.UserDetail
	SPDetails         map[string]*databricks.SPDetail
	CatalogPerms      map[string][]databricks.CatalogPermission // key: principalName
	SPPermissions     map[string][]databricks.SPAccessEntry     // key: spID
	Err               error
}

func (m *IdentityProvider) ListGroups(_ context.Context) ([]databricks.Group, error) {
	return m.Groups, m.Err
}

func (m *IdentityProvider) ListUsers(_ context.Context) ([]databricks.IdentityUser, error) {
	return m.Users, m.Err
}

func (m *IdentityProvider) ListServicePrincipals(_ context.Context) ([]databricks.WorkspaceServicePrincipal, error) {
	return m.ServicePrincipals, m.Err
}

func (m *IdentityProvider) GetUser(_ context.Context, userID string) (*databricks.UserDetail, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.UserDetails[userID], nil
}

func (m *IdentityProvider) GetServicePrincipal(_ context.Context, spID string) (*databricks.SPDetail, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.SPDetails[spID], nil
}

func (m *IdentityProvider) GetCatalogPermissions(_ context.Context, principalName string) ([]databricks.CatalogPermission, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.CatalogPerms[principalName], nil
}

func (m *IdentityProvider) GetSPPermissions(_ context.Context, spID string) ([]databricks.SPAccessEntry, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.SPPermissions[spID], nil
}

// CatalogProvider is a mock implementation of databricks.CatalogProvider.
type CatalogProvider struct {
	Catalogs      []databricks.CatalogInfo
	Schemas       map[string][]databricks.SchemaInfo  // key: catalogName
	Tables        map[string][]databricks.TableInfo   // key: "catalogName.schemaName"
	ObjectDetails map[string]*databricks.ObjectDetail // key: fullName
	Err           error
}

func (m *CatalogProvider) ListCatalogs(_ context.Context) ([]databricks.CatalogInfo, error) {
	return m.Catalogs, m.Err
}

func (m *CatalogProvider) ListSchemas(_ context.Context, catalogName string) ([]databricks.SchemaInfo, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Schemas[catalogName], nil
}

func (m *CatalogProvider) ListTables(_ context.Context, catalogName, schemaName string) ([]databricks.TableInfo, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Tables[catalogName+"."+schemaName], nil
}

func (m *CatalogProvider) GetObjectDetail(_ context.Context, _, fullName string) (*databricks.ObjectDetail, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.ObjectDetails[fullName], nil
}

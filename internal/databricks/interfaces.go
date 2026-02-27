package databricks

import "context"

// JobsProvider is the interface for all job-related Databricks operations.
// Production code uses the SDK implementation; tests use the mock.
type JobsProvider interface {
	ListJobs(ctx context.Context, limit int) ([]Job, error)
	GetJob(ctx context.Context, jobID int64) (*Job, error)
	ListRecentRuns(ctx context.Context, jobID int64, limit int) ([]JobRun, error)
	GetRunOutput(ctx context.Context, runID int64) (*RunOutput, error)
}

// ClustersProvider is the interface for cluster-related operations.
type ClustersProvider interface {
	ListClusters(ctx context.Context) ([]Cluster, error)
	GetCluster(ctx context.Context, clusterID string) (*Cluster, error)
}

// WarehousesProvider is the interface for SQL warehouse operations.
type WarehousesProvider interface {
	ListWarehouses(ctx context.Context) ([]SqlWarehouse, error)
	GetWarehouse(ctx context.Context, warehouseID string) (*SqlWarehouse, error)
}

// PipelinesProvider is the interface for DLT pipeline operations.
type PipelinesProvider interface {
	ListPipelines(ctx context.Context) ([]Pipeline, error)
	GetPipeline(ctx context.Context, pipelineID string) (*Pipeline, error)
	ListPipelineUpdates(ctx context.Context, pipelineID string, limit int) ([]PipelineUpdate, error)
}

// IdentityProvider is the interface for workspace-level identity (SCIM) operations.
// Requires workspace admin access; no account admin needed.
type IdentityProvider interface {
	ListGroups(ctx context.Context) ([]Group, error)
	ListUsers(ctx context.Context) ([]IdentityUser, error)
	ListServicePrincipals(ctx context.Context) ([]WorkspaceServicePrincipal, error)
	GetUser(ctx context.Context, userID string) (*UserDetail, error)
	GetServicePrincipal(ctx context.Context, spID string) (*SPDetail, error)
	// GetCatalogPermissions returns Unity Catalog grants for the given principal at
	// catalog level. principalName is the user's email or SP's applicationId string.
	// Returns nil (not error) when Unity Catalog is unavailable.
	GetCatalogPermissions(ctx context.Context, principalName string) ([]CatalogPermission, error)
}

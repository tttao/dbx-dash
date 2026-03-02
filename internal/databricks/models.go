// Package databricks defines the domain models and provider interfaces used
// throughout dbx-dash. Concrete implementations live in sub-packages (sdk, mock).
package databricks

import "time"

// Job represents a Databricks job definition.
type Job struct {
	JobID       int64
	Name        string
	Creator     string
	CreatedTime time.Time
}

// JobRun represents a single execution of a job.
type JobRun struct {
	RunID       int64
	JobID       int64
	// Life-cycle state: PENDING, RUNNING, TERMINATING, TERMINATED, SKIPPED, INTERNAL_ERROR
	State       string
	// Terminal result: SUCCESS, FAILED, TIMEDOUT, CANCELED (empty while running)
	ResultState string
	StartTime   time.Time
	EndTime     time.Time
	DurationMs  int64
	RunPageURL  string
}

// RunOutput holds the output and logs for a completed job run.
type RunOutput struct {
	RunID      int64
	Metadata   *JobRun
	Error      string
	ErrorTrace string
	Logs       string
}

// Cluster represents a Databricks all-purpose or job cluster.
type Cluster struct {
	ClusterID    string
	Name         string
	// State: PENDING, RUNNING, RESTARTING, RESIZING, TERMINATING, TERMINATED, ERROR, UNKNOWN
	State        string
	Source       string
	DriverTypeID string
	NodeTypeID   string
	NumWorkers   int
	AutoscaleMin int
	AutoscaleMax int
	SparkVersion string
	Creator      string
	StartTime    time.Time
}

// SqlWarehouse represents a Databricks SQL warehouse.
type SqlWarehouse struct {
	WarehouseID    string
	Name           string
	// State: STARTING, RUNNING, STOPPING, STOPPED, DELETING, DELETED
	State          string
	ClusterSize    string
	MinClusters    int
	MaxClusters    int
	AutoStopMins   int
	Creator        string
	ActiveSessions int
}

// Pipeline represents a Delta Live Tables pipeline.
type Pipeline struct {
	PipelineID   string
	Name         string
	// State: IDLE, RUNNING, DEPLOYING, FAILED, DELETING, RECOVERING
	State        string
	ClusterID    string
	Creator      string
	LastModified time.Time
}

// PipelineUpdate represents a single DLT pipeline update run.
type PipelineUpdate struct {
	UpdateID    string
	PipelineID  string
	State       string
	StartTime   time.Time
	FullRefresh bool
	Cause       string
}

// GroupMember is a member of a workspace group (user, service principal, or nested group).
type GroupMember struct {
	ID          string
	DisplayName string
	// Type is "User", "Group", or "ServicePrincipal"
	Type        string
}

// Group is a workspace-level SCIM group.
type Group struct {
	ID          string
	DisplayName string
	Members     []GroupMember
}

// IdentityUser is a workspace-level SCIM user.
type IdentityUser struct {
	ID          string
	UserName    string
	DisplayName string
	Active      bool
}

// WorkspaceServicePrincipal is a workspace-level service principal.
type WorkspaceServicePrincipal struct {
	ID            string
	ApplicationID string
	DisplayName   string
	Active        bool
}

// CatalogPermission represents Unity Catalog grants for a principal on a catalog.
type CatalogPermission struct {
	CatalogName string
	Privileges  []string
}

// SPAccessEntry is one entry in a service principal's workspace permission ACL.
// Exactly one of UserName or GroupName is non-empty.
type SPAccessEntry struct {
	UserName  string // non-empty when a user has been granted access
	GroupName string // non-empty when a group has been granted access
	Level     string // e.g. "CAN_USE", "CAN_MANAGE", "IS_OWNER"
}

// SPUserAccess records one service principal reachable by a user via group co-membership.
type SPUserAccess struct {
	SPID        string
	DisplayName string
	ViaGroups   []string // group DisplayNames linking user ↔ SP
}

// UserDetail holds the full SCIM detail for a workspace user, including permissions.
type UserDetail struct {
	ID           string
	UserName     string
	DisplayName  string
	Active       bool
	Entitlements []string // e.g. "workspace-access", "allow-cluster-create"
	// Groups contains direct SCIM group memberships (display names).
	// Used as fallback when workspace group data is unavailable for transitive lookup.
	Groups             []string
	Roles              []string // e.g. "admin"
	CatalogPermissions []CatalogPermission // nil = UC not available; empty slice = no grants
	SPAccess           []SPUserAccess      // SPs reachable via group co-membership
}

// SPDetail holds the full SCIM detail for a workspace service principal.
type SPDetail struct {
	ID            string
	ApplicationID string
	DisplayName   string
	Active        bool
	Entitlements  []string
	// Groups contains direct SCIM group memberships (display names).
	Groups             []string
	Roles              []string
	CatalogPermissions []CatalogPermission
	AccessControl      []SPAccessEntry // workspace-level permission ACL; nil = API not supported
}

// CatalogInfo represents a Unity Catalog catalog.
type CatalogInfo struct {
	Name    string
	Comment string
	Owner   string
}

// SchemaInfo represents a Unity Catalog schema.
type SchemaInfo struct {
	FullName    string // "catalog.schema"
	Name        string
	CatalogName string
	Owner       string
}

// TableInfo represents a Unity Catalog table or view.
type TableInfo struct {
	FullName    string // "catalog.schema.table"
	Name        string
	SchemaName  string
	CatalogName string
	// TableType: TABLE, VIEW, MATERIALIZED_VIEW, STREAMING_TABLE, etc.
	TableType string
	Owner     string
}

// GrantEntry is a single principal's direct grant on a Unity Catalog securable.
// Via is non-empty for entries added by client-side group expansion (indirect access).
type GrantEntry struct {
	Principal  string
	Privileges []string
	Via        string // display name of the group that grants this access indirectly; "" = direct
}

// ObjectDetail holds the full metadata and permission hierarchy for a
// Unity Catalog object (catalog, schema, or table).
type ObjectDetail struct {
	Kind            string // "catalog", "schema", "table"
	FullName        string
	Name            string
	Owner           string
	Comment         string
	StorageLocation string
	StorageRoot     string // catalogs and schemas
	DataFormat      string // tables: DELTA, PARQUET, CSV, etc.
	TableType       string // TABLE, VIEW, MATERIALIZED_VIEW, etc.
	ViewDefinition  string // views only
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// Grants hierarchy (nil = not applicable or not fetched).
	DirectGrants  []GrantEntry // grants on this object
	ParentGrants  []GrantEntry // schema grants (tables only)
	GrandpaGrants []GrantEntry // catalog grants (tables and schemas)
}

// WorkspaceProviders bundles all providers for a single workspace.
type WorkspaceProviders struct {
	WorkspaceName string
	Jobs          JobsProvider
	Clusters      ClustersProvider
	Warehouses    WarehousesProvider
	Pipelines     PipelinesProvider
	Identity      IdentityProvider
	Catalog       CatalogProvider
}

package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tttao/dbx-dash/internal/databricks"
)

// JobSnapshot is a persisted summary of a job's most recent run.
type JobSnapshot struct {
	Workspace     string
	JobID         int64
	JobName       string
	LastRunID     int64
	LastRunState  string
	LastRunResult string
	DurationMs    int64
	FetchedAt     time.Time
}

// ClusterSnapshot is a persisted summary of a cluster.
type ClusterSnapshot struct {
	Workspace   string
	ClusterID   string
	ClusterName string
	State       string
	NumWorkers  int
	FetchedAt   time.Time
}

// Repository provides read/write access to cache snapshots.
type Repository struct {
	db *sql.DB
}

// NewRepository wraps an open database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// UpsertJob writes (or updates) a job snapshot.
func (r *Repository) UpsertJob(workspace string, j databricks.Job, run *databricks.JobRun) error {
	var runID int64
	var state, result string
	var dur int64
	if run != nil {
		runID = run.RunID
		state = run.State
		result = run.ResultState
		dur = run.DurationMs
	}
	_, err := r.db.Exec(`
		INSERT INTO job_snapshots
			(workspace, job_id, job_name, last_run_id, last_run_state, last_run_result, duration_ms, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace, job_id) DO UPDATE SET
			job_name       = excluded.job_name,
			last_run_id    = excluded.last_run_id,
			last_run_state = excluded.last_run_state,
			last_run_result = excluded.last_run_result,
			duration_ms    = excluded.duration_ms,
			fetched_at     = excluded.fetched_at
	`, workspace, j.JobID, j.Name, runID, state, result, dur)
	if err != nil {
		return fmt.Errorf("upsert job snapshot: %w", err)
	}
	return nil
}

// GetJobs returns all cached job snapshots for a workspace.
func (r *Repository) GetJobs(workspace string) ([]JobSnapshot, error) {
	rows, err := r.db.Query(`
		SELECT workspace, job_id, job_name, last_run_id, last_run_state, last_run_result, duration_ms, fetched_at
		FROM job_snapshots WHERE workspace = ?
		ORDER BY job_name`, workspace)
	if err != nil {
		return nil, fmt.Errorf("get jobs: %w", err)
	}
	defer rows.Close()

	var out []JobSnapshot
	for rows.Next() {
		var s JobSnapshot
		if err := rows.Scan(
			&s.Workspace, &s.JobID, &s.JobName,
			&s.LastRunID, &s.LastRunState, &s.LastRunResult,
			&s.DurationMs, &s.FetchedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpsertCluster writes (or updates) a cluster snapshot.
func (r *Repository) UpsertCluster(workspace string, c databricks.Cluster) error {
	_, err := r.db.Exec(`
		INSERT INTO cluster_snapshots
			(workspace, cluster_id, cluster_name, state, num_workers, fetched_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace, cluster_id) DO UPDATE SET
			cluster_name = excluded.cluster_name,
			state        = excluded.state,
			num_workers  = excluded.num_workers,
			fetched_at   = excluded.fetched_at
	`, workspace, c.ClusterID, c.Name, c.State, c.NumWorkers)
	if err != nil {
		return fmt.Errorf("upsert cluster snapshot: %w", err)
	}
	return nil
}

// GetJobsAsDomain returns cached jobs as domain types (best-effort: run info is partial).
func (r *Repository) GetJobsAsDomain(workspace string) ([]databricks.Job, map[int64]*databricks.JobRun, error) {
	snaps, err := r.GetJobs(workspace)
	if err != nil {
		return nil, nil, err
	}
	jobs := make([]databricks.Job, 0, len(snaps))
	runs := make(map[int64]*databricks.JobRun, len(snaps))
	for _, s := range snaps {
		jobs = append(jobs, databricks.Job{JobID: s.JobID, Name: s.JobName})
		if s.LastRunID != 0 {
			runs[s.JobID] = &databricks.JobRun{
				RunID:       s.LastRunID,
				JobID:       s.JobID,
				State:       s.LastRunState,
				ResultState: s.LastRunResult,
				DurationMs:  s.DurationMs,
			}
		}
	}
	return jobs, runs, nil
}

// GetClusters returns all cached cluster snapshots for a workspace.
func (r *Repository) GetClusters(workspace string) ([]ClusterSnapshot, error) {
	rows, err := r.db.Query(`
		SELECT workspace, cluster_id, cluster_name, state, num_workers, fetched_at
		FROM cluster_snapshots WHERE workspace = ?
		ORDER BY cluster_name`, workspace)
	if err != nil {
		return nil, fmt.Errorf("get clusters: %w", err)
	}
	defer rows.Close()

	var out []ClusterSnapshot
	for rows.Next() {
		var s ClusterSnapshot
		if err := rows.Scan(
			&s.Workspace, &s.ClusterID, &s.ClusterName,
			&s.State, &s.NumWorkers, &s.FetchedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpsertIdentity persists the workspace group hierarchy to the cache.
// Only groups (with their members) are stored — sufficient for client-side
// transitive permission expansion.
func (r *Repository) UpsertIdentity(workspace string, groups []databricks.Group) error {
	b, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}
	_, err = r.db.Exec(`
		INSERT INTO identity_cache (workspace, groups_json, fetched_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace) DO UPDATE SET
			groups_json = excluded.groups_json,
			fetched_at  = excluded.fetched_at
	`, workspace, string(b))
	if err != nil {
		return fmt.Errorf("upsert identity cache: %w", err)
	}
	return nil
}

// GetIdentityGroups returns cached groups for a workspace, or nil if not cached.
func (r *Repository) GetIdentityGroups(workspace string) ([]databricks.Group, error) {
	var raw string
	err := r.db.QueryRow(
		`SELECT groups_json FROM identity_cache WHERE workspace = ?`, workspace,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get identity cache: %w", err)
	}
	var groups []databricks.Group
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("unmarshal groups: %w", err)
	}
	return groups, nil
}

// GetClustersAsDomain returns cached clusters as domain types.
func (r *Repository) GetClustersAsDomain(workspace string) ([]databricks.Cluster, error) {
	snaps, err := r.GetClusters(workspace)
	if err != nil {
		return nil, err
	}
	out := make([]databricks.Cluster, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, databricks.Cluster{
			ClusterID:  s.ClusterID,
			Name:       s.ClusterName,
			State:      s.State,
			NumWorkers: s.NumWorkers,
		})
	}
	return out, nil
}

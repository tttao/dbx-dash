package sdk

import (
	"context"
	"fmt"
	"time"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/listing"
	"github.com/databricks/databricks-sdk-go/service/jobs"
	"github.com/you/dbx-dash/internal/databricks"
)

// SDKJobsProvider implements databricks.JobsProvider using databricks-sdk-go.
type SDKJobsProvider struct {
	client *dbsdk.WorkspaceClient
}

const maxJobsPageSize = 100 // SDK maximum for ListJobsRequest.Limit
const maxRunsPageSize = 24  // SDK maximum for ListRunsRequest.Limit (must be < 25)

func (p *SDKJobsProvider) ListJobs(ctx context.Context, limit int) ([]databricks.Job, error) {
	req := jobs.ListJobsRequest{}
	if limit > 0 && limit <= maxJobsPageSize {
		req.Limit = limit
	} else if limit > maxJobsPageSize {
		req.Limit = maxJobsPageSize
	}
	all, err := p.client.Jobs.ListAll(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	out := make([]databricks.Job, 0, len(all))
	for _, j := range all {
		out = append(out, databricks.Job{
			JobID:       j.JobId,
			Name:        j.Settings.Name,
			Creator:     j.CreatorUserName,
			CreatedTime: msToTime(j.CreatedTime),
		})
	}
	return out, nil
}

func (p *SDKJobsProvider) GetJob(ctx context.Context, jobID int64) (*databricks.Job, error) {
	j, err := p.client.Jobs.GetByJobId(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get job %d: %w", jobID, err)
	}
	return &databricks.Job{
		JobID:       j.JobId,
		Name:        j.Settings.Name,
		Creator:     j.CreatorUserName,
		CreatedTime: msToTime(j.CreatedTime),
	}, nil
}

// ListRecentRuns returns at most `limit` recent runs for a job.
// Uses the lazy iterator from ListRuns and listing.ToSliceN to stop early,
// avoiding a full paginated scan when only a few runs are needed.
// Limit is capped at maxRunsPageSize (24).
func (p *SDKJobsProvider) ListRecentRuns(ctx context.Context, jobID int64, limit int) ([]databricks.JobRun, error) {
	if limit <= 0 || limit > maxRunsPageSize {
		limit = maxRunsPageSize
	}
	req := jobs.ListRunsRequest{JobId: jobID, Limit: limit}
	iter := p.client.Jobs.ListRuns(ctx, req)
	page, err := listing.ToSliceN(ctx, iter, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs for job %d: %w", jobID, err)
	}
	out := make([]databricks.JobRun, 0, len(page))
	for _, r := range page {
		out = append(out, baseRunToModel(r))
	}
	return out, nil
}

func (p *SDKJobsProvider) GetRunOutput(ctx context.Context, runID int64) (*databricks.RunOutput, error) {
	o, err := p.client.Jobs.GetRunOutput(ctx, jobs.GetRunOutputRequest{RunId: runID})
	if err != nil {
		return nil, fmt.Errorf("get run output %d: %w", runID, err)
	}
	out := &databricks.RunOutput{
		RunID:      runID,
		Error:      o.Error,
		ErrorTrace: o.ErrorTrace,
		Logs:       o.Logs,
	}
	if o.Metadata != nil {
		m := runToModel(*o.Metadata)
		out.Metadata = &m
	}
	return out, nil
}

// baseRunToModel converts a jobs.BaseRun (returned by ListRunsAll) to domain model.
func baseRunToModel(r jobs.BaseRun) databricks.JobRun {
	run := databricks.JobRun{
		RunID:      r.RunId,
		JobID:      r.JobId,
		RunPageURL: r.RunPageUrl,
		StartTime:  msToTime(r.StartTime),
		DurationMs: r.ExecutionDuration,
		EndTime:    msToTime(r.EndTime),
	}
	if r.State != nil {
		run.State = string(r.State.LifeCycleState)
		run.ResultState = string(r.State.ResultState)
	}
	return run
}

// runToModel converts a jobs.Run (returned by GetRunOutput.Metadata) to domain model.
func runToModel(r jobs.Run) databricks.JobRun {
	run := databricks.JobRun{
		RunID:      r.RunId,
		JobID:      r.JobId,
		RunPageURL: r.RunPageUrl,
		StartTime:  msToTime(r.StartTime),
		DurationMs: r.ExecutionDuration,
		EndTime:    msToTime(r.EndTime),
	}
	if r.State != nil {
		run.State = string(r.State.LifeCycleState)
		run.ResultState = string(r.State.ResultState)
	}
	return run
}

func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

package sdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/listing"
	"github.com/databricks/databricks-sdk-go/service/jobs"
	"github.com/tttao/dbx-dash/internal/databricks"
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
	// Fetch the run to get metadata and the list of task runs.
	run, err := p.client.Jobs.GetRun(ctx, jobs.GetRunRequest{RunId: runID})
	if err != nil {
		return nil, fmt.Errorf("get run %d: %w", runID, err)
	}

	base := &databricks.RunOutput{RunID: runID}
	if run != nil {
		m := runToModel(*run)
		base.Metadata = &m
	}

	// Collect task runs that have been assigned a run ID.
	taskRuns := make([]jobs.RunTask, 0, len(run.Tasks))
	for _, t := range run.Tasks {
		if t.RunId != 0 {
			taskRuns = append(taskRuns, t)
		}
	}

	if len(taskRuns) > 1 {
		return p.fetchMultiTaskOutput(ctx, base, taskRuns)
	}

	// Single-task or legacy run: use the direct output endpoint.
	// If the API still complains about multiple tasks (tasks not yet reflected
	// in GetRun response), surface a clear message instead of a raw API error.
	outputRunID := runID
	if len(taskRuns) == 1 {
		outputRunID = taskRuns[0].RunId
	}
	o, err := p.client.Jobs.GetRunOutput(ctx, jobs.GetRunOutputRequest{RunId: outputRunID})
	if err != nil {
		if strings.Contains(err.Error(), "multiple tasks") {
			base.Logs = "(multi-task run — individual task outputs not yet available)"
			return base, nil
		}
		return nil, fmt.Errorf("get run output %d: %w", runID, err)
	}
	base.Error = o.Error
	base.ErrorTrace = o.ErrorTrace
	base.Logs = o.Logs
	if o.Metadata != nil {
		m := runToModel(*o.Metadata)
		base.Metadata = &m
	}
	return base, nil
}

// fetchMultiTaskOutput calls GetRunOutput for each task and concatenates results.
func (p *SDKJobsProvider) fetchMultiTaskOutput(ctx context.Context, base *databricks.RunOutput, taskRuns []jobs.RunTask) (*databricks.RunOutput, error) {
	var logParts []string
	var errParts []string
	for _, task := range taskRuns {
		o, err := p.client.Jobs.GetRunOutput(ctx, jobs.GetRunOutputRequest{RunId: task.RunId})
		if err != nil {
			errParts = append(errParts, fmt.Sprintf("[%s] error: %s", task.TaskKey, err.Error()))
			continue
		}
		if o.Logs != "" {
			logParts = append(logParts, fmt.Sprintf("=== Task: %s ===\n%s", task.TaskKey, o.Logs))
		}
		if o.Error != "" {
			errParts = append(errParts, fmt.Sprintf("[%s] %s", task.TaskKey, o.Error))
		}
		if o.ErrorTrace != "" {
			errParts = append(errParts, fmt.Sprintf("[%s] %s", task.TaskKey, o.ErrorTrace))
		}
	}
	base.Logs = strings.Join(logParts, "\n\n")
	base.Error = strings.Join(errParts, "\n")
	if base.Logs == "" && base.Error == "" {
		base.Logs = "(no output for any task)"
	}
	return base, nil
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

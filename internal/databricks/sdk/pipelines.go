package sdk

import (
	"context"
	"fmt"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/pipelines"
	"github.com/you/dbx-dash/internal/databricks"
)

// SDKPipelinesProvider implements databricks.PipelinesProvider using databricks-sdk-go.
type SDKPipelinesProvider struct {
	client *dbsdk.WorkspaceClient
}

// ListPipelines returns all DLT pipelines. PipelineStateInfo does not carry
// LastModified, so that field is left as zero time.
func (p *SDKPipelinesProvider) ListPipelines(ctx context.Context) ([]databricks.Pipeline, error) {
	all, err := p.client.Pipelines.ListPipelinesAll(ctx, pipelines.ListPipelinesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}
	out := make([]databricks.Pipeline, 0, len(all))
	for _, pl := range all {
		out = append(out, databricks.Pipeline{
			PipelineID: pl.PipelineId,
			Name:       pl.Name,
			State:      string(pl.State),
			ClusterID:  pl.ClusterId,
			Creator:    pl.CreatorUserName,
		})
	}
	return out, nil
}

// GetPipeline returns a single pipeline. GetPipelineResponse carries LastModified.
func (p *SDKPipelinesProvider) GetPipeline(ctx context.Context, pipelineID string) (*databricks.Pipeline, error) {
	pl, err := p.client.Pipelines.GetByPipelineId(ctx, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("get pipeline %s: %w", pipelineID, err)
	}
	return &databricks.Pipeline{
		PipelineID:   pl.PipelineId,
		Name:         pl.Name,
		State:        string(pl.State),
		ClusterID:    pl.ClusterId,
		Creator:      pl.CreatorUserName,
		LastModified: msToTime(pl.LastModified),
	}, nil
}

func (p *SDKPipelinesProvider) ListPipelineUpdates(ctx context.Context, pipelineID string, limit int) ([]databricks.PipelineUpdate, error) {
	req := pipelines.ListUpdatesRequest{PipelineId: pipelineID}
	if limit > 0 {
		req.MaxResults = limit
	}
	resp, err := p.client.Pipelines.ListUpdates(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list updates for pipeline %s: %w", pipelineID, err)
	}
	out := make([]databricks.PipelineUpdate, 0, len(resp.Updates))
	for _, u := range resp.Updates {
		out = append(out, databricks.PipelineUpdate{
			UpdateID:    u.UpdateId,
			PipelineID:  pipelineID,
			State:       string(u.State),
			FullRefresh: u.FullRefresh,
			Cause:       string(u.Cause), // UpdateInfoCause is a named string type
			StartTime:   msToTime(u.CreationTime),
		})
	}
	return out, nil
}

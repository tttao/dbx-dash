package sdk

import (
	"context"
	"fmt"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/compute"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// SDKClustersProvider implements databricks.ClustersProvider using databricks-sdk-go.
type SDKClustersProvider struct {
	client *dbsdk.WorkspaceClient
}

func (p *SDKClustersProvider) ListClusters(ctx context.Context) ([]databricks.Cluster, error) {
	all, err := p.client.Clusters.ListAll(ctx, compute.ListClustersRequest{})
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	out := make([]databricks.Cluster, 0, len(all))
	for _, c := range all {
		out = append(out, clusterToModel(c))
	}
	return out, nil
}

func (p *SDKClustersProvider) GetCluster(ctx context.Context, clusterID string) (*databricks.Cluster, error) {
	c, err := p.client.Clusters.GetByClusterId(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("get cluster %s: %w", clusterID, err)
	}
	m := clusterToModel(*c)
	return &m, nil
}

func clusterToModel(c compute.ClusterDetails) databricks.Cluster {
	cl := databricks.Cluster{
		ClusterID:    c.ClusterId,
		Name:         c.ClusterName,
		State:        string(c.State),
		Source:       string(c.ClusterSource),
		DriverTypeID: c.DriverNodeTypeId,
		NodeTypeID:   c.NodeTypeId,
		NumWorkers:   int(c.NumWorkers),
		SparkVersion: c.SparkVersion,
		Creator:      c.CreatorUserName,
		StartTime:    msToTime(c.StartTime),
	}
	if c.Autoscale != nil {
		cl.AutoscaleMin = int(c.Autoscale.MinWorkers)
		cl.AutoscaleMax = int(c.Autoscale.MaxWorkers)
	}
	return cl
}

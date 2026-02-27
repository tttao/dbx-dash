package sdk

import (
	"context"
	"fmt"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/sql"
	"github.com/you/dbx-dash/internal/databricks"
)

// SDKWarehousesProvider implements databricks.WarehousesProvider using databricks-sdk-go.
type SDKWarehousesProvider struct {
	client *dbsdk.WorkspaceClient
}

func (p *SDKWarehousesProvider) ListWarehouses(ctx context.Context) ([]databricks.SqlWarehouse, error) {
	resp, err := p.client.Warehouses.ListAll(ctx, sql.ListWarehousesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list warehouses: %w", err)
	}
	out := make([]databricks.SqlWarehouse, 0, len(resp))
	for _, w := range resp {
		out = append(out, warehouseToModel(w))
	}
	return out, nil
}

func (p *SDKWarehousesProvider) GetWarehouse(ctx context.Context, warehouseID string) (*databricks.SqlWarehouse, error) {
	w, err := p.client.Warehouses.GetById(ctx, warehouseID)
	if err != nil {
		return nil, fmt.Errorf("get warehouse %s: %w", warehouseID, err)
	}
	m := warehouseToModel(sql.EndpointInfo{
		Id:                w.Id,
		Name:              w.Name,
		State:             sql.State(w.State),
		ClusterSize:       w.ClusterSize,
		MinNumClusters:    w.MinNumClusters,
		MaxNumClusters:    w.MaxNumClusters,
		AutoStopMins:      w.AutoStopMins,
		CreatorName:       w.CreatorName,
		NumActiveSessions: w.NumActiveSessions,
	})
	return &m, nil
}

func warehouseToModel(w sql.EndpointInfo) databricks.SqlWarehouse {
	return databricks.SqlWarehouse{
		WarehouseID:    w.Id,
		Name:           w.Name,
		State:          string(w.State),
		ClusterSize:    w.ClusterSize,
		MinClusters:    int(w.MinNumClusters),
		MaxClusters:    int(w.MaxNumClusters),
		AutoStopMins:   int(w.AutoStopMins),
		Creator:        w.CreatorName,
		ActiveSessions: int(w.NumActiveSessions),
	}
}

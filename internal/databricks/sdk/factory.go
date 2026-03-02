// Package sdk provides databricks-sdk-go backed implementations of the
// provider interfaces defined in internal/databricks.
package sdk

import (
	"context"
	"fmt"
	"sync"

	dbsdk "github.com/databricks/databricks-sdk-go"
	"github.com/tttao/dbx-dash/internal/config"
	"github.com/tttao/dbx-dash/internal/databricks"
)

// WorkspaceClientFactory creates and caches SDK workspace clients, one per
// profile. Clients are initialised lazily on first use so auth errors surface
// per-workspace rather than at startup.
type WorkspaceClientFactory struct {
	mu      sync.Mutex
	clients map[string]*dbsdk.WorkspaceClient
}

// NewWorkspaceClientFactory returns a ready-to-use factory.
func NewWorkspaceClientFactory() *WorkspaceClientFactory {
	return &WorkspaceClientFactory{
		clients: make(map[string]*dbsdk.WorkspaceClient),
	}
}

// GetProviders returns a WorkspaceProviders bundle backed by the SDK client
// for the given profile. The client is created once and reused on subsequent
// calls for the same profile name.
func (f *WorkspaceClientFactory) GetProviders(p config.WorkspaceProfile) (*databricks.WorkspaceProviders, error) {
	client, err := f.getOrCreate(p)
	if err != nil {
		return nil, fmt.Errorf("workspace %q: %w", p.Name, err)
	}
	return &databricks.WorkspaceProviders{
		WorkspaceName: p.Name,
		Jobs:          &SDKJobsProvider{client: client},
		Clusters:      &SDKClustersProvider{client: client},
		Warehouses:    &SDKWarehousesProvider{client: client},
		Pipelines:     &SDKPipelinesProvider{client: client},
		Identity:      &SDKIdentityProvider{client: client},
		Catalog:       &SDKCatalogProvider{client: client},
	}, nil
}

// ValidateAuth makes a lightweight API call to confirm credentials are valid.
func (f *WorkspaceClientFactory) ValidateAuth(ctx context.Context, p config.WorkspaceProfile) error {
	client, err := f.getOrCreate(p)
	if err != nil {
		return err
	}
	_, err = client.CurrentUser.Me(ctx)
	return err
}

// EvictCache removes the cached client so the next call creates a fresh one.
func (f *WorkspaceClientFactory) EvictCache(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.clients, name)
}

func (f *WorkspaceClientFactory) getOrCreate(p config.WorkspaceProfile) (*dbsdk.WorkspaceClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if c, ok := f.clients[p.Name]; ok {
		return c, nil
	}

	cfg := &dbsdk.Config{
		Host:         p.Host,
		Token:        p.Token,
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
	}
	c, err := dbsdk.NewWorkspaceClient(cfg)
	if err != nil {
		return nil, err
	}
	f.clients[p.Name] = c
	return c, nil
}

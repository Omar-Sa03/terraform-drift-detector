package provider

import (
	"context"
	"fmt"

	"github.com/medom/terraform-drift-detector/internal/model"
)

type Stub struct {
	Provider string
	Prefix   string
}

func (s Stub) Name() string { return s.Provider }

func (s Stub) Supports(tfType string) bool {
	n := len(s.Prefix)
	return len(tfType) >= n && tfType[:n] == s.Prefix
}

func (s Stub) Fetch(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	_ = ctx
	return nil, fmt.Errorf("%s provider is not implemented yet (%d resources requested)", s.Provider, len(expected))
}

func AzureStub() Fetcher {
	return Stub{Provider: model.ProviderAzure, Prefix: "azurerm_"}
}

func GCPStub() Fetcher {
	return Stub{Provider: model.ProviderGCP, Prefix: "google_"}
}

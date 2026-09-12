package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/medom/terraform-drift-detector/internal/model"
	"github.com/medom/terraform-drift-detector/internal/provider"
)

func TestStubs(t *testing.T) {
	az := provider.AzureStub()
	if !az.Supports("azurerm_virtual_network") || az.Supports("aws_vpc") {
		t.Fatal("azure supports")
	}
	_, err := az.Fetch(context.Background(), []model.Resource{{Type: "azurerm_virtual_network"}})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("%v", err)
	}
	g := provider.GCPStub()
	if !g.Supports("google_compute_instance") {
		t.Fatal("gcp")
	}
}

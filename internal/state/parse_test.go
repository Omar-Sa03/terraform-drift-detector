package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/medom/terraform-drift-detector/internal/state"
)

func TestParseV4Example(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 6 {
		t.Fatalf("expected 6 managed resources, got %d", len(res))
	}
	byAddr := map[string]string{}
	for _, r := range res {
		byAddr[r.Address] = r.ID
		if r.Provider != "aws" {
			t.Errorf("%s provider = %s", r.Address, r.Provider)
		}
	}
	if byAddr["aws_instance.web"] != "i-0abc123" {
		t.Errorf("instance id = %s", byAddr["aws_instance.web"])
	}
	if byAddr["aws_s3_bucket.logs"] != "example-logs-bucket" {
		t.Errorf("bucket id = %s", byAddr["aws_s3_bucket.logs"])
	}
}

func TestParseV3(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "v3.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d resources", len(res))
	}
	if res[0].Address != "aws_vpc.legacy" || res[0].ID != "vpc-legacy" {
		t.Fatalf("%+v", res[0])
	}
	if res[0].Tags["Name"] != "legacy" {
		t.Fatalf("tags = %#v", res[0].Tags)
	}
}

func TestProviderFromType(t *testing.T) {
	if g := state.ProviderFromType("azurerm_virtual_machine", ""); g != "azure" {
		t.Fatal(g)
	}
	if g := state.ProviderFromType("google_compute_instance", ""); g != "gcp" {
		t.Fatal(g)
	}
}

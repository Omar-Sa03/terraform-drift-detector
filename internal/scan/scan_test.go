package scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/medom/terraform-drift-detector/internal/model"
	"github.com/medom/terraform-drift-detector/internal/provider"
	"github.com/medom/terraform-drift-detector/internal/scan"
)

type mockAWS struct {
	actual []model.Resource
}

func (m mockAWS) Name() string { return model.ProviderAWS }
func (m mockAWS) Supports(tfType string) bool {
	return len(tfType) > 4 && tfType[:4] == "aws_"
}
func (m mockAWS) Fetch(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	return m.actual, nil
}

func TestEngineRunDetectsDrift(t *testing.T) {
	statePath := filepath.Join("..", "..", "testdata", "example.tfstate")
	out := filepath.Join(t.TempDir(), "report.json")
	actual := []model.Resource{
		{Address: "aws_instance.web", Type: "aws_instance", Provider: "aws", ID: "i-0abc123",
			Attributes: map[string]any{"ami": "ami-12345678", "instance_type": "t3.small", "subnet_id": "subnet-aaa", "availability_zone": "us-east-1a", "vpc_security_group_ids": []any{"sg-web"}},
			Tags:       map[string]string{"Name": "web", "Environment": "staging"}},
		{Address: "aws_s3_bucket.logs", Type: "aws_s3_bucket", Provider: "aws", ID: "example-logs-bucket",
			Attributes: map[string]any{"bucket": "example-logs-bucket"},
			Tags:       map[string]string{"Environment": "prod"}},
		{Address: "aws_security_group.web", Type: "aws_security_group", Provider: "aws", ID: "sg-web",
			Attributes: map[string]any{"name": "web", "description": "web access", "vpc_id": "vpc-aaa"},
			Tags:       map[string]string{"Name": "web"}},
		{Address: "aws_vpc.main", Type: "aws_vpc", Provider: "aws", ID: "vpc-aaa",
			Attributes: map[string]any{"cidr_block": "10.0.0.0/16"},
			Tags:       map[string]string{"Name": "main"}},
		{Address: "aws_subnet.public", Type: "aws_subnet", Provider: "aws", ID: "subnet-aaa",
			Attributes: map[string]any{"vpc_id": "vpc-aaa", "cidr_block": "10.0.1.0/24", "availability_zone": "us-east-1a"},
			Tags:       map[string]string{"Name": "public"}},
		{Address: "aws_iam_role.app", Type: "aws_iam_role", Provider: "aws", ID: "app",
			Attributes: map[string]any{"name": "app", "assume_role_policy": `{"Version":"2012-10-17"}`},
			Tags:       map[string]string{"Team": "platform"}},
	}
	engine := scan.New(provider.NewRegistry(mockAWS{actual: actual}, provider.AzureStub(), provider.GCPStub()))
	rep, err := engine.Run(context.Background(), scan.Options{StatePath: statePath, OutPath: out})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.Modified == 0 || rep.Summary.TagChanged == 0 {
		t.Fatalf("expected instance type + tag drift, summary=%+v drifts=%+v", rep.Summary, rep.Drifts)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedTypeWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(`{
	  "version": 4,
	  "resources": [{"mode":"managed","type":"random_id","name":"x","instances":[{"attributes":{"id":"abc"}}]}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := scan.New(provider.NewRegistry(mockAWS{}, provider.AzureStub(), provider.GCPStub()))
	rep, err := engine.Run(context.Background(), scan.Options{StatePath: path, OutPath: filepath.Join(dir, "out.json")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("expected skip warning")
	}
}

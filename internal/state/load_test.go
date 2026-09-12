package state_test

import (
	"context"
	"testing"

	"github.com/medom/terraform-drift-detector/internal/state"
)

func TestLoadS3(t *testing.T) {
	data, err := testdataV4()
	if err != nil {
		t.Fatal(err)
	}
	l := &state.Loader{GetObject: func(ctx context.Context, bucket, key string) ([]byte, error) {
		if bucket != "b" || key != "env/terraform.tfstate" {
			t.Fatalf("%s %s", bucket, key)
		}
		return data, nil
	}}
	res, src, err := l.Load(context.Background(), "", "b/env/terraform.tfstate")
	if err != nil {
		t.Fatal(err)
	}
	if src != "s3://b/env/terraform.tfstate" {
		t.Fatal(src)
	}
	if len(res) == 0 {
		t.Fatal("empty")
	}
}

func testdataV4() ([]byte, error) {
	return []byte(`{"version":4,"resources":[{"mode":"managed","type":"aws_vpc","name":"x","instances":[{"attributes":{"id":"vpc-1","cidr_block":"10.0.0.0/16"}}]}]}`), nil
}

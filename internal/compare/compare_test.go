package compare_test

import (
	"testing"

	"github.com/medom/terraform-drift-detector/internal/compare"
	"github.com/medom/terraform-drift-detector/internal/model"
)

func TestDiffDeletedModifiedTags(t *testing.T) {
	expected := []model.Resource{
		{
			Address:  "aws_instance.web",
			Type:     "aws_instance",
			Provider: "aws",
			ID:       "i-1",
			Attributes: map[string]any{
				"instance_type": "t3.micro",
				"ami":           "ami-aaa",
				"timeouts":      nil,
			},
			Tags: map[string]string{"Env": "prod", "Name": "web"},
		},
		{
			Address:    "aws_s3_bucket.gone",
			Type:       "aws_s3_bucket",
			Provider:   "aws",
			ID:         "gone-bucket",
			Attributes: map[string]any{"bucket": "gone-bucket"},
		},
	}
	actual := []model.Resource{
		{
			Address:  "aws_instance.web",
			Type:     "aws_instance",
			Provider: "aws",
			ID:       "i-1",
			Attributes: map[string]any{
				"instance_type": "t3.small",
				"ami":           "ami-aaa",
			},
			Tags: map[string]string{"Env": "dev", "Name": "web"},
		},
	}

	drifts := compare.Diff(expected, actual, compare.Options{})
	kinds := map[model.DriftKind]int{}
	for _, d := range drifts {
		kinds[d.Kind]++
	}
	if kinds[model.KindDeleted] != 1 {
		t.Fatalf("deleted=%d drifts=%v", kinds[model.KindDeleted], drifts)
	}
	if kinds[model.KindModified] != 1 {
		t.Fatalf("modified=%d", kinds[model.KindModified])
	}
	if kinds[model.KindTagChanged] != 1 {
		t.Fatalf("tags=%d", kinds[model.KindTagChanged])
	}
}

func TestDiffUnmanaged(t *testing.T) {
	expected := []model.Resource{}
	actual := []model.Resource{{
		Type: "aws_vpc", Provider: "aws", ID: "vpc-x", Address: "",
		Attributes: map[string]any{"cidr_block": "10.0.0.0/16"},
	}}
	if n := len(compare.Diff(expected, actual, compare.Options{})); n != 0 {
		t.Fatalf("unmanaged off: %d", n)
	}
	got := compare.Diff(expected, actual, compare.Options{Unmanaged: true})
	if len(got) != 1 || got[0].Kind != model.KindCreated {
		t.Fatalf("%v", got)
	}
}

func TestIgnoresIDAndTimeouts(t *testing.T) {
	a := []model.Resource{{
		Address: "aws_vpc.main", Type: "aws_vpc", Provider: "aws", ID: "vpc-1",
		Attributes: map[string]any{"id": "vpc-1", "cidr_block": "10.0.0.0/16", "timeouts": "x"},
	}}
	b := []model.Resource{{
		Address: "aws_vpc.main", Type: "aws_vpc", Provider: "aws", ID: "vpc-1",
		Attributes: map[string]any{"id": "vpc-1", "cidr_block": "10.0.0.0/16"},
	}}
	if drifts := compare.Diff(a, b, compare.Options{}); len(drifts) != 0 {
		t.Fatalf("%v", drifts)
	}
}

func TestNoFalsePositiveForComputedAttrs(t *testing.T) {
	expected := []model.Resource{
		{
			Address:  "aws_s3_bucket.my_bucket",
			Type:     "aws_s3_bucket",
			Provider: "aws",
			ID:       "my-bucket",
			Attributes: map[string]any{
				"id":                                   "my-bucket",
				"bucket":                               "my-bucket",
				"bucket_domain_name":                  "my-bucket.s3.amazonaws.com",
				"hosted_zone_id":                      "Z3R1K369G5AVDG",
				"force_destroy":                        false,
				"server_side_encryption_configuration": []any{map[string]any{"rule": []any{}}},
			},
		},
	}
	actual := []model.Resource{
		{
			Address:  "aws_s3_bucket.my_bucket",
			Type:     "aws_s3_bucket",
			Provider: "aws",
			ID:       "my-bucket",
			Attributes: map[string]any{
				"id":     "my-bucket",
				"bucket": "my-bucket",
			},
		},
	}

	drifts := compare.Diff(expected, actual, compare.Options{})
	if len(drifts) != 0 {
		t.Fatalf("expected 0 drifts, got %d: %+v", len(drifts), drifts)
	}
}

package aws_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/medom/terraform-drift-detector/internal/model"
	awsp "github.com/medom/terraform-drift-detector/internal/provider/aws"
)

type mockEC2 struct {
	instances []ec2types.Instance
}

func (m *mockEC2) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: m.instances}}}, nil
}
func (m *mockEC2) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return &ec2.DescribeSecurityGroupsOutput{}, nil
}
func (m *mockEC2) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{}, nil
}
func (m *mockEC2) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{}, nil
}

type mockS3 struct {
	tags map[string]string
}

func (m mockS3) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}
func (m mockS3) GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	out := &s3.GetBucketTaggingOutput{}
	for k, v := range m.tags {
		kk, vv := k, v
		out.TagSet = append(out.TagSet, s3types.Tag{Key: &kk, Value: &vv})
	}
	return out, nil
}
func (m mockS3) GetBucketVersioning(ctx context.Context, params *s3.GetBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error) {
	return &s3.GetBucketVersioningOutput{Status: s3types.BucketVersioningStatusEnabled}, nil
}

type mockIAM struct{}

func (mockIAM) GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	name := aws.ToString(params.RoleName)
	return &iam.GetRoleOutput{Role: &iamtypes.Role{
		RoleName:                 aws.String(name),
		Arn:                      aws.String("arn:aws:iam::1:role/" + name),
		AssumeRolePolicyDocument: aws.String("%7B%22Version%22%3A%222012-10-17%22%7D"),
	}}, nil
}
func (mockIAM) ListRoleTags(ctx context.Context, params *iam.ListRoleTagsInput, optFns ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error) {
	return &iam.ListRoleTagsOutput{}, nil
}

func TestFetchInstanceBucketRole(t *testing.T) {
	f := &awsp.Fetcher{
		EC2: &mockEC2{instances: []ec2types.Instance{{
			InstanceId:   aws.String("i-0abc123"),
			ImageId:      aws.String("ami-12345678"),
			InstanceType: ec2types.InstanceTypeT3Micro,
			SubnetId:     aws.String("subnet-aaa"),
			Placement:    &ec2types.Placement{AvailabilityZone: aws.String("us-east-1a")},
			SecurityGroups: []ec2types.GroupIdentifier{{
				GroupId: aws.String("sg-web"),
			}},
			Tags: []ec2types.Tag{{
				Key: aws.String("Name"), Value: aws.String("web"),
			}},
		}}},
		S3:  mockS3{tags: map[string]string{"Environment": "prod"}},
		IAM: mockIAM{},
	}
	got, err := f.Fetch(context.Background(), []model.Resource{
		{Type: "aws_instance", ID: "i-0abc123", Address: "aws_instance.web", Provider: "aws"},
		{Type: "aws_s3_bucket", ID: "example-logs-bucket", Address: "aws_s3_bucket.logs", Provider: "aws"},
		{Type: "aws_iam_role", ID: "app", Address: "aws_iam_role.app", Provider: "aws"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.Type == "aws_instance" && r.Attributes["instance_type"] != "t3.micro" {
			t.Fatalf("%v", r.Attributes)
		}
		if r.Type == "aws_iam_role" && r.Attributes["assume_role_policy"] != `{"Version":"2012-10-17"}` {
			t.Fatalf("policy=%v", r.Attributes["assume_role_policy"])
		}
		if r.Type == "aws_s3_bucket" && r.Tags["Environment"] != "prod" {
			t.Fatalf("tags=%v", r.Tags)
		}
	}
}

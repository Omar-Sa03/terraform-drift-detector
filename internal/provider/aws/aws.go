package aws

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	awscfg "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/medom/terraform-drift-detector/internal/model"
)

var supported = map[string]bool{
	"aws_instance":       true,
	"aws_s3_bucket":      true,
	"aws_security_group": true,
	"aws_vpc":            true,
	"aws_subnet":         true,
	"aws_iam_role":       true,
}

type EC2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
}

type S3API interface {
	GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	GetBucketVersioning(ctx context.Context, params *s3.GetBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error)
}

type IAMAPI interface {
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	ListRoleTags(ctx context.Context, params *iam.ListRoleTagsInput, optFns ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error)
}

type Fetcher struct {
	EC2 EC2API
	S3  S3API
	IAM IAMAPI
}

func (f *Fetcher) Name() string { return model.ProviderAWS }

func (f *Fetcher) Supports(tfType string) bool { return supported[tfType] }

func New(ctx context.Context, region string) (*Fetcher, error) {
	opts := []func(*config.LoadOptions) error{}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return &Fetcher{
		EC2: ec2.NewFromConfig(cfg),
		S3:  s3.NewFromConfig(cfg),
		IAM: iam.NewFromConfig(cfg),
	}, nil
}

func (f *Fetcher) Fetch(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	byType := map[string][]model.Resource{}
	for _, r := range expected {
		byType[r.Type] = append(byType[r.Type], r)
	}

	var (
		mu   sync.Mutex
		out  []model.Resource
		wg   sync.WaitGroup
		errs []error
	)
	run := func(fn func() ([]model.Resource, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := fn()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			out = append(out, got...)
		}()
	}

	if rs := byType["aws_instance"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchInstances(ctx, rs) })
	}
	if rs := byType["aws_security_group"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchSecurityGroups(ctx, rs) })
	}
	if rs := byType["aws_vpc"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchVpcs(ctx, rs) })
	}
	if rs := byType["aws_subnet"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchSubnets(ctx, rs) })
	}
	if rs := byType["aws_s3_bucket"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchBuckets(ctx, rs) })
	}
	if rs := byType["aws_iam_role"]; len(rs) > 0 {
		run(func() ([]model.Resource, error) { return f.fetchRoles(ctx, rs) })
	}

	wg.Wait()
	if len(errs) > 0 {
		return out, errs[0]
	}
	return out, nil
}

func ids(rs []model.Resource) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.ID != "" {
			out = append(out, r.ID)
		}
	}
	return out
}

func addrByID(rs []model.Resource) map[string]string {
	m := map[string]string{}
	for _, r := range rs {
		m[r.ID] = r.Address
	}
	return m
}

func (f *Fetcher) fetchInstances(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	idList := ids(expected)
	if len(idList) == 0 {
		return nil, nil
	}
	out, err := f.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: idList,
	})
	if err != nil {
		if isNotFound(err) {
			return fetchEach(idList, addrByID(expected), func(id string) (*model.Resource, error) {
				one, err := f.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{id}})
				if err != nil {
					if isNotFound(err) {
						return nil, nil
					}
					return nil, err
				}
				for _, res := range one.Reservations {
					for _, inst := range res.Instances {
						r := mapInstance(inst, addrByID(expected))
						return &r, nil
					}
				}
				return nil, nil
			})
		}
		return nil, err
	}
	var got []model.Resource
	lookup := addrByID(expected)
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			got = append(got, mapInstance(inst, lookup))
		}
	}
	return got, nil
}

func mapInstance(inst ec2types.Instance, lookup map[string]string) model.Resource {
	id := awscfg.ToString(inst.InstanceId)
	sgs := make([]any, 0, len(inst.SecurityGroups))
	for _, sg := range inst.SecurityGroups {
		sgs = append(sgs, awscfg.ToString(sg.GroupId))
	}
	az := ""
	if inst.Placement != nil {
		az = awscfg.ToString(inst.Placement.AvailabilityZone)
	}
	return model.Resource{
		Address:  lookup[id],
		Type:     "aws_instance",
		Provider: model.ProviderAWS,
		ID:       id,
		Attributes: map[string]any{
			"id":                     id,
			"ami":                    awscfg.ToString(inst.ImageId),
			"instance_type":          string(inst.InstanceType),
			"subnet_id":              awscfg.ToString(inst.SubnetId),
			"availability_zone":      az,
			"vpc_security_group_ids": sgs,
		},
		Tags: tagsFromEC2(inst.Tags),
	}
}

func (f *Fetcher) fetchSecurityGroups(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	idList := ids(expected)
	if len(idList) == 0 {
		return nil, nil
	}
	out, err := f.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: idList})
	if err != nil {
		if isNotFound(err) {
			return fetchEach(idList, addrByID(expected), func(id string) (*model.Resource, error) {
				one, err := f.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{id}})
				if err != nil {
					if isNotFound(err) {
						return nil, nil
					}
					return nil, err
				}
				if len(one.SecurityGroups) == 0 {
					return nil, nil
				}
				r := mapSG(one.SecurityGroups[0], addrByID(expected))
				return &r, nil
			})
		}
		return nil, err
	}
	lookup := addrByID(expected)
	got := make([]model.Resource, 0, len(out.SecurityGroups))
	for _, sg := range out.SecurityGroups {
		got = append(got, mapSG(sg, lookup))
	}
	return got, nil
}

func mapSG(sg ec2types.SecurityGroup, lookup map[string]string) model.Resource {
	id := awscfg.ToString(sg.GroupId)
	return model.Resource{
		Address:  lookup[id],
		Type:     "aws_security_group",
		Provider: model.ProviderAWS,
		ID:       id,
		Attributes: map[string]any{
			"id":          id,
			"name":        awscfg.ToString(sg.GroupName),
			"description": awscfg.ToString(sg.Description),
			"vpc_id":      awscfg.ToString(sg.VpcId),
		},
		Tags: tagsFromEC2(sg.Tags),
	}
}

func (f *Fetcher) fetchVpcs(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	idList := ids(expected)
	if len(idList) == 0 {
		return nil, nil
	}
	out, err := f.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: idList})
	if err != nil {
		if isNotFound(err) {
			return fetchEach(idList, addrByID(expected), func(id string) (*model.Resource, error) {
				one, err := f.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{id}})
				if err != nil {
					if isNotFound(err) {
						return nil, nil
					}
					return nil, err
				}
				if len(one.Vpcs) == 0 {
					return nil, nil
				}
				r := mapVPC(one.Vpcs[0], addrByID(expected))
				return &r, nil
			})
		}
		return nil, err
	}
	lookup := addrByID(expected)
	got := make([]model.Resource, 0, len(out.Vpcs))
	for _, v := range out.Vpcs {
		got = append(got, mapVPC(v, lookup))
	}
	return got, nil
}

func mapVPC(v ec2types.Vpc, lookup map[string]string) model.Resource {
	id := awscfg.ToString(v.VpcId)
	return model.Resource{
		Address:  lookup[id],
		Type:     "aws_vpc",
		Provider: model.ProviderAWS,
		ID:       id,
		Attributes: map[string]any{
			"id":         id,
			"cidr_block": awscfg.ToString(v.CidrBlock),
		},
		Tags: tagsFromEC2(v.Tags),
	}
}

func (f *Fetcher) fetchSubnets(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	idList := ids(expected)
	if len(idList) == 0 {
		return nil, nil
	}
	out, err := f.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: idList})
	if err != nil {
		if isNotFound(err) {
			return fetchEach(idList, addrByID(expected), func(id string) (*model.Resource, error) {
				one, err := f.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: []string{id}})
				if err != nil {
					if isNotFound(err) {
						return nil, nil
					}
					return nil, err
				}
				if len(one.Subnets) == 0 {
					return nil, nil
				}
				r := mapSubnet(one.Subnets[0], addrByID(expected))
				return &r, nil
			})
		}
		return nil, err
	}
	lookup := addrByID(expected)
	got := make([]model.Resource, 0, len(out.Subnets))
	for _, s := range out.Subnets {
		got = append(got, mapSubnet(s, lookup))
	}
	return got, nil
}

func mapSubnet(s ec2types.Subnet, lookup map[string]string) model.Resource {
	id := awscfg.ToString(s.SubnetId)
	return model.Resource{
		Address:  lookup[id],
		Type:     "aws_subnet",
		Provider: model.ProviderAWS,
		ID:       id,
		Attributes: map[string]any{
			"id":                id,
			"vpc_id":            awscfg.ToString(s.VpcId),
			"cidr_block":        awscfg.ToString(s.CidrBlock),
			"availability_zone": awscfg.ToString(s.AvailabilityZone),
		},
		Tags: tagsFromEC2(s.Tags),
	}
}

func (f *Fetcher) fetchBuckets(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	lookup := addrByID(expected)
	var got []model.Resource
	for _, r := range expected {
		if r.ID == "" {
			continue
		}
		_, err := f.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: awscfg.String(r.ID)})
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return got, err
		}
		tags := map[string]string{}
		tagOut, err := f.S3.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: awscfg.String(r.ID)})
		if err == nil {
			for _, t := range tagOut.TagSet {
				tags[awscfg.ToString(t.Key)] = awscfg.ToString(t.Value)
			}
		} else if !isNotFound(err) && !strings.Contains(strings.ToLower(err.Error()), "nosuchtagset") {
			return got, err
		}
		attrs := map[string]any{
			"id":     r.ID,
			"bucket": r.ID,
		}
		verOut, err := f.S3.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: awscfg.String(r.ID)})
		if err == nil && verOut != nil {
			enabled := verOut.Status == s3types.BucketVersioningStatusEnabled
			mfaDelete := verOut.MFADelete == s3types.MFADeleteStatusEnabled
			if enabled || mfaDelete {
				attrs["versioning"] = []any{
					map[string]any{
						"enabled":    enabled,
						"mfa_delete": mfaDelete,
					},
				}
			} else if expVer, ok := r.Attributes["versioning"].([]any); ok && len(expVer) > 0 {
				attrs["versioning"] = []any{
					map[string]any{
						"enabled":    false,
						"mfa_delete": false,
					},
				}
			}
		}
		got = append(got, model.Resource{
			Address:    lookup[r.ID],
			Type:       "aws_s3_bucket",
			Provider:   model.ProviderAWS,
			ID:         r.ID,
			Attributes: attrs,
			Tags:       tags,
		})
	}
	return got, nil
}

func (f *Fetcher) fetchRoles(ctx context.Context, expected []model.Resource) ([]model.Resource, error) {
	lookup := addrByID(expected)
	var got []model.Resource
	for _, r := range expected {
		if r.ID == "" {
			continue
		}
		out, err := f.IAM.GetRole(ctx, &iam.GetRoleInput{RoleName: awscfg.String(r.ID)})
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return got, err
		}
		role := out.Role
		tags := map[string]string{}
		tagOut, err := f.IAM.ListRoleTags(ctx, &iam.ListRoleTagsInput{RoleName: role.RoleName})
		if err == nil {
			for _, t := range tagOut.Tags {
				tags[awscfg.ToString(t.Key)] = awscfg.ToString(t.Value)
			}
		}
		id := awscfg.ToString(role.RoleName)
		policy := awscfg.ToString(role.AssumeRolePolicyDocument)
		if decoded, err := url.QueryUnescape(policy); err == nil {
			policy = decoded
		}
		got = append(got, model.Resource{
			Address:  lookup[r.ID],
			Type:     "aws_iam_role",
			Provider: model.ProviderAWS,
			ID:       id,
			Attributes: map[string]any{
				"id":                 id,
				"name":               id,
				"arn":                awscfg.ToString(role.Arn),
				"assume_role_policy": policy,
			},
			Tags: tags,
		})
	}
	return got, nil
}

func tagsFromEC2(tags []ec2types.Tag) map[string]string {
	out := map[string]string{}
	for _, t := range tags {
		out[awscfg.ToString(t.Key)] = awscfg.ToString(t.Value)
	}
	return out
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") ||
		strings.Contains(msg, "invalidinstanceid.notfound") ||
		strings.Contains(msg, "invalidgroup.notfound") ||
		strings.Contains(msg, "invalidvpcid.notfound") ||
		strings.Contains(msg, "invalidsubnetid.notfound") ||
		strings.Contains(msg, "nosuchbucket") ||
		strings.Contains(msg, "nosuchentity") ||
		strings.Contains(msg, "404")
}

func fetchEach(ids []string, lookup map[string]string, fn func(id string) (*model.Resource, error)) ([]model.Resource, error) {
	_ = lookup
	var out []model.Resource
	for _, id := range ids {
		r, err := fn(id)
		if err != nil {
			return out, err
		}
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

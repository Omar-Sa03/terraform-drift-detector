package state

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/medom/terraform-drift-detector/internal/model"
)

type Loader struct {
	GetObject func(ctx context.Context, bucket, key string) ([]byte, error)
}

func NewLoader() *Loader {
	return &Loader{}
}

func (l *Loader) Load(ctx context.Context, statePath, backendS3 string) ([]model.Resource, string, error) {
	var (
		data []byte
		src  string
		err  error
	)
	switch {
	case backendS3 != "":
		data, err = l.loadS3(ctx, backendS3)
		src = "s3://" + strings.TrimPrefix(backendS3, "s3://")
	case statePath != "":
		data, err = os.ReadFile(statePath)
		src = statePath
	default:
		return nil, "", fmt.Errorf("provide --state or --backend-s3")
	}
	if err != nil {
		return nil, "", err
	}
	res, err := Parse(data)
	if err != nil {
		return nil, "", err
	}
	return res, src, nil
}

func (l *Loader) loadS3(ctx context.Context, spec string) ([]byte, error) {
	spec = strings.TrimPrefix(spec, "s3://")
	bucket, key, ok := strings.Cut(spec, "/")
	if !ok || bucket == "" || key == "" {
		return nil, fmt.Errorf("invalid --backend-s3 value %q (want bucket/key)", spec)
	}
	if l.GetObject != nil {
		return l.GetObject(ctx, bucket, key)
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	out, err := s3.NewFromConfig(cfg).GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

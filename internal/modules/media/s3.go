package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config describes an S3-compatible object store. ArvanCloud, AWS, MinIO and Wasabi all speak
// this protocol; only the endpoint and path-style flag differ.
type S3Config struct {
	Endpoint  string // e.g. https://s3.ir-thr-at1.arvanstorage.ir — empty means real AWS
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle is required by MinIO and most self-hosted stores; AWS prefers virtual-host style.
	PathStyle bool
}

// S3Storage is the production StorageAdapter. The interface already had a comment naming this
// type; it did not exist, so a deployment could only write media to a local disk.
type S3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3Storage builds the adapter. It does not touch the network: a bad endpoint or credential
// surfaces on the first Put, not at startup.
func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("media: s3 bucket is required")
	}
	if cfg.Region == "" {
		cfg.Region = "default"
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("media: s3 config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("media: s3 put %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("media: s3 get %s: %w", key, err)
	}
	return out.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("media: s3 delete %s: %w", key, err)
	}
	return nil
}

// Exists asks for the object's metadata rather than fetching it: a 10 MB asset must not be
// downloaded to answer a yes/no question.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("media: s3 head %s: %w", key, err)
}

// isNotFound recognises a missing key across the shapes the SDK and S3-compatible servers use.
// A plain error-string match is not enough: MinIO answers NoSuchKey, others answer 404.
func isNotFound(err error) bool {
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return true
		}
	}
	return strings.Contains(err.Error(), "StatusCode: 404")
}

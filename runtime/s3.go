package runtime

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/nyaruka/gocommon/storage"
)

// StorageAuth is how mailroom authenticates object storage at startup.
type StorageAuth string

const (
	StorageAuthFS     StorageAuth = "fs"
	StorageAuthIRSA   StorageAuth = "irsa"
	StorageAuthStatic StorageAuth = "static"
)

func hasIRSA() bool {
	return os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" && os.Getenv("AWS_ROLE_ARN") != ""
}

// ResolveStorageAuth chooses IRSA, static keys, or filesystem. IRSA wins even if keys are set,
// unless AWSForceStaticCredentials is true.
func ResolveStorageAuth(cfg *Config) StorageAuth {
	forceStatic := cfg != nil && cfg.AWSForceStaticCredentials
	if hasIRSA() && !forceStatic {
		return StorageAuthIRSA
	}
	if cfg != nil && cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		return StorageAuthStatic
	}
	return StorageAuthFS
}

// newAWSSession creates the session credentials are resolved from. The S3 endpoint and other S3
// specific options are deliberately left out because with IRSA this session also calls STS to
// exchange the web identity token, and STS doesn't live at the S3 endpoint.
func newAWSSession(cfg *Config, useStaticCredentials bool) (*session.Session, error) {
	awsCfg := &aws.Config{
		Region:     aws.String(cfg.S3Region),
		MaxRetries: aws.Int(3),
	}
	if useStaticCredentials {
		awsCfg.Credentials = credentials.NewStaticCredentials(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, "")
	}

	return session.NewSession(awsCfg)
}

// NewS3Client builds an S3 client. When useStaticCredentials is false, the AWS SDK default
// chain is used (IRSA, env, instance profile).
func NewS3Client(cfg *Config, useStaticCredentials bool) (storage.S3Client, error) {
	awsSession, err := newAWSSession(cfg, useStaticCredentials)
	if err != nil {
		return nil, err
	}

	return withoutACL(s3.New(awsSession, &aws.Config{
		Endpoint:         aws.String(cfg.S3Endpoint),
		DisableSSL:       aws.Bool(cfg.S3DisableSSL),
		S3ForcePathStyle: aws.Bool(cfg.S3ForcePathStyle),
	})), nil
}

// noACLS3Client strips canned ACLs from PutObject. Modern buckets with
// ObjectOwnership BucketOwnerEnforced reject ACL headers with AccessControlListNotSupported.
type noACLS3Client struct {
	inner storage.S3Client
}

func withoutACL(inner storage.S3Client) storage.S3Client {
	return &noACLS3Client{inner: inner}
}

func (c *noACLS3Client) HeadBucketWithContext(ctx context.Context, input *s3.HeadBucketInput, opts ...request.Option) (*s3.HeadBucketOutput, error) {
	return c.inner.HeadBucketWithContext(ctx, input, opts...)
}

func (c *noACLS3Client) GetObjectWithContext(ctx context.Context, input *s3.GetObjectInput, opts ...request.Option) (*s3.GetObjectOutput, error) {
	return c.inner.GetObjectWithContext(ctx, input, opts...)
}

func (c *noACLS3Client) PutObjectWithContext(ctx context.Context, input *s3.PutObjectInput, opts ...request.Option) (*s3.PutObjectOutput, error) {
	if input != nil && input.ACL != nil {
		copied := *input
		copied.ACL = nil
		input = &copied
	}
	return c.inner.PutObjectWithContext(ctx, input, opts...)
}

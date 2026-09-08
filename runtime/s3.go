package runtime

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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

// ResolveStorageAuth chooses IRSA, static keys, or filesystem. IRSA wins even if keys are set.
func ResolveStorageAuth(cfg *Config) StorageAuth {
	if hasIRSA() {
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

	return s3.New(awsSession, &aws.Config{
		Endpoint:         aws.String(cfg.S3Endpoint),
		DisableSSL:       aws.Bool(cfg.S3DisableSSL),
		S3ForcePathStyle: aws.Bool(cfg.S3ForcePathStyle),
	}), nil
}

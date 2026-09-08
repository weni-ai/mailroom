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

// NewS3Client builds an S3 client. When useStaticCredentials is false, the AWS SDK default
// chain is used (IRSA, env, instance profile).
func NewS3Client(cfg *Config, useStaticCredentials bool) (storage.S3Client, error) {
	awsCfg := &aws.Config{
		Endpoint:         aws.String(cfg.S3Endpoint),
		Region:           aws.String(cfg.S3Region),
		DisableSSL:       aws.Bool(cfg.S3DisableSSL),
		S3ForcePathStyle: aws.Bool(cfg.S3ForcePathStyle),
		MaxRetries:       aws.Int(3),
	}
	if useStaticCredentials {
		awsCfg.Credentials = credentials.NewStaticCredentials(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, "")
	}

	s3Session, err := session.NewSession(awsCfg)
	if err != nil {
		return nil, err
	}

	return s3.New(s3Session), nil
}

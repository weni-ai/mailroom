package runtime_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/nyaruka/mailroom/runtime"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveStorageAuth(t *testing.T) {
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_ROLE_ARN", "")

	cfg := runtime.NewDefaultConfig()
	cfg.AWSAccessKeyID = ""
	cfg.AWSSecretAccessKey = ""
	assert.Equal(t, runtime.StorageAuthFS, runtime.ResolveStorageAuth(cfg))

	cfg.AWSAccessKeyID = "AKIAEXAMPLE"
	cfg.AWSSecretAccessKey = "secret"
	assert.Equal(t, runtime.StorageAuthStatic, runtime.ResolveStorageAuth(cfg))

	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/var/run/secrets/eks.amazonaws.com/serviceaccount/token")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/mailroom")
	assert.Equal(t, runtime.StorageAuthIRSA, runtime.ResolveStorageAuth(cfg))

	cfg.AWSAccessKeyID = ""
	cfg.AWSSecretAccessKey = ""
	assert.Equal(t, runtime.StorageAuthIRSA, runtime.ResolveStorageAuth(cfg))
}

func TestNewS3Client(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.S3Endpoint = "https://minio.internal:9000"
	cfg.AWSAccessKeyID = "AKIAEXAMPLE"
	cfg.AWSSecretAccessKey = "secret"

	for _, useStaticCredentials := range []bool{true, false} {
		client, err := runtime.NewS3Client(cfg, useStaticCredentials)
		require.NoError(t, err)
		require.NotNil(t, client)

		s3Client, ok := client.(*s3.S3)
		require.True(t, ok)
		assert.Equal(t, "https://minio.internal:9000", s3Client.ClientInfo.Endpoint)
	}
}

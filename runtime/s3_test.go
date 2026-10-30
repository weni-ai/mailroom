package runtime_test

import (
	"context"
	"net/url"
	"testing"
	"time"

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

	cfg.AWSForceStaticCredentials = true
	assert.Equal(t, runtime.StorageAuthStatic, runtime.ResolveStorageAuth(cfg))

	cfg.AWSAccessKeyID = ""
	cfg.AWSSecretAccessKey = ""
	assert.Equal(t, runtime.StorageAuthFS, runtime.ResolveStorageAuth(cfg))

	cfg.AWSForceStaticCredentials = false
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
		assert.NotNil(t, client)
	}
}

func TestPresignGet(t *testing.T) {
	cfg := runtime.NewDefaultConfig()
	cfg.S3Region = "us-east-1"
	cfg.S3Endpoint = "https://s3.amazonaws.com"
	cfg.AWSAccessKeyID = "AKID"
	cfg.AWSSecretAccessKey = "secret"

	svc, err := runtime.NewS3Service(cfg, true)
	require.NoError(t, err)

	signer := runtime.NewMediaSigner(svc, "mailroom-media")
	signed, err := signer.PresignGet(context.Background(), "/media/1/a.jpg", 15*time.Minute)
	require.NoError(t, err)

	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Contains(t, parsed.Host, "mailroom-media")
	assert.Equal(t, "900", parsed.Query().Get("X-Amz-Expires"))
	assert.Contains(t, parsed.EscapedPath(), "media/1/a.jpg")
}

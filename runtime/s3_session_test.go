package runtime

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// the session is also used to resolve web identity credentials against STS, so it must not
// carry the S3 endpoint or S3 specific options
func TestNewAWSSessionKeepsS3OptionsOutOfSession(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.S3Endpoint = "https://minio.internal:9000"
	cfg.S3Region = "us-east-1"
	cfg.S3DisableSSL = true
	cfg.S3ForcePathStyle = true

	sess, err := newAWSSession(cfg, false)
	require.NoError(t, err)

	assert.Empty(t, aws.StringValue(sess.Config.Endpoint))
	assert.False(t, aws.BoolValue(sess.Config.DisableSSL))
	assert.False(t, aws.BoolValue(sess.Config.S3ForcePathStyle))
	assert.Equal(t, "us-east-1", aws.StringValue(sess.Config.Region))
}

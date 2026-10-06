package runtime

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingS3Client struct {
	put *s3.PutObjectInput
}

func (c *capturingS3Client) HeadBucketWithContext(ctx context.Context, input *s3.HeadBucketInput, opts ...request.Option) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, nil
}

func (c *capturingS3Client) GetObjectWithContext(ctx context.Context, input *s3.GetObjectInput, opts ...request.Option) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{}, nil
}

func (c *capturingS3Client) PutObjectWithContext(ctx context.Context, input *s3.PutObjectInput, opts ...request.Option) (*s3.PutObjectOutput, error) {
	c.put = input
	return &s3.PutObjectOutput{}, nil
}

func TestWithoutACLDropsCannedACL(t *testing.T) {
	inner := &capturingS3Client{}
	client := withoutACL(inner)

	original := &s3.PutObjectInput{
		Bucket: aws.String("media"),
		Key:    aws.String("/media/a.jpg"),
		ACL:    aws.String(s3.BucketCannedACLPublicRead),
	}

	_, err := client.PutObjectWithContext(context.Background(), original)
	require.NoError(t, err)
	require.NotNil(t, inner.put)
	assert.Nil(t, inner.put.ACL)
	assert.Equal(t, "public-read", aws.StringValue(original.ACL))
}

func TestNewS3ClientWrapsWithoutACL(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.S3Endpoint = "https://minio.internal:9000"

	client, err := NewS3Client(cfg, true)
	require.NoError(t, err)

	wrapped, ok := client.(*noACLS3Client)
	require.True(t, ok)

	s3Client, ok := wrapped.inner.(*s3.S3)
	require.True(t, ok)
	assert.Equal(t, "https://minio.internal:9000", s3Client.ClientInfo.Endpoint)
}

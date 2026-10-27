package presign_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/runtime"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mediaURL = "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"

func useSigner(t *testing.T) {
	signer, err := storage.NewS3URLSigner(&storage.S3Options{AWSAccessKeyID: "AKIDTEST", AWSSecretAccessKey: "secret", Region: "sa-east-1"}, []string{"media-bucket"})
	require.NoError(t, err)
	presign.SetSigner(signer, 2*time.Hour)
	t.Cleanup(func() { presign.SetSigner(nil, 0) })
}

func assertSigned(t *testing.T, signedURL string) {
	t.Helper()
	assert.True(t, strings.HasPrefix(signedURL, mediaURL+"?"), "expected %s to be a signed %s", signedURL, mediaURL)
	u, err := url.Parse(signedURL)
	require.NoError(t, err)
	assert.Equal(t, "7200", u.Query().Get("X-Amz-Expires"))
	assert.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
}

func TestSignWithoutSigner(t *testing.T) {
	presign.SetSigner(nil, 0)

	assert.Equal(t, mediaURL, presign.SignURL(mediaURL))
	assert.Equal(t, utils.Attachment("image/jpeg:"+mediaURL), presign.Sign(utils.Attachment("image/jpeg:"+mediaURL)))
}

func TestSignURL(t *testing.T) {
	useSigner(t)

	assertSigned(t, presign.SignURL(mediaURL))

	// other buckets and hosts are untouched
	for _, rawURL := range []string{
		"https://other-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg",
		"https://example.com/foo.jpg",
		"",
	} {
		assert.Equal(t, rawURL, presign.SignURL(rawURL))
	}
}

func TestSignURLError(t *testing.T) {
	signer, err := storage.NewS3URLSigner(&storage.S3Options{AWSAccessKeyID: "AKIDTEST", AWSSecretAccessKey: "secret", Region: "sa-east-1"}, []string{"media-bucket"})
	require.NoError(t, err)
	presign.SetSigner(signer, 0)
	t.Cleanup(func() { presign.SetSigner(nil, 0) })

	assert.Equal(t, mediaURL, presign.SignURL(mediaURL))
}

func TestSign(t *testing.T) {
	useSigner(t)

	signed := presign.Sign(utils.Attachment("image/jpeg:" + mediaURL))
	assert.Equal(t, "image/jpeg", signed.ContentType())
	assertSigned(t, signed.URL())

	for _, a := range []utils.Attachment{
		"geo:-2.90875,-79.0117686",
		"image/jpeg:https://example.com/foo.jpg",
	} {
		assert.Equal(t, a, presign.Sign(a))
	}

	original := []utils.Attachment{"image/jpeg:" + mediaURL, "geo:1,2"}
	all := presign.SignAll(original)
	require.Len(t, all, 2)
	assertSigned(t, all[0].URL())
	assert.Equal(t, utils.Attachment("geo:1,2"), all[1])
	assert.Equal(t, utils.Attachment("image/jpeg:"+mediaURL), original[0], "input slice must not be modified")

	assert.Nil(t, presign.SignAll(nil))
}

func TestInit(t *testing.T) {
	t.Cleanup(func() { presign.SetSigner(nil, 0) })

	cfg := runtime.NewDefaultConfig()
	cfg.S3Region = "sa-east-1"
	cfg.S3MediaBucket = "media-bucket"
	cfg.S3SignedBuckets = "flows-bucket, courier-bucket"
	cfg.S3PresignedURLExpiration = 3
	cfg.AWSAccessKeyID = "AKIDTEST"
	cfg.AWSSecretAccessKey = "secret"

	require.NoError(t, presign.Init(cfg, runtime.StorageAuthStatic))

	for _, bucket := range []string{"media-bucket", "flows-bucket", "courier-bucket"} {
		rawURL := "https://" + bucket + ".s3.sa-east-1.amazonaws.com/foo.jpg"
		signedURL := presign.SignURL(rawURL)
		assert.True(t, strings.HasPrefix(signedURL, rawURL+"?"), "%s should be signed", bucket)
		assert.Contains(t, signedURL, "X-Amz-Expires=10800")
		assert.Contains(t, signedURL, "X-Amz-Credential=AKIDTEST")
	}
	assert.Equal(t, "https://other.s3.sa-east-1.amazonaws.com/foo.jpg", presign.SignURL("https://other.s3.sa-east-1.amazonaws.com/foo.jpg"))

	// filesystem storage disables signing
	require.NoError(t, presign.Init(cfg, runtime.StorageAuthFS))
	assert.Equal(t, mediaURL, presign.SignURL(mediaURL))
}

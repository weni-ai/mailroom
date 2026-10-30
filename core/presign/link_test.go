package presign_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useLinks(t *testing.T, secret, previous, domain, endpoint string, buckets ...string) {
	t.Helper()
	signer, err := storage.NewS3URLSigner(&storage.S3Options{
		AWSAccessKeyID: "AKIDTEST", AWSSecretAccessKey: "secret", Region: "sa-east-1", Endpoint: endpoint,
	}, buckets)
	require.NoError(t, err)
	presign.SetSigner(signer, time.Hour)
	presign.SetLinkConfig(presign.LinkConfig{
		Domain: domain, Secret: secret, Previous: previous, Endpoint: endpoint, Region: "sa-east-1", Buckets: buckets,
	})
	t.Cleanup(func() {
		presign.SetSigner(nil, 0)
		presign.ClearLinkConfig()
	})
}

func TestDurableURLSameObjectOneToken(t *testing.T) {
	useLinks(t, "current-secret", "old-secret", "mailroom.test", "", "media-bucket")

	canonical := "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"
	expired := canonical + "?X-Amz-Signature=old&X-Amz-Expires=60"
	first := presign.DurableURL(canonical)
	second := presign.DurableURL(expired)

	assert.Equal(t, first, second)
	assert.True(t, strings.HasPrefix(first, "https://mailroom.test/mr/media/v1."))
	assert.NotContains(t, first, "X-Amz-")

	bucket, key, ok := presign.VerifyMediaToken(strings.TrimPrefix(first, "https://mailroom.test/mr/media/"))
	assert.True(t, ok)
	assert.Equal(t, "media-bucket", bucket)
	assert.Equal(t, "media/foo.jpg", key)

	other := presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/other.jpg")
	assert.NotEqual(t, first, other)
}

func TestDurableURLRejectsBadTokens(t *testing.T) {
	useLinks(t, "current-secret", "", "mailroom.test", "", "media-bucket")
	token := strings.TrimPrefix(presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"), "https://mailroom.test/mr/media/")

	flipped := token[:len(token)-1] + "A"
	if token[len(token)-1] == 'A' {
		flipped = token[:len(token)-1] + "B"
	}
	_, _, ok := presign.VerifyMediaToken(flipped)
	assert.False(t, ok)
	_, _, ok = presign.VerifyMediaToken("v1.onlytwo")
	assert.False(t, ok)

	presign.SetLinkConfig(presign.LinkConfig{Domain: "mailroom.test", Secret: "someone-else", Region: "sa-east-1", Buckets: []string{"media-bucket"}})
	_, _, ok = presign.VerifyMediaToken(token)
	assert.False(t, ok)
}

func TestDurableURLPreviousSecretVerifiesOnly(t *testing.T) {
	useLinks(t, "old-secret", "", "mailroom.test", "", "media-bucket")
	oldToken := strings.TrimPrefix(presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"), "https://mailroom.test/mr/media/")

	presign.SetLinkConfig(presign.LinkConfig{
		Domain: "mailroom.test", Secret: "current-secret", Previous: "old-secret", Region: "sa-east-1", Buckets: []string{"media-bucket"},
	})
	_, _, ok := presign.VerifyMediaToken(oldToken)
	assert.True(t, ok)

	fresh := strings.TrimPrefix(presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"), "https://mailroom.test/mr/media/")
	presign.SetLinkConfig(presign.LinkConfig{
		Domain: "mailroom.test", Secret: "old-secret", Region: "sa-east-1", Buckets: []string{"media-bucket"},
	})
	_, _, ok = presign.VerifyMediaToken(fresh)
	assert.False(t, ok, "new tokens must be signed with the current secret only")
}

func TestDurableURLPassthrough(t *testing.T) {
	external := "https://example.com/foo.jpg"
	geo := "-2.90875,-79.0117686"
	canonical := "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"

	presign.SetSigner(nil, 0)
	presign.ClearLinkConfig()
	assert.Equal(t, canonical, presign.DurableURL(canonical))

	useLinks(t, "current-secret", "", "mailroom.test", "", "media-bucket")
	assert.Equal(t, external, presign.DurableURL(external))
	assert.Equal(t, geo, presign.DurableURL(geo))
	assert.Equal(t, "https://other.s3.sa-east-1.amazonaws.com/x.jpg", presign.DurableURL("https://other.s3.sa-east-1.amazonaws.com/x.jpg"))
	already := presign.DurableURL(canonical)
	assert.Equal(t, already, presign.DurableURL(already))

	presign.SetLinkConfig(presign.LinkConfig{Domain: "", Secret: "current-secret", Region: "sa-east-1", Buckets: []string{"media-bucket"}})
	assert.Contains(t, presign.DurableURL(canonical), "X-Amz-Signature=")

	presign.SetLinkConfig(presign.LinkConfig{Domain: "mailroom.test", Secret: "", Region: "sa-east-1", Buckets: []string{"media-bucket"}})
	assert.Contains(t, presign.DurableURL(canonical), "X-Amz-Signature=")
}

func TestDurableURLCustomEndpoint(t *testing.T) {
	useLinks(t, "current-secret", "", "mailroom.test", "http://localhost:9000", "media-bucket")
	awsURL := "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"
	minioURL := "http://localhost:9000/media-bucket/media/foo.jpg?X-Amz-Signature=old"
	assert.Equal(t, presign.DurableURL(awsURL), presign.DurableURL(minioURL))
}

func TestCanonicalObjectURL(t *testing.T) {
	useLinks(t, "current-secret", "", "mailroom.test", "", "media-bucket")
	assert.Equal(t, "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg", presign.CanonicalObjectURL("media-bucket", "media/foo.jpg"))
	assert.True(t, presign.BucketAllowed("media-bucket"))
	assert.False(t, presign.BucketAllowed("other"))

	presign.SetLinkConfig(presign.LinkConfig{
		Domain: "mailroom.test", Secret: "current-secret", Endpoint: "http://localhost:9000", Region: "sa-east-1", Buckets: []string{"media-bucket"},
	})
	assert.Equal(t, "http://localhost:9000/media-bucket/media/foo.jpg", presign.CanonicalObjectURL("media-bucket", "media/foo.jpg"))
}

func TestInitWarnsWhenSecretMissing(t *testing.T) {
	hook := &countHook{}
	logrus.AddHook(hook)
	t.Cleanup(func() {
		presign.SetSigner(nil, 0)
		presign.ClearLinkConfig()
	})

	cfg := runtime.NewDefaultConfig()
	cfg.S3Region = "sa-east-1"
	cfg.S3MediaBucket = "media-bucket"
	cfg.AWSAccessKeyID = "AKIDTEST"
	cfg.AWSSecretAccessKey = "secret"
	cfg.Domain = "mailroom.test"
	cfg.MediaLinkSecret = ""

	require.NoError(t, presign.Init(cfg, runtime.StorageAuthStatic))
	assert.Equal(t, 1, hook.n)
	assert.Contains(t, presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"), "X-Amz-Signature=")

	cfg.MediaLinkSecret = "current-secret"
	require.NoError(t, presign.Init(cfg, runtime.StorageAuthStatic))
	assert.True(t, strings.HasPrefix(presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"), "https://mailroom.test/mr/media/"))

	require.NoError(t, presign.Init(cfg, runtime.StorageAuthFS))
	assert.Equal(t, "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg", presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"))
}

type countHook struct{ n int }

func (h *countHook) Levels() []logrus.Level { return []logrus.Level{logrus.WarnLevel} }
func (h *countHook) Fire(e *logrus.Entry) error {
	if strings.Contains(e.Message, "media link secret is empty") {
		h.n++
	}
	return nil
}

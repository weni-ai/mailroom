package testsuite

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/stretchr/testify/require"
)

// UseAttachmentSigner turns on attachment URL signing for the given buckets until the test ends. Fake static
// credentials are used, so signing works offline.
func UseAttachmentSigner(t testing.TB, buckets ...string) {
	signer, err := storage.NewS3URLSigner(&storage.S3Options{
		AWSAccessKeyID:     "AKIDTEST",
		AWSSecretAccessKey: "secret",
		Region:             "sa-east-1",
	}, buckets)
	require.NoError(t, err)

	presign.SetSigner(signer, time.Hour)
	t.Cleanup(func() { presign.SetSigner(nil, 0) })
}

// UseDurableLinks enables signing and durable /mr/media links for buckets until the test ends.
func UseDurableLinks(t testing.TB, domain, secret string, buckets ...string) {
	t.Helper()
	UseAttachmentSigner(t, buckets...)
	presign.SetLinkConfig(presign.LinkConfig{
		Domain: domain, Secret: secret, Region: "sa-east-1", Buckets: buckets,
	})
	t.Cleanup(presign.ClearLinkConfig)
}

// PresignedRequestor answers requests for presigned URLs with a fixed response and records their URLs. Other
// requests are passed to the wrapped requestor. Presigned URLs embed the signing time, so they can't be keyed
// in a MockRequestor.
type PresignedRequestor struct {
	inner    httpx.Requestor
	response httpx.MockResponse

	mutex sync.Mutex
	urls  []string
}

// NewPresignedRequestor creates a new requestor answering presigned URLs with the given response
func NewPresignedRequestor(inner httpx.Requestor, response httpx.MockResponse) *PresignedRequestor {
	return &PresignedRequestor{inner: inner, response: response}
}

func (r *PresignedRequestor) Do(client *http.Client, request *http.Request) (*http.Response, error) {
	if request.URL.Query().Get("X-Amz-Signature") == "" {
		return r.inner.Do(client, request)
	}

	r.mutex.Lock()
	r.urls = append(r.urls, request.URL.String())
	r.mutex.Unlock()

	return r.response.Make(request), nil
}

// URLs returns the presigned URLs requested so far
func (r *PresignedRequestor) URLs() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]string(nil), r.urls...)
}

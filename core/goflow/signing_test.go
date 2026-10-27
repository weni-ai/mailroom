package goflow

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/presign"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMediaURL = "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"

func useTestSigner(t *testing.T) {
	signer, err := storage.NewS3URLSigner(&storage.S3Options{AWSAccessKeyID: "AKIDTEST", AWSSecretAccessKey: "secret", Region: "sa-east-1"}, []string{"media-bucket"})
	require.NoError(t, err)
	presign.SetSigner(signer, time.Hour)
	t.Cleanup(func() { presign.SetSigner(nil, 0) })
}

type recordingBrainService struct {
	attachments []utils.Attachment
}

func (s *recordingBrainService) Call(session flows.Session, projectUUID uuids.UUID, text string, contact *flows.Contact, as []utils.Attachment) (*flows.BrainCall, error) {
	s.attachments = as
	return &flows.BrainCall{}, nil
}

func TestSigningBrainService(t *testing.T) {
	useTestSigner(t)

	recorder := &recordingBrainService{}
	factory := signingBrainServiceFactory(func(flows.Session) (flows.BrainService, error) { return recorder, nil })

	svc, err := factory(nil)
	require.NoError(t, err)

	original := []utils.Attachment{utils.Attachment("image/jpeg:" + testMediaURL), "image/png:https://example.com/a.png"}
	_, err = svc.Call(nil, uuids.New(), "hi", nil, original)
	require.NoError(t, err)

	require.Len(t, recorder.attachments, 2)
	assert.True(t, strings.HasPrefix(recorder.attachments[0].URL(), testMediaURL+"?X-Amz-"), recorder.attachments[0].URL())
	assert.Equal(t, utils.Attachment("image/png:https://example.com/a.png"), recorder.attachments[1])
	assert.Equal(t, utils.Attachment("image/jpeg:"+testMediaURL), original[0])

	// factory errors are passed through
	failing := signingBrainServiceFactory(func(flows.Session) (flows.BrainService, error) { return nil, errors.New("boom") })
	_, err = failing(nil)
	assert.EqualError(t, err, "boom")
}

type recordingWebhookService struct {
	body       []byte
	resentBody []byte
	length     int64
}

func (s *recordingWebhookService) Call(session flows.Session, request *http.Request) (*flows.WebhookCall, error) {
	s.body, s.resentBody, s.length = nil, nil, request.ContentLength
	if request.Body != nil {
		s.body, _ = io.ReadAll(request.Body)
	}
	if request.GetBody != nil {
		r, _ := request.GetBody()
		s.resentBody, _ = io.ReadAll(r)
	}
	return &flows.WebhookCall{}, nil
}

func callSigningWebhook(t *testing.T, method, contentType, body string) *recordingWebhookService {
	recorder := &recordingWebhookService{}
	svc, err := signingWebhookServiceFactory(func(flows.Session) (flows.WebhookService, error) { return recorder, nil })(nil)
	require.NoError(t, err)

	request, err := http.NewRequest(method, "https://webhook.example.com/", strings.NewReader(body))
	require.NoError(t, err)
	if body == "" {
		request.Body = http.NoBody
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	_, err = svc.Call(nil, request)
	require.NoError(t, err)
	return recorder
}

func TestSigningWebhookServiceJSON(t *testing.T) {
	useTestSigner(t)

	body := `{
  "attachments": ["image/jpeg:` + testMediaURL + `", "image/png:https://example.com/a.png"],
  "photo": "` + testMediaURL + `?X-Amz-Signature=old",
  "text": "[image/jpeg:` + testMediaURL + `, image/png:https://example.com/a.png]"
}`

	for _, contentType := range []string{"application/json", "application/json; charset=utf-8", ""} {
		recorder := callSigningWebhook(t, "POST", contentType, body)

		require.True(t, json.Valid(recorder.body), "signed body must still be valid JSON: %s", recorder.body)
		assert.Equal(t, recorder.body, recorder.resentBody, "GetBody must return the signed body")
		assert.Equal(t, int64(len(recorder.body)), recorder.length)

		var payload struct {
			Attachments []utils.Attachment `json:"attachments"`
			Photo       string             `json:"photo"`
			Text        string             `json:"text"`
		}
		require.NoError(t, json.Unmarshal(recorder.body, &payload))

		assert.True(t, strings.HasPrefix(payload.Attachments[0].URL(), testMediaURL+"?X-Amz-"), payload.Attachments[0].URL())
		assert.Equal(t, utils.Attachment("image/png:https://example.com/a.png"), payload.Attachments[1])
		assert.True(t, strings.HasPrefix(payload.Photo, testMediaURL+"?X-Amz-"), payload.Photo)
		assert.NotContains(t, payload.Photo, "Signature=old")
		assert.Contains(t, payload.Text, testMediaURL+"?X-Amz-")
		assert.Contains(t, payload.Text, ", image/png:https://example.com/a.png]")
	}
}

func TestSigningWebhookServiceNonJSON(t *testing.T) {
	useTestSigner(t)

	formBody := "photo=" + testMediaURL
	recorder := callSigningWebhook(t, "POST", "application/x-www-form-urlencoded", formBody)
	assert.Equal(t, formBody, string(recorder.body))

	// a JSON looking body sent as form is still left alone
	recorder = callSigningWebhook(t, "POST", "application/x-www-form-urlencoded", `{"photo":"`+testMediaURL+`"}`)
	assert.Equal(t, `{"photo":"`+testMediaURL+`"}`, string(recorder.body))

	textBody := "photo: " + testMediaURL
	recorder = callSigningWebhook(t, "POST", "", textBody)
	assert.Equal(t, textBody, string(recorder.body))

	recorder = callSigningWebhook(t, "GET", "", "")
	assert.Empty(t, recorder.body)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestSigningWebhookServiceBodyReadError(t *testing.T) {
	useTestSigner(t)

	recorder := &recordingWebhookService{}
	svc, err := signingWebhookServiceFactory(func(flows.Session) (flows.WebhookService, error) { return recorder, nil })(nil)
	require.NoError(t, err)

	request, err := http.NewRequest("POST", "https://webhook.example.com/", io.NopCloser(failingReader{}))
	require.NoError(t, err)

	call, err := svc.Call(nil, request)
	assert.EqualError(t, err, "read failed")
	assert.Nil(t, call)
	assert.Equal(t, http.NoBody, request.Body)
}

func TestSigningWebhookServiceWithoutSigner(t *testing.T) {
	presign.SetSigner(nil, 0)

	body := `{"photo":"` + testMediaURL + `"}`
	recorder := callSigningWebhook(t, "POST", "application/json", body)
	assert.Equal(t, body, string(recorder.body))
}

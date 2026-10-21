package goflow

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/presign"
)

// bodyURLRegex stops at characters that end a URL inside JSON strings and rendered attachment lists
var bodyURLRegex = regexp.MustCompile(`https?://[^\s"'<>\\,\]]+`)

type signingBrainService struct {
	flows.BrainService
}

func (s *signingBrainService) Call(session flows.Session, projectUUID uuids.UUID, text string, contact *flows.Contact, as []utils.Attachment) (*flows.BrainCall, error) {
	return s.BrainService.Call(session, projectUUID, text, contact, presign.SignAll(as))
}

// signingBrainServiceFactory presigns attachment URLs before they are sent to the brain
func signingBrainServiceFactory(factory engine.BrainServiceFactory) engine.BrainServiceFactory {
	return func(session flows.Session) (flows.BrainService, error) {
		svc, err := factory(session)
		if err != nil {
			return nil, err
		}
		return &signingBrainService{BrainService: svc}, nil
	}
}

type signingWebhookService struct {
	flows.WebhookService
}

func (s *signingWebhookService) Call(session flows.Session, request *http.Request) (*flows.WebhookCall, error) {
	if err := signJSONRequestBody(request); err != nil {
		return nil, err
	}
	return s.WebhookService.Call(session, request)
}

// signingWebhookServiceFactory presigns attachment URLs in JSON webhook bodies. Query strings and form bodies
// are left alone because the & of a signed URL would break their encoding.
func signingWebhookServiceFactory(factory engine.WebhookServiceFactory) engine.WebhookServiceFactory {
	return func(session flows.Session) (flows.WebhookService, error) {
		svc, err := factory(session)
		if err != nil {
			return nil, err
		}
		return &signingWebhookService{WebhookService: svc}, nil
	}
}

func signJSONRequestBody(request *http.Request) error {
	if !presign.Enabled() || request.Body == nil || request.Body == http.NoBody {
		return nil
	}

	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		return err
	}

	if isJSONBody(request, body) {
		body = bodyURLRegex.ReplaceAllFunc(body, func(u []byte) []byte {
			return []byte(presign.SignURL(string(u)))
		})
	}

	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	request.ContentLength = int64(len(body))
	return nil
}

func isJSONBody(request *http.Request, body []byte) bool {
	if mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type")); err == nil {
		if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
			return true
		}
		if mediaType == "application/x-www-form-urlencoded" || strings.HasPrefix(mediaType, "multipart/") {
			return false
		}
	}
	return json.Valid(body)
}

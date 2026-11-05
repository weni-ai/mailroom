package goflow

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
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
	if err := rewriteWebhookBody(request); err != nil {
		return nil, err
	}
	return s.WebhookService.Call(session, request)
}

// signingWebhookServiceFactory replaces known-location file addresses in JSON and form bodies with stable
// links. Query strings and multipart bodies are left alone. A form field keeps its original address when the
// replacement would contain &, which a short-lived signed URL does.
func signingWebhookServiceFactory(factory engine.WebhookServiceFactory) engine.WebhookServiceFactory {
	return func(session flows.Session) (flows.WebhookService, error) {
		svc, err := factory(session)
		if err != nil {
			return nil, err
		}
		return &signingWebhookService{WebhookService: svc}, nil
	}
}

func rewriteWebhookBody(request *http.Request) error {
	if !presign.Enabled() || request.Body == nil || request.Body == http.NoBody {
		return nil
	}

	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		request.Body = http.NoBody
		return err
	}

	switch {
	case isJSONBody(request, body):
		body = bodyURLRegex.ReplaceAllFunc(body, func(u []byte) []byte {
			return []byte(presign.DurableURL(string(u)))
		})
	case isFormBody(request):
		body = rewriteFormBody(body)
	}

	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	request.ContentLength = int64(len(body))
	return nil
}

func isFormBody(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/x-www-form-urlencoded"
}

func rewriteFormBody(body []byte) []byte {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return body
	}

	changed := false
	for key, vals := range values {
		for i, val := range vals {
			next := bodyURLRegex.ReplaceAllStringFunc(val, func(raw string) string {
				rewritten := presign.DurableURL(raw)
				if strings.Contains(rewritten, "&") {
					return raw
				}
				return rewritten
			})
			if next != val {
				values[key][i] = next
				changed = true
			}
		}
	}
	if !changed {
		return body
	}
	return []byte(values.Encode())
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

package generic

import (
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
)

// apiVersion is the value emitted in the X-API-Version header on all
// platform-to-ticketer requests.
const apiVersion = "1"

// Client is the HTTP client used by the generic ticketer service to call the
// partner endpoints documented in docs/generic-ticketer-service.md.
type Client struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
	apiToken    string
}

// NewClient builds a Client targeting the given partner base URL using the
// provided bearer token for authentication.
func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, apiToken string) *Client {
	return &Client{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		baseURL:     baseURL,
		apiToken:    apiToken,
	}
}

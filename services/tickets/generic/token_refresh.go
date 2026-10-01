package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

const (
	// TokenRefreshTypeCustom sends token_refresh_config.body as the token request body.
	TokenRefreshTypeCustom TokenRefreshType = "custom"
	// TokenRefreshTypeRefresh sends grant_type=refresh_token using config.refresh_token.
	TokenRefreshTypeRefresh TokenRefreshType = "refresh"

	TokenRefreshMatchAny = "any"

	configTokenRefreshEnabled = "token_refresh_enabled"
	configTokenRefreshType    = "token_refresh_type"
	configTokenRefreshConfig  = "token_refresh_config"
	configRefreshToken        = "refresh_token"
	configExpiresIn           = "expires_in"
	configClientID            = "client_id"
	configClientSecret        = "client_secret"
)

// TokenRefreshType selects how the token endpoint body is built.
type TokenRefreshType string

// TokenRefreshWhen controls when a token refresh is attempted. Conditions are
// combined with match=any (OR).
type TokenRefreshWhen struct {
	Match        string   `json:"match"`
	StatusCodes  []int    `json:"status_codes"`
	BodyContains []string `json:"body_contains"`
	Expired      bool     `json:"expired"`
}

// TokenRefreshConfig is the JSON object stored in token_refresh_config.
type TokenRefreshConfig struct {
	When              TokenRefreshWhen  `json:"when"`
	Method            string            `json:"method"`
	URL               string            `json:"url"`
	Headers           map[string]string `json:"headers"`
	Body              string            `json:"body"`
	TokenField        string            `json:"token_field"`
	RefreshTokenField string            `json:"refresh_token_field"`
	ExpiresInField    string            `json:"expires_in_field"`
	ExpiresInDefault  int64             `json:"expires_in_default"`
}

// TokenRefreshOptions is the runtime token-refresh setup passed to WithTokenRefresh.
type TokenRefreshOptions struct {
	Type         TokenRefreshType
	Config       TokenRefreshConfig
	RefreshToken string
	ClientID     string
	ClientSecret string
	ExpiresAt    int64
	OnUpdate     func(accessToken, refreshToken string, expiresAt int64)
}

// WithTokenRefresh enables configurable access-token renewal on the client.
func WithTokenRefresh(opts TokenRefreshOptions) ClientOption {
	return func(c *Client) {
		copied := opts
		if copied.Config.Headers != nil {
			headers := make(map[string]string, len(copied.Config.Headers))
			for k, v := range copied.Config.Headers {
				headers[k] = v
			}
			copied.Config.Headers = headers
		}
		c.tokenRefresh = &copied
	}
}

// ParseTokenRefreshOptions reads ticketer config. Returns (nil, nil) when refresh is disabled.
func ParseTokenRefreshOptions(config map[string]string) (*TokenRefreshOptions, error) {
	if !skipWebhookHMACValue(config[configTokenRefreshEnabled]) {
		return nil, nil
	}

	typ := TokenRefreshType(strings.TrimSpace(strings.ToLower(config[configTokenRefreshType])))
	if typ != TokenRefreshTypeCustom && typ != TokenRefreshTypeRefresh {
		return nil, errors.New("token_refresh_type must be custom or refresh")
	}

	raw := strings.TrimSpace(config[configTokenRefreshConfig])
	if raw == "" {
		return nil, errors.New("missing token_refresh_config in generic ticketer config")
	}

	cfg := TokenRefreshConfig{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, errors.Wrap(err, "invalid token_refresh_config")
	}

	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return nil, errors.New("token_refresh_config.url is required")
	}
	if strings.TrimSpace(cfg.Method) == "" {
		cfg.Method = http.MethodPost
	}
	if strings.TrimSpace(cfg.TokenField) == "" {
		cfg.TokenField = "access_token"
	}

	match := strings.TrimSpace(strings.ToLower(cfg.When.Match))
	if match == "" {
		match = TokenRefreshMatchAny
	}
	if match != TokenRefreshMatchAny {
		return nil, errors.New("token_refresh_config.when.match must be any")
	}
	cfg.When.Match = TokenRefreshMatchAny

	if typ == TokenRefreshTypeCustom && strings.TrimSpace(cfg.Body) == "" {
		return nil, errors.New("token_refresh_config.body is required when token_refresh_type is custom")
	}

	refreshToken := strings.TrimSpace(config[configRefreshToken])
	if typ == TokenRefreshTypeRefresh && refreshToken == "" {
		return nil, errors.New("refresh_token is required when token_refresh_type is refresh")
	}

	var expiresAt int64
	if v := strings.TrimSpace(config[configExpiresIn]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "invalid expires_in")
		}
		expiresAt = n
	}

	return &TokenRefreshOptions{
		Type:         typ,
		Config:       cfg,
		RefreshToken: refreshToken,
		ClientID:     strings.TrimSpace(config[configClientID]),
		ClientSecret: strings.TrimSpace(config[configClientSecret]),
		ExpiresAt:    expiresAt,
	}, nil
}

func (c *Client) refreshEnabled() bool {
	return c.tokenRefresh != nil
}

func (c *Client) tokenExpired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokenRefresh == nil || !c.tokenRefresh.Config.When.Expired {
		return false
	}
	if c.tokenRefresh.ExpiresAt <= 0 {
		return false
	}
	return dates.Now().Unix() >= c.tokenRefresh.ExpiresAt
}

func (c *Client) matchesRefreshWhen(trace *httpx.Trace) bool {
	if c.tokenRefresh == nil || trace == nil || trace.Response == nil {
		return false
	}
	when := c.tokenRefresh.Config.When
	for _, code := range when.StatusCodes {
		if trace.Response.StatusCode == code {
			return true
		}
	}
	body := string(trace.ResponseBody)
	for _, needle := range when.BodyContains {
		if needle != "" && strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

func (c *Client) refreshAccessToken() error {
	if c.tokenRefresh == nil {
		return errors.New("token refresh is not configured")
	}

	c.mu.Lock()
	opts := *c.tokenRefresh
	c.mu.Unlock()

	method := opts.Config.Method
	if method == "" {
		method = http.MethodPost
	}

	headers := map[string]string{}
	for k, v := range opts.Config.Headers {
		headers[k] = v
	}

	var body []byte
	switch opts.Type {
	case TokenRefreshTypeRefresh:
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", opts.RefreshToken)
		if opts.ClientID != "" {
			form.Set("client_id", opts.ClientID)
		}
		if opts.ClientSecret != "" {
			form.Set("client_secret", opts.ClientSecret)
		}
		body = []byte(form.Encode())
		if headers["Content-Type"] == "" {
			headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
	default:
		body = []byte(opts.Config.Body)
	}

	req, err := httpx.NewRequest(method, opts.Config.URL, bytes.NewReader(body), headers)
	if err != nil {
		return err
	}

	trace, err := httpx.DoTrace(c.httpClient, req, nil, nil, -1)
	if err != nil {
		return errors.Wrap(err, "token refresh request failed")
	}
	if trace.Response.StatusCode >= 400 {
		return fmt.Errorf("token refresh returned status %d", trace.Response.StatusCode)
	}

	tokenField := strings.TrimSpace(opts.Config.TokenField)
	if tokenField == "" {
		tokenField = "access_token"
	}

	accessToken, ok := lookupJSONPath(trace.ResponseBody, tokenField)
	if !ok {
		return fmt.Errorf("token refresh response missing %s", tokenField)
	}

	refreshToken := opts.RefreshToken
	if opts.Config.RefreshTokenField != "" {
		if next, found := lookupJSONPath(trace.ResponseBody, opts.Config.RefreshTokenField); found {
			refreshToken = next
		}
	}

	expiresAt := opts.ExpiresAt
	if seconds, ok := parseExpiresInSeconds(trace.ResponseBody, opts.Config.ExpiresInField); ok {
		expiresAt = dates.Now().Unix() + seconds
	} else if opts.Config.ExpiresInDefault > 0 {
		expiresAt = dates.Now().Unix() + opts.Config.ExpiresInDefault
	}

	c.mu.Lock()
	c.apiToken = accessToken
	var onUpdate func(string, string, int64)
	if c.tokenRefresh != nil {
		c.tokenRefresh.RefreshToken = refreshToken
		c.tokenRefresh.ExpiresAt = expiresAt
		onUpdate = c.tokenRefresh.OnUpdate
	}
	c.mu.Unlock()

	if onUpdate != nil {
		onUpdate(accessToken, refreshToken, expiresAt)
	}
	return nil
}

func parseExpiresInSeconds(raw []byte, field string) (int64, bool) {
	if strings.TrimSpace(field) == "" {
		return 0, false
	}
	v, ok := lookupJSONPath(raw, field)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(v, 64)
		if ferr != nil {
			return 0, false
		}
		n = int64(f)
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

func lookupJSONPath(raw []byte, path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || len(raw) == 0 {
		return "", false
	}
	var root interface{}
	if err := jsonx.Unmarshal(raw, &root); err != nil {
		return "", false
	}
	cur := root
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return "", false
		}
		cur, ok = m[p]
		if !ok {
			return "", false
		}
	}
	switch v := cur.(type) {
	case nil:
		return "", false
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case json.Number:
		s := v.String()
		return s, s != ""
	case bool:
		return strconv.FormatBool(v), true
	default:
		s := fmt.Sprint(v)
		if s == "" {
			return "", false
		}
		return s, true
	}
}

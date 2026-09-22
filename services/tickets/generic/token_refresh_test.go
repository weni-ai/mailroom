package generic_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/tickets/generic"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTokenRefreshOptionsDisabled(t *testing.T) {
	opts, err := generic.ParseTokenRefreshOptions(map[string]string{
		"api_token": "abc",
	})
	require.NoError(t, err)
	assert.Nil(t, opts)
}

func TestParseTokenRefreshOptionsCustom(t *testing.T) {
	cfgJSON, err := json.Marshal(map[string]interface{}{
		"when": map[string]interface{}{
			"status_codes":  []int{401, 403},
			"body_contains": []string{"INVALID_SESSION_ID"},
			"expired":       true,
		},
		"method":              "POST",
		"url":                 "https://example.com/oauth/token",
		"headers":             map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		"body":                "grant_type=password&client_id=abc",
		"token_field":         "access_token",
		"expires_in_field":    "expires_in",
		"expires_in_default":  7200,
		"refresh_token_field": "refresh_token",
	})
	require.NoError(t, err)

	opts, err := generic.ParseTokenRefreshOptions(map[string]string{
		"token_refresh_enabled": "true",
		"token_refresh_type":    "custom",
		"token_refresh_config":  string(cfgJSON),
		"api_token":             "old-token",
		"expires_in":            "1774132800",
	})
	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, generic.TokenRefreshTypeCustom, opts.Type)
	assert.Equal(t, "grant_type=password&client_id=abc", opts.Config.Body)
	assert.Equal(t, int64(1774132800), opts.ExpiresAt)
	assert.Equal(t, []int{401, 403}, opts.Config.When.StatusCodes)
	assert.Equal(t, generic.TokenRefreshMatchAny, opts.Config.When.Match)
}

func TestParseTokenRefreshOptionsRefreshRequiresToken(t *testing.T) {
	cfgJSON := `{"url":"https://example.com/oauth/token","token_field":"access_token"}`
	_, err := generic.ParseTokenRefreshOptions(map[string]string{
		"token_refresh_enabled": "true",
		"token_refresh_type":    "refresh",
		"token_refresh_config":  cfgJSON,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh_token")
}

func TestParseTokenRefreshOptionsCustomRequiresBody(t *testing.T) {
	cfgJSON := `{"url":"https://example.com/oauth/token"}`
	_, err := generic.ParseTokenRefreshOptions(map[string]string{
		"token_refresh_enabled": "true",
		"token_refresh_type":    "custom",
		"token_refresh_config":  cfgJSON,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body")
}

func TestTokenRefreshCustomRetriesAfter401(t *testing.T) {
	resetGlobals(t)
	httpx.SetRequestor(httpx.DefaultRequestor)

	var mu sync.Mutex
	var tokenCalls int
	var seenAuth []string
	var tokenBody string

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		tokenCalls++
		raw, _ := io.ReadAll(r.Body)
		tokenBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-token","expires_in":7200,"refresh_token":"r2"}`))
	})
	mux.HandleFunc("/v1/tickets", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenAuth = append(seenAuth, r.Header.Get("Authorization"))
		auth := r.Header.Get("Authorization")
		mu.Unlock()
		if auth != "Bearer new-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"INVALID_SESSION_ID"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"external_id":"EXT-1","status":"open","created_at":"2026-05-20T14:30:03Z"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var updatedAccess, updatedRefresh string
	var updatedExpiry int64
	client := generic.NewClient(http.DefaultClient, nil, srv.URL, "old-token", generic.WithTokenRefresh(generic.TokenRefreshOptions{
		Type: generic.TokenRefreshTypeCustom,
		Config: generic.TokenRefreshConfig{
			When: generic.TokenRefreshWhen{
				Match:        generic.TokenRefreshMatchAny,
				StatusCodes:  []int{401},
				BodyContains: []string{"INVALID_SESSION_ID"},
			},
			Method:            http.MethodPost,
			URL:               srv.URL + "/oauth/token",
			Headers:           map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			Body:              "grant_type=password&client_id=abc&client_secret=shh",
			TokenField:        "access_token",
			RefreshTokenField: "refresh_token",
			ExpiresInField:    "expires_in",
		},
		OnUpdate: func(accessToken, refreshToken string, expiresAt int64) {
			updatedAccess = accessToken
			updatedRefresh = refreshToken
			updatedExpiry = expiresAt
		},
	}))

	resp, trace, err := client.OpenTicket(&generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+5511999999999"},
		OpenedAt: openedAt(),
	}, "open-1")
	require.NoError(t, err)
	assert.Equal(t, "EXT-1", resp.ExternalID)
	assert.Equal(t, "Bearer new-token", trace.Request.Header.Get("Authorization"))
	assert.Equal(t, []string{"Bearer old-token", "Bearer new-token"}, seenAuth)
	assert.Equal(t, 1, tokenCalls)
	assert.Contains(t, tokenBody, "grant_type=password")
	assert.Equal(t, "new-token", updatedAccess)
	assert.Equal(t, "r2", updatedRefresh)
	assert.Greater(t, updatedExpiry, dates.Now().Unix())
}

func TestTokenRefreshTypeRefreshBuildsFormBody(t *testing.T) {
	resetGlobals(t)
	httpx.SetRequestor(httpx.DefaultRequestor)

	var tokenForm url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		tokenForm, _ = url.ParseQuery(string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"refreshed"}`))
	})
	mux.HandleFunc("/v1/tickets", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer refreshed" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"token_expired"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"external_id":"EXT-2","status":"open","created_at":"2026-05-20T14:30:03Z"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := generic.NewClient(http.DefaultClient, nil, srv.URL, "stale", generic.WithTokenRefresh(generic.TokenRefreshOptions{
		Type:         generic.TokenRefreshTypeRefresh,
		RefreshToken: "rt-1",
		ClientID:     "cid",
		ClientSecret: "csecret",
		Config: generic.TokenRefreshConfig{
			When: generic.TokenRefreshWhen{
				Match:        generic.TokenRefreshMatchAny,
				StatusCodes:  []int{403},
				BodyContains: []string{"token_expired"},
			},
			Method: http.MethodPost,
			URL:    srv.URL + "/oauth/token",
			Body:   "this-must-be-ignored",
		},
	}))

	resp, _, err := client.OpenTicket(&generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+5511999999999"},
		OpenedAt: openedAt(),
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "EXT-2", resp.ExternalID)
	assert.Equal(t, "refresh_token", tokenForm.Get("grant_type"))
	assert.Equal(t, "rt-1", tokenForm.Get("refresh_token"))
	assert.Equal(t, "cid", tokenForm.Get("client_id"))
	assert.Equal(t, "csecret", tokenForm.Get("client_secret"))
	assert.NotContains(t, tokenForm.Encode(), "this-must-be-ignored")
}

func TestTokenRefreshBeforeRequestWhenExpired(t *testing.T) {
	resetGlobals(t)
	httpx.SetRequestor(httpx.DefaultRequestor)
	defer dates.SetNowSource(dates.DefaultNowSource)
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, time.UTC)
	dates.SetNowSource(dates.NewFixedNowSource(now))

	var tokenCalls int
	var ticketCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		_, _ = w.Write([]byte(`{"access_token":"fresh"}`))
	})
	mux.HandleFunc("/v1/tickets", func(w http.ResponseWriter, r *http.Request) {
		ticketCalls++
		assert.Equal(t, "Bearer fresh", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"external_id":"EXT-3","status":"open","created_at":"2026-05-20T14:30:03Z"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := generic.NewClient(http.DefaultClient, nil, srv.URL, "stale", generic.WithTokenRefresh(generic.TokenRefreshOptions{
		Type:      generic.TokenRefreshTypeCustom,
		ExpiresAt: now.Add(-time.Minute).Unix(),
		Config: generic.TokenRefreshConfig{
			When: generic.TokenRefreshWhen{
				Match:   generic.TokenRefreshMatchAny,
				Expired: true,
			},
			Method:     http.MethodPost,
			URL:        srv.URL + "/oauth/token",
			Body:       "grant_type=password&x=1",
			TokenField: "access_token",
		},
	}))

	_, _, err := client.OpenTicket(&generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+5511999999999"},
		OpenedAt: openedAt(),
	}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, tokenCalls)
	assert.Equal(t, 1, ticketCalls)
}

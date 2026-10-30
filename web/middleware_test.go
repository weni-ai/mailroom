package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestLogPathRedactsMediaToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/mr/media/v1.secret.token.mac", nil)
	assert.Equal(t, "/mr/media/<redacted>", requestLogPath(req))

	other := httptest.NewRequest(http.MethodGet, "/mr/health?x=1", nil)
	assert.Equal(t, "/mr/health?x=1", requestLogPath(other))
}

func TestSkipPathPrefixBypassesMedia(t *testing.T) {
	var saw string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { saw = "next" })
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			saw = "wrapped"
		})
	}
	h := skipPathPrefix("/mr/media/", mw)(next)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/mr/media/abc", nil))
	assert.Equal(t, "next", saw)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/mr/health", nil))
	assert.Equal(t, "wrapped", saw)
}

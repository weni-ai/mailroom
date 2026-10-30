package media

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDeadline(t *testing.T) {
	assert.Equal(t, 5*time.Minute, writeDeadline)
}

func TestMediaHandler(t *testing.T) {
	testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
	raw := "https://media-bucket.s3.sa-east-1.amazonaws.com/media/foo.jpg"
	token := strings.TrimPrefix(presign.DurableURL(raw), "https://mailroom.test/mr/media/")

	var got *http.Request
	fetchSigned = func(req *http.Request) (*http.Response, error) {
		got = req
		body := "hello-bytes"
		status := http.StatusOK
		header := make(http.Header)
		header.Set("Content-Type", "image/jpeg")
		header.Set("Accept-Ranges", "bytes")
		if req.Header.Get("Range") == "bytes=0-0" {
			status = http.StatusPartialContent
			header.Set("Content-Range", "bytes 0-0/11")
			header.Set("Content-Length", "1")
			body = "h"
		} else if req.Header.Get("Range") == "bytes=0-4" {
			status = http.StatusPartialContent
			header.Set("Content-Range", "bytes 0-4/11")
			header.Set("Content-Length", "5")
			body = "hello"
		}
		return &http.Response{
			StatusCode: status,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
	t.Cleanup(func() {
		fetchSigned = func(req *http.Request) (*http.Response, error) { return http.DefaultClient.Do(req) }
	})

	rec := call(http.MethodGet, token, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello-bytes", rec.Body.String())
	assert.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	assert.Equal(t, "private, max-age=3600", rec.Header().Get("Cache-Control"))
	assert.NotContains(t, rec.Body.String(), "X-Amz-")
	require.NotNil(t, got)
	assert.Contains(t, got.URL.String(), "X-Amz-Signature=")
	assert.Empty(t, got.Header.Get("Range"))

	got = nil
	rec = call(http.MethodGet, token, "bytes=0-4")
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
	assert.Equal(t, "bytes 0-4/11", rec.Header().Get("Content-Range"))
	assert.Equal(t, "bytes=0-4", got.Header.Get("Range"))

	rec = call(http.MethodHead, token, "")
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "bytes 0-0/11", rec.Header().Get("Content-Range"))
	assert.Equal(t, "11", rec.Header().Get("Content-Length"))
	assert.Equal(t, "bytes=0-0", got.Header.Get("Range"))

	rec = call(http.MethodGet, token+"x", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Body.String())

	fetchSigned = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
	}
	rec = call(http.MethodGet, token, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Body.String())

	fetchSigned = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom")), Header: make(http.Header)}, nil
	}
	rec = call(http.MethodGet, token, "")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Empty(t, rec.Body.String())

	fetchSigned = func(req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Range", "bytes */11")
		return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	rec = call(http.MethodGet, token, "bytes=99-100")
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Code)
	assert.Equal(t, "bytes */11", rec.Header().Get("Content-Range"))

	fetchSigned = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("raw"))}, nil
	}
	rec = call(http.MethodGet, token, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
}

func call(method, token, rng string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/mr/media/"+token, nil)
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", token)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	_ = handle(req.Context(), nil, req, rec)
	return rec
}

package web

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaRouteIsPublicGet(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	rt.Config.S3MediaPrefix = "/media/"
	rt.MediaSigner = nil

	server := NewServer(ctx, rt, &sync.WaitGroup{})
	req := httptest.NewRequest(http.MethodGet, "/media/1/abcd/ef01/missing.txt", nil)
	rec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "attachment not found\n", rec.Body.String())

	rt.Config.S3MediaPrefix = "/attachments/"
	server = NewServer(ctx, rt, &sync.WaitGroup{})
	req = httptest.NewRequest(http.MethodGet, "/attachments/1/abcd/ef01/missing.txt", nil)
	rec = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, "attachment not found\n", rec.Body.String())

	req = httptest.NewRequest(http.MethodPost, "/attachments/1/abcd/ef01/missing.txt", nil)
	rec = httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

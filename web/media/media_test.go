package media

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSigner struct {
	calls int
	url   string
	err   error
	ttl   time.Duration
	key   string
}

func (f *fakeSigner) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	f.calls++
	f.key = key
	f.ttl = ttl
	if f.err != nil {
		return "", f.err
	}
	return f.url, nil
}

func TestRoutePattern(t *testing.T) {
	assert.Equal(t, "/media/*", RoutePattern("/media/"))
	assert.Equal(t, "/attachments/*", RoutePattern("attachments"))
}

func TestHandleRejectsPathOutsidePrefix(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	rt.Config.S3MediaPrefix = "/media/"
	signer := &fakeSigner{url: "https://signed.example/file"}
	rt.MediaSigner = signer

	for _, requestPath := range []string{"/media/../secret.jpg", "/media", "/media/"} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		rec := httptest.NewRecorder()
		err := Handle(ctx, rt, req, rec)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "invalid media path\n", rec.Body.String())
	}
	assert.Equal(t, 0, signer.calls)
}

func TestHandleRedirectsAndCachesPresign(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetRedis)

	rt.Config.S3MediaPrefix = "/media/"
	rt.Config.S3MediaBucket = "mailroom-media"
	signer := &fakeSigner{url: "https://signed.example/file"}
	rt.MediaSigner = signer
	path := "/media/1/abcd/ef01/hello.txt"

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	require.NoError(t, Handle(ctx, rt, req, rec))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://signed.example/file", rec.Header().Get("Location"))
	assert.Equal(t, 1, signer.calls)
	assert.Equal(t, path, signer.key)
	assert.Equal(t, 15*time.Minute, signer.ttl)

	rc := rt.RP.Get()
	defer rc.Close()
	cached, err := redis.String(rc.Do("GET", "media:presign:mailroom-media:"+path))
	require.NoError(t, err)
	assert.Equal(t, "https://signed.example/file", cached)
	ttl, err := redis.Int(rc.Do("TTL", "media:presign:mailroom-media:"+path))
	require.NoError(t, err)
	assert.Greater(t, ttl, 13*60)
	assert.LessOrEqual(t, ttl, 14*60)

	rec = httptest.NewRecorder()
	require.NoError(t, Handle(ctx, rt, req, rec))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://signed.example/file", rec.Header().Get("Location"))
	assert.Equal(t, 1, signer.calls)
}

func TestHandleSignsWhenRedisIsDown(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	original := rt.RP
	rt.RP = &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return nil, fmt.Errorf("redis down")
		},
	}
	defer func() { rt.RP = original }()

	rt.Config.S3MediaPrefix = "/media/"
	signer := &fakeSigner{url: "https://signed.example/fresh"}
	rt.MediaSigner = signer

	req := httptest.NewRequest(http.MethodGet, "/media/1/abcd/ef01/hello.txt", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, Handle(ctx, rt, req, rec))
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://signed.example/fresh", rec.Header().Get("Location"))
	assert.Equal(t, 1, signer.calls)
}

func TestHandleReturnsPresignError(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetRedis)

	rt.Config.S3MediaPrefix = "/media/"
	rt.MediaSigner = &fakeSigner{err: errors.New("boom")}

	req := httptest.NewRequest(http.MethodGet, "/media/1/abcd/ef01/hello.txt", nil)
	rec := httptest.NewRecorder()
	err := Handle(ctx, rt, req, rec)
	require.EqualError(t, err, "unable to presign attachment: boom")
}

func TestHandleServesFilesystem(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetStorage)

	rt.Config.S3MediaPrefix = "/media/"
	rt.MediaSigner = nil
	path := "/media/1/abcd/ef01/hello.txt"
	_, err := rt.MediaStorage.Put(ctx, path, "text/plain", []byte("hello"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	require.NoError(t, Handle(ctx, rt, req, rec))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))

	missing := httptest.NewRequest(http.MethodGet, "/media/1/abcd/ef01/missing.txt", nil)
	rec = httptest.NewRecorder()
	require.NoError(t, Handle(ctx, rt, missing, rec))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "attachment not found\n", rec.Body.String())
}

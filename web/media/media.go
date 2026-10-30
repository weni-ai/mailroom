package media

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	presignTTL         = 15 * time.Minute
	presignCacheTTL    = 14 * time.Minute
	presignCacheKey    = "media:presign:%s:%s"
	invalidMediaPath   = "invalid media path"
	attachmentNotFound = "attachment not found"
)

// RoutePattern is the chi pattern for the configured media prefix.
func RoutePattern(prefix string) string {
	return strings.TrimSuffix(normalizedPrefix(prefix), "/") + "/*"
}

func normalizedPrefix(prefix string) string {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func cleanMediaPath(prefix, requestPath string) (string, error) {
	cleaned := path.Clean(requestPath)
	normalized := normalizedPrefix(prefix)
	if cleaned == "." || cleaned == "/" || cleaned == strings.TrimSuffix(normalized, "/") || !strings.HasPrefix(cleaned, normalized) {
		return "", errors.New(invalidMediaPath)
	}
	return cleaned, nil
}

// Handle redirects to a cached presigned URL, or serves the file from filesystem storage.
func Handle(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	cleaned, err := cleanMediaPath(rt.Config.S3MediaPrefix, r.URL.Path)
	if err != nil {
		http.Error(w, invalidMediaPath, http.StatusBadRequest)
		return nil
	}

	if rt.MediaSigner != nil {
		signed, err := cachedPresign(ctx, rt, cleaned)
		if err != nil {
			return err
		}
		http.Redirect(w, r, signed, http.StatusFound)
		return nil
	}

	contentType, body, err := rt.MediaStorage.Get(ctx, cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, attachmentNotFound, http.StatusNotFound)
			return nil
		}
		return errors.Wrap(err, "unable to read attachment")
	}
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	return errors.Wrap(err, "unable to write attachment")
}

func cachedPresign(ctx context.Context, rt *runtime.Runtime, objectPath string) (string, error) {
	key := fmt.Sprintf(presignCacheKey, rt.Config.S3MediaBucket, objectPath)
	rc := rt.RP.Get()
	defer rc.Close()

	cached, err := redis.String(rc.Do("GET", key))
	if err == nil && cached != "" {
		return cached, nil
	}
	if err != nil && err != redis.ErrNil {
		log.WithError(err).WithField("key", key).Error("error reading presigned media url cache")
	}

	signed, err := rt.MediaSigner.PresignGet(ctx, objectPath, presignTTL)
	if err != nil {
		return "", errors.Wrap(err, "unable to presign attachment")
	}

	if _, setErr := rc.Do("SETEX", key, int(presignCacheTTL.Seconds()), signed); setErr != nil {
		log.WithError(setErr).WithField("key", key).Error("error writing presigned media url cache")
	}
	return signed, nil
}

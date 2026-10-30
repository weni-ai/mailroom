package media

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"
	"github.com/sirupsen/logrus"
)

const writeDeadline = 5 * time.Minute

func init() {
	web.RegisterRoute(http.MethodGet, "/mr/media/{token}", handle)
	web.RegisterRoute(http.MethodHead, "/mr/media/{token}", handle)
}

var fetchSigned = func(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func handle(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(writeDeadline))

	bucket, key, ok := presign.VerifyMediaToken(chi.URLParam(r, "token"))
	if !ok || !presign.BucketAllowed(bucket) {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}

	signed := presign.SignURL(presign.CanonicalObjectURL(bucket, key))
	if signed == "" || !strings.Contains(signed, "X-Amz-Signature=") {
		w.WriteHeader(http.StatusNotFound)
		return nil
	}

	upstream, err := http.NewRequestWithContext(ctx, http.MethodGet, signed, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return nil
	}
	rng := r.Header.Get("Range")
	if r.Method == http.MethodHead && rng == "" {
		rng = "bytes=0-0"
	}
	if rng != "" {
		upstream.Header.Set("Range", rng)
	}

	resp, err := fetchSigned(upstream)
	if err != nil {
		logErr := err
		if uerr, ok := err.(*url.Error); ok && uerr.Err != nil {
			logErr = uerr.Err
		}
		logrus.WithError(logErr).WithField("bucket", bucket).WithField("key", key).Error("error fetching attachment")
		w.WriteHeader(http.StatusBadGateway)
		return nil
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		copyMediaHeaders(w, resp)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
			w.Header().Set("Cache-Control", "private, max-age=3600")
		}
		if r.Method == http.MethodHead {
			if total, ok := contentRangeTotal(resp.Header.Get("Content-Range")); ok {
				w.Header().Set("Content-Length", strconv.FormatInt(total, 10))
			}
			w.WriteHeader(resp.StatusCode)
			return nil
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return nil
	case http.StatusNotFound:
		w.WriteHeader(http.StatusNotFound)
		return nil
	default:
		logrus.WithField("bucket", bucket).WithField("key", key).WithField("status", resp.StatusCode).Error("unexpected attachment response")
		w.WriteHeader(http.StatusBadGateway)
		return nil
	}
}

func copyMediaHeaders(w http.ResponseWriter, resp *http.Response) {
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
}

func contentRangeTotal(v string) (int64, bool) {
	_, rest, ok := strings.Cut(v, "/")
	if !ok || rest == "" || rest == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

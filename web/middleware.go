package web

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/middleware"
	log "github.com/sirupsen/logrus"
)

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)

		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}

		elapsed := time.Since(start)
		uri := fmt.Sprintf("%s://%s%s", scheme, r.Host, requestLogPath(r))
		ww.Header().Set("X-Elapsed-NS", strconv.FormatInt(int64(elapsed), 10))

		if r.RequestURI != "/" {
			log.WithFields(log.Fields{
				"method":     r.Method,
				"status":     ww.Status(),
				"elapsed":    elapsed,
				"length":     ww.BytesWritten(),
				"url":        uri,
				"user_agent": r.UserAgent(),
			}).Info("request completed")
		}
	})
}

// recovers from panics, logs them to sentry and returns an HTTP 500 response
func panicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				debug.PrintStack()
				log.WithError(errors.New(fmt.Sprint(rvr))).Error("recovered from panic in web handling")
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func skipPathPrefix(prefix string, mw func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}

func requestLogPath(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/mr/media/") {
		return "/mr/media/<redacted>"
	}
	return r.RequestURI
}

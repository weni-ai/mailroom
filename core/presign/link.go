package presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/nyaruka/gocommon/storage"
)

// LinkConfig is the non-AWS material DurableURL and the media handler share.
type LinkConfig struct {
	Domain   string
	Secret   string
	Previous string
	Endpoint string
	Region   string
	Buckets  []string
}

type linkState struct {
	domain       string
	secret       []byte
	previous     []byte
	endpoint     string
	endpointHost string
	region       string
	buckets      map[string]bool
}

var links atomic.Pointer[linkState]

// SetLinkConfig replaces the durable-link configuration.
func SetLinkConfig(cfg LinkConfig) {
	buckets := make(map[string]bool, len(cfg.Buckets))
	for _, b := range cfg.Buckets {
		if b = strings.TrimSpace(b); b != "" {
			buckets[b] = true
		}
	}
	links.Store(&linkState{
		domain:       cfg.Domain,
		secret:       []byte(cfg.Secret),
		previous:     []byte(cfg.Previous),
		endpoint:     cfg.Endpoint,
		endpointHost: endpointHost(cfg.Endpoint),
		region:       cfg.Region,
		buckets:      buckets,
	})
}

// ClearLinkConfig removes durable-link configuration.
func ClearLinkConfig() { links.Store(nil) }

// DurableURL returns a stable media URL for an allowed object, or a presigned URL, or rawURL.
// See docs/superpowers/specs/2026-10-30-durable-media-links-design.md.
func DurableURL(rawURL string) string {
	if current.Load() == nil {
		return rawURL
	}
	cfg := links.Load()
	if cfg == nil || len(cfg.secret) == 0 || cfg.domain == "" {
		return SignURL(rawURL)
	}
	bucket, key, region, ok := objectIdentity(rawURL, cfg)
	if !ok || !cfg.buckets[bucket] {
		return rawURL
	}
	return "https://" + cfg.domain + "/mr/media/" + issueToken(bucket, key, region, cfg.secret)
}

// VerifyMediaToken checks the HMAC against the current secret, then the previous one.
// v2 tokens return the region stored in the token. v1 tokens verify and return an empty region.
func VerifyMediaToken(token string) (bucket, key, region string, ok bool) {
	cfg := links.Load()
	if cfg == nil {
		return "", "", "", false
	}
	for _, secret := range [][]byte{cfg.secret, cfg.previous} {
		if len(secret) == 0 {
			continue
		}
		if bucket, key, region, ok = parseToken(token, secret); ok {
			return bucket, key, region, true
		}
	}
	return "", "", "", false
}

// BucketAllowed reports whether bucket is in the configured allow list.
func BucketAllowed(bucket string) bool {
	cfg := links.Load()
	return cfg != nil && cfg.buckets[bucket]
}

// CanonicalObjectURL is the unsigned URL SignURL can sign for this bucket, key, and region.
// A non-amazonaws endpoint (MinIO) is path-style and ignores region. AWS hosts, including
// https://s3.amazonaws.com, are virtual-hosted in the token region.
func CanonicalObjectURL(bucket, key, region string) string {
	cfg := links.Load()
	if cfg == nil {
		return ""
	}
	if cfg.endpointHost != "" && !strings.HasSuffix(cfg.endpointHost, ".amazonaws.com") {
		base := cfg.endpoint
		if !strings.Contains(base, "://") {
			base = "https://" + base
		}
		u, err := url.Parse(base)
		if err != nil {
			return ""
		}
		u.Path = "/" + bucket + "/" + key
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	if region == "" {
		region = cfg.region
	}
	if region == "" {
		region = "us-east-1"
	}
	return (&url.URL{
		Scheme: "https",
		Host:   bucket + ".s3." + region + ".amazonaws.com",
		Path:   "/" + key,
	}).String()
}

func issueToken(bucket, key, region string, secret []byte) string {
	body := "v2." + base64.RawURLEncoding.EncodeToString([]byte(bucket)) + "." + base64.RawURLEncoding.EncodeToString([]byte(key)) + "." + base64.RawURLEncoding.EncodeToString([]byte(region))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseToken(token string, secret []byte) (bucket, key, region string, ok bool) {
	parts := strings.Split(token, ".")
	var body string
	switch {
	case len(parts) == 5 && parts[0] == "v2":
		body = parts[0] + "." + parts[1] + "." + parts[2] + "." + parts[3]
	case len(parts) == 4 && parts[0] == "v1":
		body = parts[0] + "." + parts[1] + "." + parts[2]
	default:
		return "", "", "", false
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[len(parts)-1])
	if err != nil {
		return "", "", "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return "", "", "", false
	}
	rawBucket, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", false
	}
	rawKey, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", "", false
	}
	if parts[0] == "v2" {
		rawRegion, err := base64.RawURLEncoding.DecodeString(parts[3])
		if err != nil {
			return "", "", "", false
		}
		region = string(rawRegion)
	}
	return string(rawBucket), string(rawKey), region, true
}

func objectIdentity(rawURL string, cfg *linkState) (bucket, key, region string, ok bool) {
	if bucket, key, region, ok = storage.ParseS3URL(rawURL); ok {
		if cfg.endpointHost != "" && !strings.HasSuffix(cfg.endpointHost, ".amazonaws.com") {
			region = ""
		}
		return bucket, key, region, true
	}
	if cfg.endpointHost == "" {
		return "", "", "", false
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, cfg.endpointHost) {
		return "", "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], "", true
}

func endpointHost(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

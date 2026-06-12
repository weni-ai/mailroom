package generic

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// computeSig mirrors what a partner would do to produce a valid
// X-Webhook-Signature header.
func computeSig(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	const (
		secret = "shh-its-a-secret"
		body   = `{"external_id":"EXT-1","direction":"outgoing","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	)
	good := computeSig(secret, body)

	cases := []struct {
		name      string
		secret    string
		body      string
		signature string
		want      bool
	}{
		{"valid with sha256 prefix", secret, body, "sha256=" + good, true},
		{"valid without prefix", secret, body, good, true},
		{"uppercase hex still accepted", secret, body, "sha256=" + strings.ToUpper(good), true},
		{"trailing whitespace tolerated", secret, body, "sha256=" + good + " ", true},
		{"wrong secret rejected", "other", body, "sha256=" + good, false},
		{"tampered body rejected", secret, body + "!", "sha256=" + good, false},
		{"empty signature header rejected", secret, body, "", false},
		{"empty secret rejected", "", body, "sha256=" + good, false},
		{"prefix only rejected", secret, body, "sha256=", false},
		{"garbage hex rejected", secret, body, "sha256=" + strings.Repeat("z", len(good)), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, verifySignature(tc.secret, []byte(tc.body), tc.signature))
		})
	}
}

func TestVerifyTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	nowUnix := strconv.FormatInt(now.Unix(), 10)
	expiredUnix := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)

	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"now in RFC3339", now.Format(time.RFC3339), true},
		{"5 min in the past", now.Add(-5 * time.Minute).Format(time.RFC3339), true},
		{"5 min in the future", now.Add(5 * time.Minute).Format(time.RFC3339), true},
		{"6 min in the past", now.Add(-6 * time.Minute).Format(time.RFC3339), false},
		{"6 min in the future", now.Add(6 * time.Minute).Format(time.RFC3339), false},
		{"unix seconds now", nowUnix, true},
		{"unix seconds expired", expiredUnix, false},
		{"empty", "", false},
		{"not a date", "garbage", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, verifyTimestamp(tc.header, now))
		})
	}
}

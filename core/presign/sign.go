package presign

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
)

type urlSigner struct {
	signer     *storage.S3URLSigner
	expiration time.Duration
}

var current atomic.Pointer[urlSigner]

// Init configures signing from the S3 config. Credentials are resolved the same way as for media storage,
// and with filesystem storage nothing is signed.
func Init(cfg *runtime.Config, auth runtime.StorageAuth) error {
	if auth == runtime.StorageAuthFS {
		SetSigner(nil, 0)
		ClearLinkConfig()
		return nil
	}

	opts := &storage.S3Options{
		Endpoint:       cfg.S3Endpoint,
		Region:         cfg.S3Region,
		DisableSSL:     cfg.S3DisableSSL,
		ForcePathStyle: cfg.S3ForcePathStyle,
		MaxRetries:     3,
	}
	if auth == runtime.StorageAuthStatic {
		opts.AWSAccessKeyID = cfg.AWSAccessKeyID
		opts.AWSSecretAccessKey = cfg.AWSSecretAccessKey
	}

	buckets := append([]string{cfg.S3MediaBucket}, strings.Split(cfg.S3SignedBuckets, ",")...)
	signer, err := storage.NewS3URLSigner(opts, buckets)
	if err != nil {
		return err
	}

	SetSigner(signer, time.Duration(cfg.S3PresignedURLExpiration)*time.Hour)
	SetLinkConfig(LinkConfig{
		Domain:   cfg.Domain,
		Secret:   cfg.MediaLinkSecret,
		Previous: cfg.MediaLinkSecretPrevious,
		Endpoint: cfg.S3Endpoint,
		Region:   cfg.S3Region,
		Buckets:  buckets,
	})
	if cfg.MediaLinkSecret == "" {
		logrus.Warn("media link secret is empty; ticketers will send presigned attachment URLs")
	}
	return nil
}

// SetSigner replaces the signer used by SignURL, a nil signer disables signing
func SetSigner(signer *storage.S3URLSigner, expiration time.Duration) {
	if signer == nil {
		current.Store(nil)
		return
	}
	current.Store(&urlSigner{signer: signer, expiration: expiration})
}

// Enabled returns whether a signer is configured
func Enabled() bool {
	return current.Load() != nil
}

// SignURL returns a presigned URL for objects in our signed buckets, any other URL is returned unchanged
func SignURL(rawURL string) string {
	s := current.Load()
	if s == nil {
		return rawURL
	}

	signedURL, _, err := s.signer.Sign(rawURL, s.expiration)
	if err != nil {
		logrus.WithError(err).WithField("url", rawURL).Error("error presigning attachment URL")
		return rawURL
	}
	return signedURL
}

// Sign returns the attachment with its URL presigned, geo attachments are returned unchanged
func Sign(a utils.Attachment) utils.Attachment {
	contentType, rawURL := a.ToParts()
	if contentType == "geo" {
		return a
	}

	signedURL := SignURL(rawURL)
	if signedURL == rawURL {
		return a
	}
	if contentType == "" {
		return utils.Attachment(signedURL)
	}
	return utils.Attachment(contentType + ":" + signedURL)
}

// SignAll returns a new slice with every attachment signed
func SignAll(as []utils.Attachment) []utils.Attachment {
	if as == nil {
		return nil
	}

	signed := make([]utils.Attachment, len(as))
	for i, a := range as {
		signed[i] = Sign(a)
	}
	return signed
}

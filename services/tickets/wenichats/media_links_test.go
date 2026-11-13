package wenichats

import (
	"testing"

	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
)

const (
	mediaPhoto = "https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg"
	mediaClip  = "https://media-bucket.s3.sa-east-1.amazonaws.com/media/clip.mp4"
)

func TestRewriteMediaLinks(t *testing.T) {
	t.Run("known buckets become durable", func(t *testing.T) {
		testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
		photo := presign.DurableURL(mediaPhoto)
		clip := presign.DurableURL(mediaClip)

		assert.Equal(t, "file "+photo, rewriteMediaLinks("file "+mediaPhoto))
		assert.Equal(t, photo+" "+clip, rewriteMediaLinks(mediaPhoto+" "+mediaClip))
		assert.Equal(t, photo+" and "+photo, rewriteMediaLinks(mediaPhoto+" and "+mediaPhoto))

		signed := mediaPhoto + "?X-Amz-Signature=secret&X-Amz-Security-Token=session"
		assert.Equal(t, photo, rewriteMediaLinks(signed))
		assert.NotContains(t, rewriteMediaLinks(signed), "X-Amz-Signature")
	})

	t.Run("other addresses and wording stay", func(t *testing.T) {
		testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
		photo := presign.DurableURL(mediaPhoto)
		public := "https://example.com/a.jpg"
		unknown := "https://other-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg"

		assert.Equal(t, "see "+public, rewriteMediaLinks("see "+public))
		assert.Equal(t, "keep "+photo, rewriteMediaLinks("keep "+photo))
		assert.Equal(t, unknown, rewriteMediaLinks(unknown))

		for _, punct := range []string{".", ",", ";", ":", "!", "?", ")", "]", "}"} {
			assert.Equal(t, "see "+photo+punct, rewriteMediaLinks("see "+mediaPhoto+punct), punct)
		}
	})

	t.Run("fallback and empty", func(t *testing.T) {
		assert.Equal(t, "", rewriteMediaLinks(""))

		t.Run("signed when durable links are unavailable", func(t *testing.T) {
			testsuite.UseAttachmentSigner(t, "media-bucket")
			presign.ClearLinkConfig()
			got := rewriteMediaLinks(mediaPhoto)
			assert.NotEqual(t, mediaPhoto, got)
			assert.Contains(t, got, "X-Amz-Algorithm")
		})

		t.Run("original when nothing can sign", func(t *testing.T) {
			presign.SetSigner(nil, 0)
			presign.ClearLinkConfig()
			assert.Equal(t, mediaPhoto, rewriteMediaLinks(mediaPhoto))
		})
	})
}

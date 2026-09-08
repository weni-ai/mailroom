package urlx_test

import (
	"testing"

	"github.com/nyaruka/mailroom/utils/urlx"

	"github.com/stretchr/testify/assert"
)

func TestExt(t *testing.T) {
	tcs := []struct {
		url string
		ext string
	}{
		{"http://coolfiles.com/a.jpg", ".jpg"},
		{"https://coolfiles.com/a.tar.gz", ".gz"},
		{"https://coolfiles.com/noext", ""},
		{"http://coolfiles.com/a.jpg#frag", ".jpg"},
		{"", ""},
		// presigned URLs carry the credentials in the query string, which is not part of the extension
		{"https://weni-staging-chats.s3.amazonaws.com/messagemedia/128eb10b.jpeg?AWSAccessKeyId=ASIAQCLGXYHI2RCN6IHC&Signature=IVCrvyWNjNdHMtfS2HqBLDMQ%2FaY%3D&Expires=1788909444", ".jpeg"},
		// not parseable as a URL, so the query is stripped by hand
		{"http://coolfiles.com/a b.jpg?x=1", ".jpg"},
	}

	for _, tc := range tcs {
		assert.Equal(t, tc.ext, urlx.Ext(tc.url), "unexpected extension for %s", tc.url)
	}
}

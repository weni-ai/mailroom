package zendesk

import (
	"testing"

	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertAttachmentsRelativeURL(t *testing.T) {
	svc := &service{rtConfig: &runtime.Config{
		S3MediaPrefix:        "/media/",
		S3MediaPrefixZendesk: "textit.com",
	}}

	fileURLs, err := svc.convertAttachments([]utils.Attachment{
		"image/jpeg:/media/1/6683/83ba/668383ba-387c-49bc-b164-1213ac0ea7aa.jpg",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://textit.com/api/v2/file/1/6683/83ba/668383ba-387c-49bc-b164-1213ac0ea7aa.jpg",
	}, fileURLs)
}

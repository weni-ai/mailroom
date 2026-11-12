package zendesk

import (
	"regexp"
	"strings"

	"github.com/nyaruka/mailroom/core/presign"
)

// mediaURLPattern finds literal http(s) URLs. The match stops before whitespace, quotes, and
// angle brackets so a URL inside a JSON string does not swallow the closing quote.
var mediaURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// trailingURLPunctuation is sentence and JSON punctuation that can sit against a URL without belonging to the object.
const trailingURLPunctuation = ".,;:!?)]}"

// rewriteMediaLinks replaces known-bucket media addresses in body with the link presign.DurableURL
// would send for an attachment. The result is not an error: an address that cannot be made durable
// stays on the same fallback DurableURL already uses, and ticket opening continues.
func rewriteMediaLinks(body string) string {
	if body == "" {
		return body
	}
	return mediaURLPattern.ReplaceAllStringFunc(body, func(match string) string {
		url := strings.TrimRight(match, trailingURLPunctuation)
		if url == "" {
			return match
		}
		rewritten := presign.DurableURL(url)
		if rewritten == url {
			return match
		}
		return rewritten + match[len(url):]
	})
}

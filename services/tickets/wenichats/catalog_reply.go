package wenichats

import (
	"strings"

	"github.com/pkg/errors"
)

// catalogBodyMaxRunes is the channel limit for an interactive catalog body.
// A longer body is rejected, so the stored body is cut to this many characters.
const catalogBodyMaxRunes = 1024

// catalogReplyBody builds the catalog wording delivery reads from metadata.
// Plain text on the outgoing message stays empty so this wording is not sent a second time.
// The body is the trimmed agent text, or the trimmed title when that text is blank.
func catalogReplyBody(agentText, title string) (body string, header string, err error) {
	body = strings.TrimSpace(agentText)
	header = strings.TrimSpace(title)
	if body == "" {
		body = header
	}
	if body == "" {
		return "", "", errors.New("catalog reply requires body text")
	}
	runes := []rune(body)
	if len(runes) > catalogBodyMaxRunes {
		body = string(runes[:catalogBodyMaxRunes])
	}
	return body, header, nil
}

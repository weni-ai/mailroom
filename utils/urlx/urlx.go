package urlx

import (
	"net/url"
	"path"
	"strings"
)

// Ext returns the file extension of the path of the given URL. Unlike filepath.Ext it ignores
// query strings, which for presigned URLs hold credentials and can be several KB long.
func Ext(rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil {
		return path.Ext(parsed.Path)
	}

	trimmed := rawURL
	if i := strings.IndexAny(trimmed, "?#"); i >= 0 {
		trimmed = trimmed[:i]
	}

	return path.Ext(trimmed)
}

package urlx

import (
	"net/url"
	"path"
	"strings"
)

// query params that services use to carry the original filename, e.g. Zendesk attachments
var filenameParams = []string{"name", "filename", "file"}

// Ext returns the file extension of the given URL. Unlike filepath.Ext it never returns part of a
// query string, which for presigned URLs holds credentials and can be several KB long. When the
// path has no extension, the params which carry a filename are checked.
func Ext(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		trimmed := rawURL
		if i := strings.IndexAny(trimmed, "?#"); i >= 0 {
			trimmed = trimmed[:i]
		}
		return path.Ext(trimmed)
	}

	if ext := path.Ext(parsed.Path); ext != "" {
		return ext
	}

	query := parsed.Query()
	for _, param := range filenameParams {
		if ext := path.Ext(query.Get(param)); ext != "" {
			return ext
		}
	}

	return ""
}

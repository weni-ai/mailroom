package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

// openTemplateFuncs are helpers available inside open_template.
var openTemplateFuncs = template.FuncMap{
	"json": func(v interface{}) (string, error) {
		data, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(data), nil
	},
	"toString": func(v interface{}) string {
		return fmt.Sprintf("%v", v)
	},
}

// parseOpenTemplate parses a Go text/template used to render the Open request body.
func parseOpenTemplate(src string) (*template.Template, error) {
	tmpl, err := template.New("open").Funcs(openTemplateFuncs).Parse(src)
	if err != nil {
		return nil, errors.Wrap(err, "invalid open_template")
	}
	return tmpl, nil
}

// renderOpenTemplate executes tmpl against the OpenRequest JSON shape and
// returns the rendered body. The output must be valid JSON.
func renderOpenTemplate(tmpl *template.Template, req *OpenRequest) ([]byte, error) {
	raw, err := jsonx.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling open request for template context")
	}

	var ctx map[string]interface{}
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return nil, errors.Wrap(err, "error building open template context")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, errors.Wrap(err, "error executing open_template")
	}

	out := bytes.TrimSpace(buf.Bytes())
	if len(out) == 0 {
		return nil, errors.New("open_template rendered empty body")
	}
	if !json.Valid(out) {
		return nil, errors.New("open_template rendered invalid JSON")
	}
	return out, nil
}

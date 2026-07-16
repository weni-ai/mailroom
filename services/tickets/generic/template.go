package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

// templateFuncs are helpers available inside open_template and open_response_template.
var templateFuncs = template.FuncMap{
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
	tmpl, err := template.New("open").Funcs(templateFuncs).Parse(src)
	if err != nil {
		return nil, errors.Wrap(err, "invalid open_template")
	}
	return tmpl, nil
}

// parseOpenResponseTemplate parses a Go text/template that maps a partner Open
// response into the platform OpenResponse JSON shape.
func parseOpenResponseTemplate(src string) (*template.Template, error) {
	tmpl, err := template.New("open_response").Funcs(templateFuncs).Parse(src)
	if err != nil {
		return nil, errors.Wrap(err, "invalid open_response_template")
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

	return executeTemplate(tmpl, ctx, "open_template")
}

// mapOpenResponse executes tmpl against the partner response JSON and
// unmarshals the result into OpenResponse. The template must render JSON with
// at least external_id (status and created_at are optional).
func mapOpenResponse(tmpl *template.Template, raw []byte) (*OpenResponse, error) {
	ctx := make(map[string]interface{})
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&ctx); err != nil {
			return nil, errors.Wrap(err, "unable to decode open response for template")
		}
	}

	out, err := executeTemplate(tmpl, ctx, "open_response_template")
	if err != nil {
		return nil, err
	}

	resp := &OpenResponse{}
	if err := json.Unmarshal(out, resp); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped open response")
	}
	return resp, nil
}

// decodeOpenResponse unmarshals a standard OpenResponse envelope.
func decodeOpenResponse(raw []byte) (*OpenResponse, error) {
	resp := &OpenResponse{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp, nil
	}
	if err := jsonx.Unmarshal(raw, resp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling open response")
	}
	return resp, nil
}

func executeTemplate(tmpl *template.Template, ctx interface{}, name string) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, errors.Wrapf(err, "error executing %s", name)
	}

	out := bytes.TrimSpace(buf.Bytes())
	if len(out) == 0 {
		return nil, errors.Errorf("%s rendered empty body", name)
	}
	if !json.Valid(out) {
		return nil, errors.Errorf("%s rendered invalid JSON", name)
	}
	return out, nil
}

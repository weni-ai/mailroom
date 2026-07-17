package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

// templateFuncs are helpers available inside request/response templates.
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

func parseNamedTemplate(name, src string) (*template.Template, error) {
	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(src)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid %s", name)
	}
	return tmpl, nil
}

// parseOpenTemplate parses a Go text/template used to render the Open request body.
func parseOpenTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("open_template", src)
}

// parseOpenResponseTemplate parses a Go text/template that maps a partner Open
// response into the platform OpenResponse JSON shape.
func parseOpenResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("open_response_template", src)
}

// parseForwardTemplate parses a Go text/template used to render the Forward request body.
func parseForwardTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("forward_template", src)
}

// parseForwardResponseTemplate parses a Go text/template that maps a partner
// Forward response into the platform MessageResponse JSON shape.
func parseForwardResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("forward_response_template", src)
}

// parseCloseTemplate parses a Go text/template used to render the Close request body.
func parseCloseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("close_template", src)
}

// renderRequestTemplate marshals req to JSON map context and executes tmpl.
func renderRequestTemplate(tmpl *template.Template, req interface{}, name string) ([]byte, error) {
	raw, err := jsonx.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "error marshalling %s request for template context", name)
	}

	var ctx map[string]interface{}
	if err := json.Unmarshal(raw, &ctx); err != nil {
		return nil, errors.Wrapf(err, "error building %s template context", name)
	}

	return executeTemplate(tmpl, ctx, name)
}

// renderOpenTemplate executes tmpl against the OpenRequest JSON shape and
// returns the rendered body. The output must be valid JSON.
func renderOpenTemplate(tmpl *template.Template, req *OpenRequest) ([]byte, error) {
	return renderRequestTemplate(tmpl, req, "open_template")
}

// renderForwardTemplate executes tmpl against the MessageRequest JSON shape and
// returns the rendered body. The output must be valid JSON.
func renderForwardTemplate(tmpl *template.Template, req *MessageRequest) ([]byte, error) {
	return renderRequestTemplate(tmpl, req, "forward_template")
}

// renderCloseTemplate executes tmpl against the CloseRequest JSON shape and
// returns the rendered body. The output must be valid JSON.
func renderCloseTemplate(tmpl *template.Template, req *CloseRequest) ([]byte, error) {
	return renderRequestTemplate(tmpl, req, "close_template")
}

// mapOpenResponse executes tmpl against the partner response JSON and
// unmarshals the result into OpenResponse. The template must render JSON with
// at least external_id (status and created_at are optional).
func mapOpenResponse(tmpl *template.Template, raw []byte) (*OpenResponse, error) {
	out, err := mapResponseBody(tmpl, raw, "open_response_template")
	if err != nil {
		return nil, err
	}
	resp := &OpenResponse{}
	if err := json.Unmarshal(out, resp); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped open response")
	}
	return resp, nil
}

// mapForwardResponse executes tmpl against the partner response JSON and
// unmarshals the result into MessageResponse.
func mapForwardResponse(tmpl *template.Template, raw []byte) (*MessageResponse, error) {
	out, err := mapResponseBody(tmpl, raw, "forward_response_template")
	if err != nil {
		return nil, err
	}
	resp := &MessageResponse{}
	if err := json.Unmarshal(out, resp); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped forward response")
	}
	return resp, nil
}

func mapResponseBody(tmpl *template.Template, raw []byte, name string) ([]byte, error) {
	ctx := make(map[string]interface{})
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&ctx); err != nil {
			return nil, errors.Wrapf(err, "unable to decode %s response for template", name)
		}
	}

	return executeTemplate(tmpl, ctx, name)
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

// decodeForwardResponse unmarshals a standard MessageResponse envelope.
func decodeForwardResponse(raw []byte) (*MessageResponse, error) {
	resp := &MessageResponse{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp, nil
	}
	if err := jsonx.Unmarshal(raw, resp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling forward response")
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

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

// parseCloseResponseTemplate parses a Go text/template that maps a partner
// Close response into the platform CloseResponse JSON shape.
func parseCloseResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("close_response_template", src)
}

// parseHistoryTemplate parses a Go text/template used to render the History
// request body in batch or one_by_one mode.
func parseHistoryTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("history_template", src)
}

// parseHistoryResponseTemplate parses a Go text/template that maps a partner
// History response into the platform HistoryResponse JSON shape.
func parseHistoryResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("history_response_template", src)
}

// parseMessagesTemplate parses a Go text/template that maps a partner inbound
// /messages webhook body into the platform agent-message payload shape.
func parseMessagesTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("messages_template", src)
}

// parseMessagesResponseTemplate parses a Go text/template that maps the
// platform /messages success response into the partner's expected shape.
func parseMessagesResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("messages_response_template", src)
}

// parseTicketsCloseTemplate parses a Go text/template that maps a partner
// inbound /tickets/close webhook body into the platform close-ticket payload.
func parseTicketsCloseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("tickets_close_template", src)
}

// parseTicketsCloseResponseTemplate parses a Go text/template that maps the
// platform /tickets/close success response into the partner's expected shape.
func parseTicketsCloseResponseTemplate(src string) (*template.Template, error) {
	return parseNamedTemplate("tickets_close_response_template", src)
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

// renderHistoryTemplate executes tmpl against a History request context and
// returns the rendered body. The output must be valid JSON.
func renderHistoryTemplate(tmpl *template.Template, ctx interface{}) ([]byte, error) {
	return renderRequestTemplate(tmpl, ctx, "history_template")
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

// mapCloseResponse executes tmpl against the partner response JSON and
// unmarshals the result into CloseResponse.
func mapCloseResponse(tmpl *template.Template, raw []byte) (*CloseResponse, error) {
	out, err := mapResponseBody(tmpl, raw, "close_response_template")
	if err != nil {
		return nil, err
	}
	resp := &CloseResponse{}
	if err := json.Unmarshal(out, resp); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped close response")
	}
	return resp, nil
}

// mapHistoryResponse executes tmpl against the partner response JSON and
// unmarshals the result into HistoryResponse.
func mapHistoryResponse(tmpl *template.Template, raw []byte) (*HistoryResponse, error) {
	out, err := mapResponseBody(tmpl, raw, "history_response_template")
	if err != nil {
		return nil, err
	}
	resp := &HistoryResponse{}
	if err := json.Unmarshal(out, resp); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped history response")
	}
	return resp, nil
}

// mapAgentMessagePayload executes tmpl against a partner inbound /messages
// body and unmarshals the result into agentMessagePayload.
func mapAgentMessagePayload(tmpl *template.Template, raw []byte) (*agentMessagePayload, error) {
	out, err := mapResponseBody(tmpl, raw, "messages_template")
	if err != nil {
		return nil, err
	}
	payload := &agentMessagePayload{}
	if err := json.Unmarshal(out, payload); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped agent message payload")
	}
	return payload, nil
}

// renderMessagesResponseTemplate executes tmpl against the platform success
// response for /messages and returns the rendered JSON body.
func renderMessagesResponseTemplate(tmpl *template.Template, resp map[string]interface{}) ([]byte, error) {
	return renderRequestTemplate(tmpl, resp, "messages_response_template")
}

// mapCloseTicketPayload executes tmpl against a partner inbound /tickets/close
// body and unmarshals the result into closeTicketPayload.
func mapCloseTicketPayload(tmpl *template.Template, raw []byte) (*closeTicketPayload, error) {
	out, err := mapResponseBody(tmpl, raw, "tickets_close_template")
	if err != nil {
		return nil, err
	}
	payload := &closeTicketPayload{}
	if err := json.Unmarshal(out, payload); err != nil {
		return nil, errors.Wrap(err, "error decoding mapped close ticket payload")
	}
	return payload, nil
}

// renderTicketsCloseResponseTemplate executes tmpl against the platform
// success response for /tickets/close and returns the rendered JSON body.
func renderTicketsCloseResponseTemplate(tmpl *template.Template, resp map[string]interface{}) ([]byte, error) {
	return renderRequestTemplate(tmpl, resp, "tickets_close_response_template")
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

// decodeCloseResponse unmarshals a standard CloseResponse envelope.
func decodeCloseResponse(raw []byte) (*CloseResponse, error) {
	resp := &CloseResponse{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp, nil
	}
	if err := jsonx.Unmarshal(raw, resp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling close response")
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

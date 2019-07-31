package imi

import (
	"fmt"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"

	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/flows/routers/waits"
	"github.com/nyaruka/goflow/flows/routers/waits/hints"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/ivr"
	"github.com/nyaruka/mailroom/models"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	imiChannelType = models.ChannelType("IMI")

	statusFailed = "1"

	sendURLConfig     = "send_url"
	phoneNumberConfig = "phone_number"
	usernameConfig    = "username"
	passwordConfig    = "password"
)

var indentMarshal = true

type client struct {
	channel         *models.Channel
	sendURL         string
	phoneNumber     string
	accountUserName string
	accountPassword string
}

type CallRequest struct {
	TransID  string `json:"TransID"`
	To       string `json:"To"`
	From     string `json:"From"`
	VxmlURL  string `json:"VXMLUrl"`
	EventURL string `json:"EventUrl"`
}

type CallResponse struct {
	TransID     string `json:"obdtransid"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type VXMLProperty struct {
	Text  string `xml:",chardata"`
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type VXMLVar struct {
	Text string `xml:",chardata"`
	Name string `xml:"name,attr"`
	Expr string `xml:"expr,attr,omitempty"`
}

type VXMLBase struct {
	XMLName  xml.Name       `xml:"vxml"`
	Text     string         `xml:",chardata"`
	Version  string         `xml:"version,attr"`
	Property []VXMLProperty `xml:"property"`
	Var      []VXMLVar      `xml:"var"`
	Form     struct {
		Text string      `xml:",chardata"`
		ID   string      `xml:"id,attr"`
		Var  []VXMLVar   `xml:"var"`
		Body interface{} `xml:",innerxml"`
	} `xml:"form"`
}

type Block struct {
	XMLName string `xml:"block"`
	Text   string `xml:",chardata"`
	Prompt struct {
		XMLName string `xml:"prompt"`
		Text    string `xml:",chardata"`
		Bargein bool `xml:"bargein,attr"`
		Body interface{} `xml:",innerxml"`
	}
}

type Audio struct {
	XMLName string `xml:"audio"`
	Text string `xml:",chardata"`
	Src  string `xml:"src,attr"`
}

type Hangup struct {
	XMLName string `xml:"exit"`
}

type Assign struct {
	Text string `xml:",chardata"`
	Name string `xml:"name,attr"`
	Expr string `xml:"expr,attr"`
}

type Value struct {
	Text string `xml:",chardata"`
	Expr string `xml:"expr,attr"`
}

type Log struct {
	Text string `xml:",chardata"`
	Value []Value `xml:"value"`
}

type Field struct {
	XMLName string `xml:"field"`
	Text    string `xml:",chardata"`
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Filled  struct {
		Text string `xml:",chardata"`
		Log []Log `xml:"log"`
		Assign []Assign `xml:"assign"`
		Data struct {
			Text     string `xml:",chardata"`
			Name     string `xml:"name,attr"`
			Src      string `xml:"src,attr"`
			Namelist string `xml:"namelist,attr"`
			Method   string `xml:"method,attr"`
			Enctype  string `xml:"enctype,attr"`
		} `xml:"data"`
		If struct {
			Text string   `xml:",chardata"`
			Cond string   `xml:"cond,attr"`
			Log  []string `xml:"log"`
			Goto struct {
				Text string `xml:",chardata"`
				Next string `xml:"next,attr"`
			} `xml:"goto"`
			Else struct {
				Text string `xml:",chardata"`
				Log  struct {
					Text  string `xml:",chardata"`
					Value struct {
						Text string `xml:",chardata"`
						Expr string `xml:"expr,attr"`
					} `xml:"value"`
				} `xml:"log"`
			} `xml:"else"`
		} `xml:"if"`
	} `xml:"filled"`
}

func VXMLDocument(body interface{}) VXMLBase {
	v := VXMLBase{}
	v.Version = "2.1"
	v.Form.ID = "AcceptDigits"

	p := make([]VXMLProperty, 0)
	p = append(p, VXMLProperty{Name: "confidencelevel", Value: "0.5"})
	p = append(p, VXMLProperty{Name: "maxage", Value: "30"})
	p = append(p, VXMLProperty{Name: "inputmodes", Value: "dtmf"})
	p = append(p, VXMLProperty{Name: "interdigittimeout", Value: "12s"})
	p = append(p, VXMLProperty{Name: "timeout", Value: "12s"})
	p = append(p, VXMLProperty{Name: "termchar", Value: "#"})
	v.Property = p

	va := make([]VXMLVar, 0)
	va = append(va, VXMLVar{Name: "recieveddtmf"})
	va = append(va, VXMLVar{Name: "ExecuteVXML"})
	v.Var = va

	va = make([]VXMLVar, 0)
	va = append(va, VXMLVar{Name: "ExecuteVXML"})
	va = append(va, VXMLVar{Name: "nResult"})
	va = append(va, VXMLVar{Name: "nResultCode", Expr: "0"})
	v.Form.Var = va
	v.Form.Body = body

	return v
}

func VXMLBlock(body interface{}) Block {
	b := Block{}
	b.Prompt.Bargein = true
	b.Prompt.Body = body
	return b
}

func VXMLField(resumeURL string, digits int) Field {
	f := Field{Name: "Digits", Type: fmt.Sprintf("%s?length=%d", "digits", digits)}
	f.Filled.Data.Name = "RespJSON"
	f.Filled.Data.Src = resumeURL
	f.Filled.Data.Namelist = "namelist"
	f.Filled.Data.Method = http.MethodPost
	f.Filled.Data.Enctype = "application/json"

	f.Filled.Assign = append(f.Filled.Assign, Assign{Name: "recieveddtmf", Expr: "Digits"})
	f.Filled.Assign = append(f.Filled.Assign, Assign{Name: "nResultCode", Expr: "JSON.parse(RespJSON).result"})

	f.Filled.If.Cond = "nResultCode === '1'"
	f.Filled.If.Goto.Next = resumeURL
	f.Filled.If.Else.Log.Value.Expr = "nResultCode"

	l := Log{Text: "Digits Value:: "}
	l.Value = append(l.Value, Value{Expr: "Digits"})
	f.Filled.Log = append(f.Filled.Log, l)

	l = Log{Text: "Recieveddtmf:: "}
	l.Value = append(l.Value, Value{Expr: "recieveddtmf"})
	f.Filled.Log = append(f.Filled.Log, l)

	l = Log{Text: "ExecuteVXML:: "}
	l.Value = append(l.Value, Value{Expr: "RespJSON"})
	f.Filled.Log = append(f.Filled.Log, l)

	l = Log{Text: "Response Code:: "}
	l.Value = append(l.Value, Value{Expr: "nResultCode"})
	f.Filled.Log = append(f.Filled.Log, l)

	return f
}

func init() {
	ivr.RegisterClientType(imiChannelType, NewClientFromChannel)
}

func NewClientFromChannel(channel *models.Channel) (ivr.Client, error) {
	sendURL := channel.ConfigValue(sendURLConfig, "")
	phoneNumber := channel.Address()
	username := channel.ConfigValue(usernameConfig, "")
	password := channel.ConfigValue(passwordConfig, "")

	if sendURL == "" || phoneNumber == "" || username == "" || password == "" {
		return nil, errors.Errorf("missing send_url, phonenumber, username or password on channel config: %v for channel: %s", channel.Config(), channel.UUID())
	}

	return &client{
		channel:         channel,
		sendURL:         sendURL,
		phoneNumber:     phoneNumber,
		accountUserName: username,
		accountPassword: password,
	}, nil
}

func (c *client) CallIDForRequest(r *http.Request) (string, error) {
	// TODO
	println("CallIDForRequest")
	return "", nil
}

func (c *client) DownloadMedia(url string) (*http.Response, error) {
	return http.Get(url)
}

// HangupCall asks IMIMobile to hang up the call that is passed in
func (c *client) HangupCall(client *http.Client, callIWriteErrorResponseD string) error {
	// TODO
	println("HangupCall")
	return nil
}

// InputForRequest returns the input for the passed in request, if any
func (c *client) InputForRequest(r *http.Request) (string, utils.Attachment, error) {
	// TODO
	println("InputForRequest")
	return r.Form.Get("recieveddtmf"), utils.Attachment(""), nil
}

func (c *client) PreprocessResume(ctx context.Context, db *sqlx.DB, rp *redis.Pool, conn *models.ChannelConnection, r *http.Request) ([]byte, error) {
	println("PreprocessResume")
	return nil, nil
}

// RequestCall causes this client to request a new outgoing call for this provider
func (c *client) RequestCall(client *http.Client, number urns.URN, callbackURL string, statusURL string) (ivr.CallID, error) {
	url, _ := url.Parse(callbackURL)
	callRequest := &CallRequest{
		TransID:  url.Query().Get("connection"),
		To:       formatPhoneNumber(number.Path()),
		From:     formatPhoneNumber(c.channel.Address()),
		VxmlURL:  callbackURL,
		EventURL: statusURL,
	}

	println("REQUEST CALL")
	println(callbackURL)
	println(statusURL)

	resp, err := c.makeRequest(client, http.MethodPost, c.sendURL, callRequest)

	if err != nil {
		return ivr.NilCallID, errors.Wrapf(err, "error trying to start call")
	}

	if resp.StatusCode != http.StatusOK {
		return ivr.NilCallID, errors.Errorf("received non 200 status for call start: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return ivr.NilCallID, errors.Wrapf(err, "error reading response body")
	}

	call := &CallResponse{}
	err = json.Unmarshal(body, call)
	if err != nil || call.TransID == "" {
		return ivr.NilCallID, errors.Errorf("unable to read call uuid")
	}

	if call.Status == statusFailed {
		return ivr.NilCallID, errors.Errorf("call status returned as failed")
	}

	logrus.WithField("body", string(body)).WithField("status", resp.StatusCode).Debug("requested call")

	return ivr.CallID(call.TransID), nil
}

func formatPhoneNumber(phoneNumber string) string {
	re := regexp.MustCompile(`([^0-9]+)`)
	return re.ReplaceAllString(phoneNumber, `$2`)
}

func (c *client) makeRequest(client *http.Client, method string, sendURL string, body interface{}) (*http.Response, error) {
	bb, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrapf(err, "error json encoding request")
	}

	req, _ := http.NewRequest(method, sendURL, bytes.NewReader(bb))
	req.SetBasicAuth(c.accountUserName, c.accountPassword)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	return client.Do(req)
}

// StatusForRequest returns the current call status for the passed in status (and optional duration if known)
func (c *client) StatusForRequest(r *http.Request) (models.ConnectionStatus, int) {
	// TODO
	println("StatusForRequest")
	return models.ConnectionStatusInProgress, 0
}

func (c *client) URNForRequest(r *http.Request) (urns.URN, error) {
	// TODO
	println("URNForRequest")
	return "", nil
}

// ValidateRequestSignature validates the signature on the passed in request, returning an error if it is invaled
func (c *client) ValidateRequestSignature(r *http.Request) error {
	return nil
}

// WriteEmptyResponse writes an empty (but valid) response
func (c *client) WriteEmptyResponse(w http.ResponseWriter, msg string) error {
	// TODO
	println("WriteEmptyResponse")
	return nil
}

// WriteErrorResponse writes an error / unavailable response
func (c *client) WriteErrorResponse(w http.ResponseWriter, err error) error {
	r := VXMLDocument(Hangup{})

	body, err := xml.Marshal(r)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(xml.Header + string(body)))
	return err
}

// WriteSessionResponse writes a VXML response for the events in the passed in session
func (c *client) WriteSessionResponse(session *models.Session, resumeURL string, r *http.Request, w http.ResponseWriter) error {
	// for errored sessions we should just output our error body
	if session.Status() == models.SessionStatusErrored {
		return errors.Errorf("cannot write IVR response for errored session")
	}

	// otherwise look for any say events
	sprint := session.Sprint()
	if sprint == nil {
		return errors.Errorf("cannot write IVR response for session with no sprint")
	}

	// get our response
	response, err := responseForSprint(resumeURL, session.Wait(), sprint.Events())
	if err != nil {
		return errors.Wrap(err, "unable to build response for IVR call")
	}

	_, err = w.Write([]byte(response))
	if err != nil {
		return errors.Wrap(err, "error writing IVR response")
	}

	return nil
}

func responseForSprint(resumeURL string, w flows.ActivatedWait, es []flows.Event) (string, error) {
	commands := make([]interface{}, 0)

	for _, e := range es {
		switch event := e.(type) {
		case *events.IVRCreatedEvent:
			if len(event.Msg.Attachments()) == 0 {
				block := VXMLBlock(event.Msg.Text())
				commands = append(commands, block)
			} else {
				for _, a := range event.Msg.Attachments() {
					a = models.NormalizeAttachment(a)
					block := VXMLBlock(Audio{Src: a.URL()})
					commands = append(commands, block)
				}
			}
		}
	}

	if w != nil {
		msgWait, isMsgWait := w.(*waits.ActivatedMsgWait)
		if !isMsgWait {
			return "", errors.Errorf("unable to use wait of type: %s in IVR call", w.Type())
		}

		switch hint := msgWait.Hint().(type) {
		case *hints.DigitsHint:
			digits := 1
			if hint.Count == nil {
				digits = 30
			}
			commands = append(commands, VXMLField(resumeURL, digits))
		case *hints.AudioHint:
			// TODO
			println("AUDIO HINT")
		default:
			return "", errors.Errorf("unable to use wait in IVR call, unknow type: %s", msgWait.Hint().Type())
		}
	} else {
		// no wait? call is over, hang up
		commands = append(commands, Hangup{})
	}

	r := VXMLDocument(commands)
	var body []byte
	var err error

	if indentMarshal {
		body, err = xml.MarshalIndent(r, "", "  ")
	} else {
		body, err = xml.Marshal(r)
	}
	if err != nil {
		return "", errors.Wrap(err, "unable to marshal vxml body")
	}

	return xml.Header + string(body), nil
}

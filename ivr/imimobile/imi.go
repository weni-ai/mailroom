package imi

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/flows/routers/waits"
	"github.com/nyaruka/goflow/flows/routers/waits/hints"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/ivr"
	"github.com/nyaruka/mailroom/models"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	validator "gopkg.in/go-playground/validator.v9"
)

const (
	imiChannelType    = models.ChannelType("IMI")
	statusFailed      = "1"
	sendURLConfig     = "send_url"
	phoneNumberConfig = "phone_number"
	usernameConfig    = "username"
	passwordConfig    = "password"
)

var indentMarshal = false
var validate = validator.New()
var vxmlResultField = `<log> Digits Value <value expr="Digits" /></log><assign expr="Digits" name="recieveddtmf"  /><log> recieveddtmf :: <value expr="recieveddtmf" /></log><data name="RespJSON" src="%s" namelist="recieveddtmf" method="post" enctype="application/json" /><log> ExecuteVXML :: <value expr="RespJSON" /></log><assign expr="JSON.parse(RespJSON).result" name="nResultCode" /><log>   Response Code:      <value expr="nResultCode" /></log><if cond="nResultCode === '1'"><log>This is get method API</log><log>Success Response Code Received. Moving to Next VXML</log><goto next="%s" /><else><log>  Invalid Response Code Received:: <value expr="nResultCode" /></log></else></if>`
var rp = redis.Pool{}

type ContentMapping struct {
	Encoded string
	Decoded string
}

var contentMappings = []ContentMapping{
	{Encoded: `&#34;`, Decoded: `"`},
	{Encoded: `&lt;`, Decoded: `<`},
	{Encoded: `&gt;`, Decoded: `>`},
	{Encoded: `&#39;`, Decoded: `'`},
	{Encoded: `&#xA;`, Decoded: ``},
	{Encoded: `&#x9;`, Decoded: ``},
}

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
	Text    string `xml:",chardata"`
	Prompt  struct {
		XMLName string      `xml:"prompt"`
		Text    string      `xml:",chardata"`
		Bargein bool        `xml:"bargein,attr"`
		Body    interface{} `xml:",innerxml"`
	}
}

type Audio struct {
	XMLName string `xml:"audio"`
	Text    string `xml:",chardata"`
	Src     string `xml:"src,attr"`
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

type Field struct {
	XMLName string `xml:"field"`
	Text    string `xml:",innerxml"`
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:"filled"`
}

type StatusRequest struct {
	XMLName  xml.Name `xml:"evt-notification"`
	Text     string   `xml:",chardata"`
	EvtID    string   `xml:"evt-id"`
	EvtDate  string   `xml:"evt-date"`
	EvtSeqno string   `xml:"evt-seqno"`
	EvtInfo  struct {
		Text          string `xml:",chardata"`
		Tid           string `xml:"tid"`
		Sid           string `xml:"sid"`
		Src           string `xml:"src"`
		Ani           string `xml:"ani"`
		Dnis          string `xml:"dnis"`
		CorrelationID string `xml:"correlationid"`
		OfferedOn     string `xml:"offered-on"`
		XParams       struct {
			Text   string `xml:",chardata"`
			XParam struct {
				Text  string `xml:",chardata"`
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"x-param"`
		} `xml:"x-params"`
	} `xml:"evt-info"`
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

func VXMLField(body string, digits int) Field {
	f := Field{
		Name: "Digits",
		Type: fmt.Sprintf("%s?length=%d", "digits", digits),
	}
	f.Body = body

	return f
}

func init() {
	ivr.RegisterClientType(imiChannelType, NewClientFromChannel)
	mailroom.AddInitFunction(StartRedisPool)
}

func StartRedisPool(mr *mailroom.Mailroom) error {
	rp = *mr.RP
	return nil
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

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == http.NoBody {
		return nil, nil
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, nil
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

func (c *client) CallIDForRequest(r *http.Request) (string, error) {
	body, err := readBody(r)
	status := &StatusRequest{}
	err = xml.Unmarshal(body, status)

	if err != nil {
		return "", errors.Wrapf(err, "error reading body from request")
	}

	callID := status.EvtInfo.CorrelationID
	if callID == "" {
		return "", errors.Errorf("no uuid set on call")
	}
	return callID, nil
}

func (c *client) DownloadMedia(url string) (*http.Response, error) {
	return http.Get(url)
}

// HangupCall asks IMIMobile to hang up the call that is passed in
func (c *client) HangupCall(client *http.Client, callIWriteErrorResponseD string) error {
	return nil
}

// InputForRequest returns the input for the passed in request, if any
func (c *client) InputForRequest(r *http.Request) (string, utils.Attachment, error) {
	return r.Form.Get("recieveddtmf"), utils.Attachment(""), nil
}

func (c *client) PreprocessResume(ctx context.Context, db *sqlx.DB, rp *redis.Pool, conn *models.ChannelConnection, r *http.Request) ([]byte, error) {
	connection := r.URL.Query().Get("connection")

	vxmlKey := fmt.Sprintf("imimobile_call_%s", connection)
	rc := rp.Get()
	vxmlResponse, _ := redis.String(rc.Do("GET", vxmlKey))
	defer rc.Close()

	if vxmlResponse != "" && r.Method == "GET" {
		return []byte(vxmlResponse), nil
	}

	return nil, nil
}

// RequestCall causes this client to request a new outgoing call for this provider
func (c *client) RequestCall(client *http.Client, number urns.URN, handleURL string, statusURL string) (ivr.CallID, error) {
	handleURL = formatImiUrl(handleURL)
	statusURL = formatImiUrl(statusURL)
	
	parseUrl, _ := url.Parse(handleURL)
	conn := parseUrl.Query().Get("connection")
	to := formatPhoneNumber(number.Path())
	from := formatPhoneNumber(c.channel.Address())

	callRequest := &CallRequest{
		TransID:  conn,
		To:       to,
		From:     from,
		VxmlURL:  handleURL,
		EventURL: statusURL,
	}

	logrus.WithFields(logrus.Fields{
		"VxmlURL":  handleURL,
		"EventURL": statusURL,
		"TransID":  conn,
		"To":       to,
		"From":     from,
	}).Info("IMIMobile request call")

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

func formatImiUrl(url string) string {
	imiUrl := strings.Replace(url, "https", "http", 1)
	imiUrl = strings.Replace(imiUrl, "rapidpro.ilhasoft.in", "rapidpro.ilhasoft.in:2454", 1)
	return imiUrl
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
func (c *client) StatusForRequest(r *http.Request, current models.ConnectionStatus) (models.ConnectionStatus, int) {
	if r.Form.Get("action") == "resume" {
		return models.ConnectionStatusInProgress, 0
	}

	status := &StatusRequest{}
	body, err := readBody(r)
	err = xml.Unmarshal(body, status)

	if err != nil {
		logrus.WithError(err).Error("error reading status request body")
		return models.ConnectionStatusErrored, 0
	}

	switch status.EvtID {
	case "offer":
		if current == models.ConnectionStatusCompleted {
			return models.ConnectionStatusCompleted, 0
		}
		return models.ConnectionStatusWired, 0

	case "accept", "answer":
		return models.ConnectionStatusInProgress, 0

	case "drop", "release":
		return models.ConnectionStatusCompleted, 0

	case "disconnect":
		return models.ConnectionStatusErrored, 0

	default:
		logrus.WithField("call_status", status).Error("unknown call status in ivr callback")
		return models.ConnectionStatusFailed, 0
	}
}

func (c *client) URNForRequest(r *http.Request) (urns.URN, error) {
	return "", nil
}

// ValidateRequestSignature validates the signature on the passed in request, returning an error if it is invaled
func (c *client) ValidateRequestSignature(r *http.Request) error {
	return nil
}

// WriteEmptyResponse writes an empty (but valid) response
func (c *client) WriteEmptyResponse(w http.ResponseWriter, msg string) error {
	msgBody := map[string]string{
		"description": msg,
	}
	body, err := json.Marshal(msgBody)
	if err != nil {
		return errors.Wrapf(err, "error marshalling imi message")
	}

	_, err = w.Write(body)
	return err
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
func (c *client) WriteSessionResponse(session *models.Session, number urns.URN, resumeURL string, r *http.Request, w http.ResponseWriter) error {
	resumeURL = formatImiUrl(resumeURL)
	// for errored sessions we should just output our error body
	if session.Status() == models.SessionStatusFailed {
		return errors.Errorf("cannot write IVR response for errored session")
	}

	// otherwise look for any say events
	sprint := session.Sprint()
	if sprint == nil {
		return errors.Errorf("cannot write IVR response for session with no sprint")
	}

	url, _ := url.Parse(resumeURL)
	conn := url.Query().Get("connection")
	vxmlKey := fmt.Sprintf("imimobile_call_%s", conn)

	responseToSave, err := responseForSprint(resumeURL, session.Wait(), sprint.Events())
	rc := rp.Get()
	rc.Do("SET", vxmlKey, string(responseToSave))
	defer rc.Close()

	if err != nil {
		return errors.Wrap(err, "unable to build response for IVR call")
	}

	recieveddtmf := r.Form.Get("recieveddtmf")
	if recieveddtmf != "" {
		msgBody := map[string]string{
			"result": "1",
		}
		msgResponse, _ := json.Marshal(msgBody)
		response := string(msgResponse)

		_, err = w.Write([]byte(response))
		return nil
	}

	_, err = w.Write([]byte(responseToSave))

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

			field := VXMLField(string(fmt.Sprintf(vxmlResultField, resumeURL, resumeURL)), digits)
			commands = append(commands, field)
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

	vxml := xml.Header + string(body)

	for _, mapping := range contentMappings {
		vxml = strings.ReplaceAll(vxml, mapping.Encoded, mapping.Decoded)
	}

	return vxml, nil
}

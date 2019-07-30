package imi

import (
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
	XMLName xml.Name `xml:"xml"`
	Text    string   `xml:",chardata"`
	Vxml    struct {
		Text     string         `xml:",chardata"`
		Version  string         `xml:"version,attr"`
		Property []VXMLProperty `xml:"property"`
		Var      []VXMLVar      `xml:"var"`
		Form     struct {
			Text string    `xml:",chardata"`
			ID   string    `xml:"id,attr"`
			Var  []VXMLVar `xml:"var"`
		} `xml:"form"`
	} `xml:"vxml"`
}

func initVXMLDocument() *VXMLBase {
	v := &VXMLBase{}
	v.Vxml.Version = "2.1"
	v.Vxml.Form.ID = "AcceptDigits"
	return v
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
	return "", ivr.NilAttachment, errors.Errorf("unknown wait_type: %s", "")
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
	println("ValidateRequestSignature")
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
	// TODO
	println("WriteErrorResponse")
	return nil
}

// WriteSessionResponse writes a TWIML response for the events in the passed in session
func (c *client) WriteSessionResponse(session *models.Session, resumeURL string, r *http.Request, w http.ResponseWriter) error {
	// TODO
	println("WriteSessionResponse")
	return nil
}

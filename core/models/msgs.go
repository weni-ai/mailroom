package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/gsm7"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/excellent"
	"github.com/nyaruka/goflow/excellent/types"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/definition/legacy/expressions"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"

	"github.com/gomodule/redigo/redis"
	"github.com/lib/pq"
	"github.com/lib/pq/hstore"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MsgID is our internal type for msg ids, which can be null/0
type MsgID null.Int

// NilMsgID is our constant for a nil msg id
const NilMsgID = MsgID(0)

type MsgDirection string

const (
	DirectionIn  = MsgDirection("I")
	DirectionOut = MsgDirection("O")
)

type MsgVisibility string

const (
	VisibilityVisible  = MsgVisibility("V")
	VisibilityArchived = MsgVisibility("A")
	VisibilityDeleted  = MsgVisibility("D")
)

type MsgType string

const (
	MsgTypeInbox = MsgType("I")
	MsgTypeFlow  = MsgType("F")
	MsgTypeIVR   = MsgType("V")
	MsgTypeUSSD  = MsgType("U")
)

type MsgStatus string

const (
	MsgStatusInitializing = MsgStatus("I")
	MsgStatusPending      = MsgStatus("P")
	MsgStatusQueued       = MsgStatus("Q")
	MsgStatusWired        = MsgStatus("W")
	MsgStatusSent         = MsgStatus("S")
	MsgStatusDelivered    = MsgStatus("D")
	MsgStatusHandled      = MsgStatus("H")
	MsgStatusErrored      = MsgStatus("E")
	MsgStatusFailed       = MsgStatus("F")
	MsgStatusResent       = MsgStatus("R")
)

type MsgFailedReason null.String

const (
	NilMsgFailedReason       = MsgFailedReason("")
	MsgFailedSuspended       = MsgFailedReason("S")
	MsgFailedLooping         = MsgFailedReason("L")
	MsgFailedErrorLimit      = MsgFailedReason("E")
	MsgFailedTooOld          = MsgFailedReason("O")
	MsgFailedNoDestination   = MsgFailedReason("D")
	MsgFailedSuspendTemplate = MsgFailedReason("T")
)

// BroadcastID is our internal type for broadcast ids, which can be null/0
type BroadcastID null.Int

// NilBroadcastID is our constant for a nil broadcast id
const NilBroadcastID = BroadcastID(0)

// TemplateState represents what state are templates are in, either already evaluated, not evaluated or
// that they are unevaluated legacy templates
type TemplateState string

const (
	TemplateStateEvaluated   = TemplateState("evaluated")
	TemplateStateLegacy      = TemplateState("legacy")
	TemplateStateUnevaluated = TemplateState("unevaluated")
)

// Msg is our type for mailroom messages
type Msg struct {
	m struct {
		ID                   flows.MsgID        `db:"id"              json:"id"`
		BroadcastID          BroadcastID        `db:"broadcast_id"    json:"broadcast_id,omitempty"`
		UUID                 flows.MsgUUID      `db:"uuid"            json:"uuid"`
		Text                 string             `db:"text"            json:"text"`
		HighPriority         bool               `db:"high_priority"   json:"high_priority"`
		CreatedOn            time.Time          `db:"created_on"      json:"created_on"`
		ModifiedOn           time.Time          `db:"modified_on"     json:"modified_on"`
		SentOn               *time.Time         `db:"sent_on"         json:"sent_on"`
		QueuedOn             time.Time          `db:"queued_on"       json:"queued_on"`
		Direction            MsgDirection       `db:"direction"       json:"direction"`
		Status               MsgStatus          `db:"status"          json:"status"`
		Visibility           MsgVisibility      `db:"visibility"      json:"visibility"`
		MsgType              MsgType            `db:"msg_type"        json:"-"`
		MsgCount             int                `db:"msg_count"       json:"tps_cost"`
		ErrorCount           int                `db:"error_count"     json:"error_count"`
		NextAttempt          *time.Time         `db:"next_attempt"    json:"next_attempt"`
		FailedReason         MsgFailedReason    `db:"failed_reason"   json:"-"`
		ExternalID           null.String        `db:"external_id"     json:"external_id"`
		ResponseToExternalID null.String        `                     json:"response_to_external_id"`
		Attachments          pq.StringArray     `db:"attachments"     json:"attachments"`
		Metadata             null.Map           `db:"metadata"        json:"metadata,omitempty"`
		ChannelID            ChannelID          `db:"channel_id"      json:"channel_id"`
		ChannelUUID          assets.ChannelUUID `                     json:"channel_uuid"`
		ContactID            ContactID          `db:"contact_id"      json:"contact_id"`
		ContactURNID         *URNID             `db:"contact_urn_id"  json:"contact_urn_id"`
		IsResend             bool               `                     json:"is_resend,omitempty"`
		URN                  urns.URN           `db:"urn_urn"         json:"urn"`
		URNAuth              null.String        `db:"urn_auth"        json:"urn_auth,omitempty"`
		OrgID                OrgID              `db:"org_id"          json:"org_id"`
		TopupID              TopupID            `db:"topup_id"        json:"-"`
		Template             null.String        `db:"template"        json:"template"`

		SessionID     SessionID     `json:"session_id,omitempty"`
		SessionStatus SessionStatus `json:"session_status,omitempty"`

		// These fields are set on the last outgoing message in a session's sprint. In the case
		// of the session being at a wait with a timeout then the timeout will be set. It is up to
		// Courier to update the session's timeout appropriately after sending the message.
		SessionWaitStartedOn *time.Time `json:"session_wait_started_on,omitempty"`
		SessionTimeout       int        `json:"session_timeout,omitempty"`
	}

	channel *Channel
}

func (m *Msg) ID() flows.MsgID                  { return m.m.ID }
func (m *Msg) BroadcastID() BroadcastID         { return m.m.BroadcastID }
func (m *Msg) UUID() flows.MsgUUID              { return m.m.UUID }
func (m *Msg) Channel() *Channel                { return m.channel }
func (m *Msg) Text() string                     { return m.m.Text }
func (m *Msg) HighPriority() bool               { return m.m.HighPriority }
func (m *Msg) CreatedOn() time.Time             { return m.m.CreatedOn }
func (m *Msg) ModifiedOn() time.Time            { return m.m.ModifiedOn }
func (m *Msg) SentOn() *time.Time               { return m.m.SentOn }
func (m *Msg) QueuedOn() time.Time              { return m.m.QueuedOn }
func (m *Msg) Direction() MsgDirection          { return m.m.Direction }
func (m *Msg) Status() MsgStatus                { return m.m.Status }
func (m *Msg) Visibility() MsgVisibility        { return m.m.Visibility }
func (m *Msg) MsgType() MsgType                 { return m.m.MsgType }
func (m *Msg) ErrorCount() int                  { return m.m.ErrorCount }
func (m *Msg) NextAttempt() *time.Time          { return m.m.NextAttempt }
func (m *Msg) FailedReason() MsgFailedReason    { return m.m.FailedReason }
func (m *Msg) ExternalID() null.String          { return m.m.ExternalID }
func (m *Msg) Metadata() map[string]interface{} { return m.m.Metadata.Map() }
func (m *Msg) MsgCount() int                    { return m.m.MsgCount }
func (m *Msg) ChannelID() ChannelID             { return m.m.ChannelID }
func (m *Msg) ChannelUUID() assets.ChannelUUID  { return m.m.ChannelUUID }
func (m *Msg) URN() urns.URN                    { return m.m.URN }
func (m *Msg) URNAuth() null.String             { return m.m.URNAuth }
func (m *Msg) OrgID() OrgID                     { return m.m.OrgID }
func (m *Msg) TopupID() TopupID                 { return m.m.TopupID }
func (m *Msg) ContactID() ContactID             { return m.m.ContactID }
func (m *Msg) ContactURNID() *URNID             { return m.m.ContactURNID }
func (m *Msg) IsResend() bool                   { return m.m.IsResend }
func (m *Msg) Template() null.String            { return m.m.Template }

func (m *Msg) SetTopup(topupID TopupID) { m.m.TopupID = topupID }

func (m *Msg) SetChannel(channel *Channel) {
	m.channel = channel
	if channel != nil {
		m.m.ChannelID = channel.ID()
		m.m.ChannelUUID = channel.UUID()
	} else {
		m.m.ChannelID = NilChannelID
		m.m.ChannelUUID = ""
	}
}

func (m *Msg) SetURN(urn urns.URN) error {
	// noop for nil urn
	if urn == urns.NilURN {
		return nil
	}

	m.m.URN = urn

	// set our ID if we have one
	urnInt := GetURNInt(urn, "id")
	if urnInt == 0 {
		return errors.Errorf("missing urn id on urn: %s", urn)
	}

	urnID := URNID(urnInt)
	m.m.ContactURNID = &urnID
	m.m.URNAuth = GetURNAuth(urn)

	return nil
}

func (m *Msg) Attachments() []utils.Attachment {
	attachments := make([]utils.Attachment, len(m.m.Attachments))
	for i := range m.m.Attachments {
		attachments[i] = utils.Attachment(m.m.Attachments[i])
	}
	return attachments
}

func (m *Msg) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.m)
}

// NewIncomingIVR creates a new incoming IVR message for the passed in text and attachment
func NewIncomingIVR(cfg *runtime.Config, orgID OrgID, conn *ChannelConnection, in *flows.MsgIn, createdOn time.Time) *Msg {
	msg := &Msg{}
	m := &msg.m

	msg.SetURN(in.URN())
	m.UUID = in.UUID()
	m.Text = in.Text()
	m.Direction = DirectionIn
	m.Status = MsgStatusHandled
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeIVR
	m.ContactID = conn.ContactID()

	urnID := conn.ContactURNID()
	m.ContactURNID = &urnID
	m.ChannelID = conn.ChannelID()

	m.OrgID = orgID
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn

	// add any attachments
	for _, a := range in.Attachments() {
		m.Attachments = append(m.Attachments, string(NormalizeAttachment(cfg, a)))
	}

	return msg
}

// NewOutgoingIVR creates a new IVR message for the passed in text with the optional attachment
func NewOutgoingIVR(cfg *runtime.Config, orgID OrgID, conn *ChannelConnection, out *flows.MsgOut, createdOn time.Time) *Msg {
	msg := &Msg{}
	m := &msg.m

	msg.SetURN(out.URN())
	m.UUID = out.UUID()
	m.Text = out.Text()
	m.HighPriority = false
	m.Direction = DirectionOut
	m.Status = MsgStatusWired
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeIVR
	m.ContactID = conn.ContactID()

	urnID := conn.ContactURNID()
	m.ContactURNID = &urnID
	m.ChannelID = conn.ChannelID()

	m.URN = out.URN()

	m.OrgID = orgID
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn
	m.SentOn = &createdOn

	// if we have attachments, add them
	for _, a := range out.Attachments() {
		m.Attachments = append(m.Attachments, string(NormalizeAttachment(cfg, a)))
	}

	return msg
}

var msgRepetitionsScript = redis.NewScript(3, `
local key, contact_id, text = KEYS[1], KEYS[2], KEYS[3]
local count = 1

-- try to look up in window
local record = redis.call("HGET", key, contact_id)
if record then
	local record_count = tonumber(string.sub(record, 1, 2))
	local record_text = string.sub(record, 4, -1)

	if record_text == text then 
		count = math.min(record_count + 1, 99)
	else
		count = 1
	end		
end

-- create our new record with our updated count
record = string.format("%02d:%s", count, text)

-- write our new record with updated count and set expiration
redis.call("HSET", key, contact_id, record)
redis.call("EXPIRE", key, 300)

return count
`)

// GetMsgRepetitions gets the number of repetitions of this msg text for the given contact in the current 5 minute window
func GetMsgRepetitions(rp *redis.Pool, contactID ContactID, msg *flows.MsgOut) (int, error) {
	rc := rp.Get()
	defer rc.Close()

	keyTime := dates.Now().UTC().Round(time.Minute * 5)
	key := fmt.Sprintf("msg_repetitions:%s", keyTime.Format("2006-01-02T15:04"))
	return redis.Int(msgRepetitionsScript.Do(rc, key, contactID, msg.Text()))
}

// GetWppMsgRepetitions gets the number of repetitions of this msg text for the given contact in the current 5 minute window
func GetWppMsgRepetitions(rp *redis.Pool, contactID ContactID, msg *flows.MsgWppOut) (int, error) {
	rc := rp.Get()
	defer rc.Close()

	keyTime := dates.Now().UTC().Round(time.Minute * 5)
	key := fmt.Sprintf("msg_repetitions:%s", keyTime.Format("2006-01-02T15:04"))
	return redis.Int(msgRepetitionsScript.Do(rc, key, contactID, msg.Text()))
}

// NewOutgoingFlowMsg creates an outgoing message for the passed in flow message
func NewOutgoingFlowMsg(rt *runtime.Runtime, org *Org, channel *Channel, session *Session, out *flows.MsgOut, createdOn time.Time) (*Msg, error) {
	return newOutgoingMsg(rt, org, channel, session.ContactID(), out, createdOn, session, NilBroadcastID, nil)
}

// NewOutgoingFlowMsgCatalog creates an outgoing message for the passed in flow message
func NewOutgoingFlowMsgCatalog(rt *runtime.Runtime, org *Org, channel *Channel, session *Session, out *flows.MsgCatalogOut, createdOn time.Time) (*Msg, error) {
	return newOutgoingMsgCatalog(rt, org, channel, session.ContactID(), out, createdOn, session, NilBroadcastID)
}

// NewOutgoingFlowMsgWpp creates an outgoing message for the passed in flow message
func NewOutgoingFlowMsgWpp(rt *runtime.Runtime, org *Org, channel *Channel, session *Session, out *flows.MsgWppOut, createdOn time.Time) (*Msg, error) {
	return newOutgoingMsgWpp(rt, org, channel, session.ContactID(), out, createdOn, session, NilBroadcastID)
}

// NewOutgoingBroadcastMsg creates an outgoing message which is part of a broadcast
func NewOutgoingBroadcastMsg(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, out *flows.MsgOut, createdOn time.Time, broadcastID BroadcastID, extraMetadata map[string]interface{}) (*Msg, error) {
	return newOutgoingMsg(rt, org, channel, contactID, out, createdOn, nil, broadcastID, extraMetadata)
}

func NewOutgoingWppBroadcastMsg(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, out *flows.MsgWppOut, createdOn time.Time, broadcastID BroadcastID) (*Msg, error) {
	return newOutgoingMsgWpp(rt, org, channel, contactID, out, createdOn, nil, broadcastID)
}

// NewOutgoingMsg creates an outgoing message that does not belong to any flow or broadcast, it's used to the a direct message to the contact
func NewOutgoingMsg(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, out *flows.MsgOut, createdOn time.Time, extraMetadata map[string]interface{}) (*Msg, error) {
	return newOutgoingMsg(rt, org, channel, contactID, out, createdOn, nil, NilBroadcastID, extraMetadata)
}

func newOutgoingMsg(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, out *flows.MsgOut, createdOn time.Time, session *Session, broadcastID BroadcastID, extraMetadata map[string]interface{}) (*Msg, error) {
	msg := &Msg{}
	m := &msg.m
	m.UUID = out.UUID()
	m.Text = out.Text()
	m.HighPriority = false
	m.Direction = DirectionOut
	m.Status = MsgStatusQueued
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeFlow
	m.MsgCount = 1
	m.ContactID = contactID
	m.BroadcastID = broadcastID
	m.OrgID = org.ID()
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn
	m.Template = null.NullString

	msg.SetChannel(channel)
	msg.SetURN(out.URN())

	if org.Suspended() {
		// we fail messages for suspended orgs right away
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedSuspended
	} else if msg.URN() == urns.NilURN || channel == nil {
		// if msg is missing the URN or channel, we also fail it
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedNoDestination
	} else {
		// also fail right away if this looks like a loop
		repetitions, err := GetMsgRepetitions(rt.RP, contactID, out)
		if err != nil {
			return nil, errors.Wrap(err, "error looking up msg repetitions")
		}
		if repetitions >= 20 {
			m.Status = MsgStatusFailed
			m.FailedReason = MsgFailedLooping

			logrus.WithFields(logrus.Fields{"contact_id": contactID, "text": out.Text(), "repetitions": repetitions}).Error("too many repetitions, failing message")
		}
	}

	// if we have a session, set fields on the message from that
	if session != nil {
		m.ResponseToExternalID = session.IncomingMsgExternalID()
		m.SessionID = session.ID()
		m.SessionStatus = session.Status()

		// if we're responding to an incoming message, send as high priority
		if session.IncomingMsgID() != NilMsgID {
			m.HighPriority = true
		}
	}

	// if we have attachments, add them
	if len(out.Attachments()) > 0 {
		for _, a := range out.Attachments() {
			m.Attachments = append(m.Attachments, string(NormalizeAttachment(rt.Config, a)))
		}
	}

	// populate metadata if we have any
	if len(out.QuickReplies()) > 0 || out.Templating() != nil || out.Topic() != flows.NilMsgTopic || out.TextLanguage != "" || out.IGCommentID() != "" || out.IGResponseType() != "" {
		metadata := make(map[string]interface{})
		if len(out.QuickReplies()) > 0 {
			metadata["quick_replies"] = out.QuickReplies()
		}
		if out.Templating() != nil {
			metadata["templating"] = out.Templating()
			m.Template = null.String(out.Templating().Template().Name)
		}
		if out.Topic() != flows.NilMsgTopic {
			metadata["topic"] = string(out.Topic())
		}
		if out.TextLanguage != "" {
			metadata["text_language"] = out.TextLanguage
		}
		if out.IGCommentID() != "" {
			metadata["ig_comment_id"] = out.IGCommentID()
		}
		if out.IGResponseType() != "" {
			metadata["ig_response_type"] = out.IGResponseType()
		}
		if out.IGTag() != "" {
			metadata["ig_tag"] = out.IGTag()
		}

		m.Metadata = null.NewMap(metadata)
	}

	if extraMetadata != nil {
		if len(m.Metadata.Map()) > 0 {
			extraMetadata = MergeMaps(extraMetadata, m.Metadata.Map())
		}
		m.Metadata = null.NewMap(extraMetadata)
	}

	// if we're sending to a phone, message may have to be sent in multiple parts
	if m.URN.Scheme() == urns.TelScheme {
		m.MsgCount = gsm7.Segments(m.Text) + len(m.Attachments)
	}

	return msg, nil
}

func newOutgoingMsgCatalog(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, msgCatalog *flows.MsgCatalogOut, createdOn time.Time, session *Session, broadcastID BroadcastID) (*Msg, error) {
	msg := &Msg{}
	m := &msg.m
	m.UUID = msgCatalog.UUID()
	m.Text = msgCatalog.Text()
	m.HighPriority = false
	m.Direction = DirectionOut
	m.Status = MsgStatusQueued
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeFlow
	m.MsgCount = 1
	m.ContactID = contactID
	m.BroadcastID = broadcastID
	m.OrgID = org.ID()
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn

	msg.SetChannel(channel)
	msg.SetURN(msgCatalog.URN())

	if org.Suspended() {
		// we fail messages for suspended orgs right away
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedSuspended
	} else if msg.URN() == urns.NilURN || channel == nil {
		// if msg is missing the URN or channel, we also fail it
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedNoDestination
	}

	// if we have a session, set fields on the message from that
	if session != nil {
		m.ResponseToExternalID = session.IncomingMsgExternalID()
		m.SessionID = session.ID()
		m.SessionStatus = session.Status()

		// if we're responding to an incoming message, send as high priority
		if session.IncomingMsgID() != NilMsgID {
			m.HighPriority = true
		}
	}

	// populate metadata if we have any
	if msgCatalog.Topic() != flows.NilMsgTopic || msgCatalog.TextLanguage != "" || msgCatalog.Header() != "" || msgCatalog.Body() != "" || msgCatalog.Footer() != "" || len(msgCatalog.Products()) != 0 {
		metadata := make(map[string]interface{})
		if msgCatalog.Topic() != flows.NilMsgTopic {
			metadata["topic"] = string(msgCatalog.Topic())
		}
		if msgCatalog.TextLanguage != "" {
			metadata["text_language"] = msgCatalog.TextLanguage
		}
		if msgCatalog.Header() != "" {
			metadata["header"] = string(msgCatalog.Header())
		}
		if msgCatalog.Body() != "" {
			metadata["body"] = string(msgCatalog.Body())
		}
		if msgCatalog.Footer() != "" {
			metadata["footer"] = string(msgCatalog.Footer())
		}
		if len(msgCatalog.Body()) != 0 {
			metadata["products"] = msgCatalog.Products()
		}
		if msgCatalog.Action() != "" {
			metadata["action"] = msgCatalog.Action()
		}
		if msgCatalog.Smart() {
			metadata["send_catalog"] = false
		} else {
			metadata["send_catalog"] = msgCatalog.SendCatalog()
		}

		m.Metadata = null.NewMap(metadata)
	}

	// if we're sending to a phone, message may have to be sent in multiple parts
	if m.URN.Scheme() == urns.TelScheme {
		m.MsgCount = gsm7.Segments(m.Text) + len(m.Attachments)
	}

	return msg, nil
}

func newOutgoingMsgWpp(rt *runtime.Runtime, org *Org, channel *Channel, contactID ContactID, msgWpp *flows.MsgWppOut, createdOn time.Time, session *Session, broadcastID BroadcastID) (*Msg, error) {
	msg := &Msg{}
	m := &msg.m
	m.UUID = msgWpp.UUID()
	m.Text = msgWpp.Text()
	m.HighPriority = true // by default, we send wpp messages as high priority since they're usually sent as an agent response
	m.Direction = DirectionOut
	m.Status = MsgStatusQueued
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeFlow
	m.MsgCount = 1
	m.ContactID = contactID
	m.BroadcastID = broadcastID
	m.OrgID = org.ID()
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn

	msg.SetChannel(channel)
	msg.SetURN(msgWpp.URN())

	suspend_template := org.o.Config.Get("suspend_template", false)

	if org.Suspended() {
		// we fail messages for suspended orgs right away
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedSuspended
	} else if msg.URN() == urns.NilURN || channel == nil {
		// if msg is missing the URN or channel, we also fail it
		m.Status = MsgStatusFailed
		m.FailedReason = MsgFailedNoDestination

		logrus.WithFields(logrus.Fields{"urn": msg.URN(), "channel": channel, "contact_id": msg.ContactID, "broadcast_id": msg.BroadcastID}).Error("nil urn or channel, failing message")
	} else {
		// also fail right away if this looks like a loop
		repetitions, err := GetWppMsgRepetitions(rt.RP, contactID, msgWpp)
		if err != nil {
			return nil, errors.Wrap(err, "error looking up msg repetitions")
		}
		if repetitions >= 20 {
			m.Status = MsgStatusFailed
			m.FailedReason = MsgFailedLooping

			logrus.WithFields(logrus.Fields{"contact_id": contactID, "text": msgWpp.Text(), "repetitions": repetitions}).Error("too many repetitions, failing message")
		}
	}

	// if we have a session, set fields on the message from that
	if session != nil {
		m.ResponseToExternalID = session.IncomingMsgExternalID()
		m.SessionID = session.ID()
		m.SessionStatus = session.Status()

		// if we're responding to an incoming message, send as high priority
		if session.IncomingMsgID() != NilMsgID {
			m.HighPriority = true
		}
	}

	// if we have attachments, add them
	if len(msgWpp.Attachments()) > 0 {
		for _, a := range msgWpp.Attachments() {
			m.Attachments = append(m.Attachments, string(NormalizeAttachment(rt.Config, a)))
		}
	}

	if len(msgWpp.QuickReplies()) > 0 || len(msgWpp.ListMessage().ListItems) > 0 || msgWpp.Topic() != flows.NilMsgTopic || msgWpp.Text() != "" || msgWpp.Footer() != "" || msgWpp.HeaderType() != "" || msgWpp.InteractionType() != "" || msgWpp.Templating() != nil || msgWpp.ActionType() != "" {
		metadata := make(map[string]interface{})
		if msgWpp.Topic() != flows.NilMsgTopic {
			metadata["topic"] = string(msgWpp.Topic())
		}
		if msgWpp.HeaderText() != "" {
			metadata["header_text"] = string(msgWpp.HeaderText())
			metadata["header_type"] = string(msgWpp.HeaderType())
		}
		if msgWpp.Text() != "" {
			metadata["text"] = string(msgWpp.Text())
		}
		if msgWpp.Footer() != "" {
			metadata["footer"] = string(msgWpp.Footer())
		}
		if len(msgWpp.Attachments()) > 0 {
			metadata["header_type"] = string(msgWpp.HeaderType())
		}
		if len(msgWpp.QuickReplies()) > 0 {
			metadata["quick_replies"] = msgWpp.QuickReplies()
			metadata["interaction_type"] = string(msgWpp.InteractionType())
		}
		if len(msgWpp.ListMessage().ListItems) > 0 {
			metadata["list_message"] = msgWpp.ListMessage()
			metadata["interaction_type"] = string(msgWpp.InteractionType())
		}
		if msgWpp.InteractionType() == "location" {
			metadata["interaction_type"] = string(msgWpp.InteractionType())
		}
		if msgWpp.InteractionType() == "cta_url" {
			metadata["interaction_type"] = msgWpp.InteractionType()
			metadata["cta_message"] = msgWpp.CTAMessage()
			metadata["cta_message"] = map[string]string{
				"url":          msgWpp.CTAMessage().URL_,
				"display_text": msgWpp.CTAMessage().DisplayText_,
			}
		}
		if msgWpp.InteractionType() == "flow_msg" {
			metadata["interaction_type"] = msgWpp.InteractionType()
			metadata["flow_message"] = msgWpp.FlowMessage()
			metadata["flow_message"] = map[string]interface{}{
				"flow_id":     msgWpp.FlowMessage().FlowID,
				"flow_screen": msgWpp.FlowMessage().FlowScreen,
				"flow_data":   msgWpp.FlowMessage().FlowData,
				"flow_cta":    msgWpp.FlowMessage().FlowCTA,
				"flow_mode":   msgWpp.FlowMessage().FlowMode,
			}
		}
		if msgWpp.InteractionType() == "order_details" {
			metadata["interaction_type"] = msgWpp.InteractionType()
			metadata["order_details_message"] = msgWpp.OrderDetailsMessage()
		}
		if len(msgWpp.Buttons()) > 0 {
			metadata["buttons"] = msgWpp.Buttons()
		}
		if msgWpp.TextLanguage != "" {
			metadata["text_language"] = msgWpp.TextLanguage
		}
		if msgWpp.Templating() != nil {
			metadata["templating"] = msgWpp.Templating()
			m.Template = null.String(msgWpp.Templating().Template().Name)
			m.HighPriority = false // template messages are usually sent for a large number of contacts, so we don't want to block other messages

			if ok, st := suspend_template.(bool); ok && st {
				m.Status = MsgStatusFailed
				m.FailedReason = MsgFailedSuspendTemplate // Suspend Template
			}

		}
		if len(msgWpp.Products()) > 0 {
			metadata["products"] = msgWpp.Products()
			metadata["body"] = msgWpp.Text()
		}
		if len(msgWpp.ActionButtonText()) != 0 {
			metadata["action"] = msgWpp.ActionButtonText()
		}
		metadata["send_catalog"] = false
		if msgWpp.SendCatalog() {
			metadata["send_catalog"] = true
			metadata["body"] = msgWpp.Text()
		}

		if (len(msgWpp.Products()) > 0 || msgWpp.SendCatalog()) && metadata["body"] != "" && msgWpp.Templating() == nil {
			metadata["text"] = ""
			metadata["header"] = string(msgWpp.HeaderText())
			m.Text = ""
		}

		if msgWpp.ActionType() != "" {
			metadata["action_type"] = msgWpp.ActionType()
		}
		if msgWpp.ActionExternalID() != "" {
			metadata["action_external_id"] = msgWpp.ActionExternalID()
		}

		m.Metadata = null.NewMap(metadata)
	}
	return msg, nil
}

// NewIncomingMsg creates a new incoming message for the passed in text and attachment
func NewIncomingMsg(cfg *runtime.Config, orgID OrgID, channel *Channel, contactID ContactID, in *flows.MsgIn, createdOn time.Time) *Msg {
	msg := &Msg{}

	msg.SetChannel(channel)
	msg.SetURN(in.URN())

	m := &msg.m
	m.UUID = in.UUID()
	m.Text = in.Text()
	m.Direction = DirectionIn
	m.Status = MsgStatusHandled
	m.Visibility = VisibilityVisible
	m.MsgType = MsgTypeFlow
	m.ContactID = contactID
	m.OrgID = orgID
	m.TopupID = NilTopupID
	m.CreatedOn = createdOn

	// add any attachments
	for _, a := range in.Attachments() {
		m.Attachments = append(m.Attachments, string(NormalizeAttachment(cfg, a)))
	}

	return msg
}

var loadMessagesSQL = `
SELECT 
	id,
	broadcast_id,
	uuid,
	text,
	created_on,
	direction,
	status,
	visibility,
	msg_count,
	error_count,
	next_attempt,
	failed_reason,
	coalesce(high_priority, FALSE) as high_priority,
	external_id,
	attachments,
	metadata,
	channel_id,
	contact_id,
	contact_urn_id,
	org_id,
	topup_id
FROM
	msgs_msg
WHERE
	org_id = $1 AND
	direction = $2 AND
	id = ANY($3)
ORDER BY
	id ASC`

// GetMessagesByID fetches the messages with the given ids
func GetMessagesByID(ctx context.Context, db Queryer, orgID OrgID, direction MsgDirection, msgIDs []MsgID) ([]*Msg, error) {
	return loadMessages(ctx, db, loadMessagesSQL, orgID, direction, pq.Array(msgIDs))
}

var loadMessagesForRetrySQL = `
SELECT 
	m.id,
	m.broadcast_id,
	m.uuid,
	m.text,
	m.created_on,
	m.direction,
	m.status,
	m.visibility,
	m.msg_count,
	m.error_count,
	m.next_attempt,
	m.failed_reason,
	m.high_priority,
	m.external_id,
	m.attachments,
	m.metadata,
	m.channel_id,
	m.contact_id,
	m.contact_urn_id,
	m.org_id,
	m.topup_id,
	u.identity AS "urn_urn",
	u.auth AS "urn_auth"
FROM
	msgs_msg m
INNER JOIN 
	contacts_contacturn u ON u.id = m.contact_urn_id
WHERE
	m.direction = 'O' AND
	m.status = 'E' AND
	m.next_attempt <= NOW()
ORDER BY
    m.next_attempt ASC, m.created_on ASC
LIMIT 5000`

func GetMessagesForRetry(ctx context.Context, db Queryer) ([]*Msg, error) {
	return loadMessages(ctx, db, loadMessagesForRetrySQL)
}

func loadMessages(ctx context.Context, db Queryer, sql string, params ...interface{}) ([]*Msg, error) {
	rows, err := db.QueryxContext(ctx, sql, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying msgs")
	}
	defer rows.Close()

	msgs := make([]*Msg, 0)
	channelIDsSeen := make(map[ChannelID]bool)
	channelIDs := make([]ChannelID, 0, 5)

	for rows.Next() {
		msg := &Msg{}
		err = rows.StructScan(&msg.m)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning msg row")
		}

		msgs = append(msgs, msg)

		if msg.ChannelID() != NilChannelID && !channelIDsSeen[msg.ChannelID()] {
			channelIDsSeen[msg.ChannelID()] = true
			channelIDs = append(channelIDs, msg.ChannelID())
		}
	}

	channels, err := GetChannelsByID(ctx, db, channelIDs)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching channels for messages")
	}

	channelsByID := make(map[ChannelID]*Channel)
	for _, ch := range channels {
		channelsByID[ch.ID()] = ch
	}

	for _, msg := range msgs {
		msg.SetChannel(channelsByID[msg.m.ChannelID])
	}

	return msgs, nil
}

var selectContactMessagesSQL = `
SELECT 
	id,
	broadcast_id,
	uuid,
	text,
	created_on,
	direction,
	status,
	visibility,
	msg_count,
	error_count,
	next_attempt,
	external_id,
	attachments,
	metadata,
	channel_id,
	contact_id,
	contact_urn_id,
	org_id,
	topup_id
FROM
	msgs_msg
WHERE
	contact_id = $1 AND
	created_on >= $2 AND
	status != 'F'
ORDER BY
	id ASC`

// SelectContactMessages loads the given messages for the passed in contact, created after the passed in time
func SelectContactMessages(ctx context.Context, db Queryer, contactID int, after time.Time) ([]*Msg, error) {
	rows, err := db.QueryxContext(ctx, selectContactMessagesSQL, contactID, after)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying msgs for contact: %d", contactID)
	}
	defer rows.Close()

	msgs := make([]*Msg, 0)
	for rows.Next() {
		msg := &Msg{}
		err = rows.StructScan(&msg.m)
		if err != nil {
			return nil, errors.Wrapf(err, "error scanning msg row")
		}

		msgs = append(msgs, msg)
	}

	return msgs, nil
}

// NormalizeAttachment will turn any relative URL in the passed in attachment and normalize it to
// include the full host for attachment domains
func NormalizeAttachment(cfg *runtime.Config, attachment utils.Attachment) utils.Attachment {
	// don't try to modify geo type attachments which are just coordinates
	if attachment.ContentType() == "geo" {
		return attachment
	}

	url := attachment.URL()
	if !strings.HasPrefix(url, "http") {
		if strings.HasPrefix(url, "/") {
			url = fmt.Sprintf("https://%s%s", cfg.AttachmentDomain, url)
		} else {
			url = fmt.Sprintf("https://%s/%s", cfg.AttachmentDomain, url)
		}
	}
	return utils.Attachment(fmt.Sprintf("%s:%s", attachment.ContentType(), url))
}

// SetTimeout sets the timeout for this message
func (m *Msg) SetTimeout(start time.Time, timeout time.Duration) {
	m.m.SessionWaitStartedOn = &start
	m.m.SessionTimeout = int(timeout / time.Second)
}

// InsertMessages inserts the passed in messages in a single query
func InsertMessages(ctx context.Context, tx Queryer, msgs []*Msg) error {
	is := make([]interface{}, len(msgs))
	for i := range msgs {
		is[i] = &msgs[i].m
	}

	return BulkQuery(ctx, "insert messages", tx, insertMsgSQL, is)
}

const insertMsgSQL = `
INSERT INTO
msgs_msg(uuid, text, high_priority, created_on, modified_on, queued_on, sent_on, direction, status, attachments, metadata,
		 visibility, msg_type, msg_count, error_count, next_attempt, failed_reason, channel_id,
		 contact_id, contact_urn_id, org_id, topup_id, broadcast_id, template)
  VALUES(:uuid, :text, :high_priority, :created_on, now(), now(), :sent_on, :direction, :status, :attachments, :metadata,
		 :visibility, :msg_type, :msg_count, :error_count, :next_attempt, :failed_reason, :channel_id,
		 :contact_id, :contact_urn_id, :org_id, :topup_id, :broadcast_id, :template)
RETURNING 
	id as id, 
	now() as modified_on,
	now() as queued_on
`

// UpdateMessage updates the passed in message status, visibility and msg type
func UpdateMessage(ctx context.Context, tx Queryer, msgID flows.MsgID, status MsgStatus, visibility MsgVisibility, msgType MsgType, topup TopupID) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE 
			msgs_msg 
		SET 
			status = $2,
			visibility = $3,
			msg_type = $4,
			topup_id = $5
		WHERE
			id = $1`,
		msgID, status, visibility, msgType, topup)

	if err != nil {
		return errors.Wrapf(err, "error updating msg: %d", msgID)
	}

	return nil
}

// MarkMessagesPending marks the passed in messages as pending(P)
func MarkMessagesPending(ctx context.Context, db Queryer, msgs []*Msg) error {
	return updateMessageStatus(ctx, db, msgs, MsgStatusPending)
}

// MarkMessagesQueued marks the passed in messages as queued(Q)
func MarkMessagesQueued(ctx context.Context, db Queryer, msgs []*Msg) error {
	return updateMessageStatus(ctx, db, msgs, MsgStatusQueued)
}

func updateMessageStatus(ctx context.Context, db Queryer, msgs []*Msg, status MsgStatus) error {
	is := make([]interface{}, len(msgs))
	for i, msg := range msgs {
		m := &msg.m
		m.Status = status
		is[i] = m
	}

	return BulkQuery(ctx, "updating message status", db, updateMsgStatusSQL, is)
}

const updateMsgStatusSQL = `
UPDATE 
	msgs_msg
SET
	status = m.status
FROM (
	VALUES(:id, :status)
) AS
	m(id, status)
WHERE
	msgs_msg.id = m.id::bigint
`

// BroadcastTranslation is the translation for the passed in language
type BroadcastTranslation struct {
	Text         string             `json:"text"`
	Attachments  []utils.Attachment `json:"attachments,omitempty"`
	QuickReplies []string           `json:"quick_replies,omitempty"`
}

// Broadcast represents a broadcast that needs to be sent
type Broadcast struct {
	b struct {
		BroadcastID   BroadcastID                             `json:"broadcast_id,omitempty" db:"id"`
		Translations  map[envs.Language]*BroadcastTranslation `json:"translations"`
		Text          hstore.Hstore                           `                              db:"text"`
		TemplateState TemplateState                           `json:"template_state"`
		BaseLanguage  envs.Language                           `json:"base_language"          db:"base_language"`
		URNs          []urns.URN                              `json:"urns,omitempty"`
		ContactIDs    []ContactID                             `json:"contact_ids,omitempty"`
		GroupIDs      []GroupID                               `json:"group_ids,omitempty"`
		OrgID         OrgID                                   `json:"org_id"                 db:"org_id"`
		ParentID      BroadcastID                             `json:"parent_id,omitempty"    db:"parent_id"`
		TicketID      TicketID                                `json:"ticket_id,omitempty"    db:"ticket_id"`
		BroadcastType events.BroadcastType                    `json:"broadcast_type"         db:"broadcast_type"`
	}
}

func (b *Broadcast) ID() BroadcastID                                       { return b.b.BroadcastID }
func (b *Broadcast) OrgID() OrgID                                          { return b.b.OrgID }
func (b *Broadcast) ContactIDs() []ContactID                               { return b.b.ContactIDs }
func (b *Broadcast) GroupIDs() []GroupID                                   { return b.b.GroupIDs }
func (b *Broadcast) URNs() []urns.URN                                      { return b.b.URNs }
func (b *Broadcast) BaseLanguage() envs.Language                           { return b.b.BaseLanguage }
func (b *Broadcast) Translations() map[envs.Language]*BroadcastTranslation { return b.b.Translations }
func (b *Broadcast) TemplateState() TemplateState                          { return b.b.TemplateState }
func (b *Broadcast) TicketID() TicketID                                    { return b.b.TicketID }
func (b *Broadcast) BroadcastType() events.BroadcastType                   { return b.b.BroadcastType }

func (b *Broadcast) MarshalJSON() ([]byte, error)    { return json.Marshal(b.b) }
func (b *Broadcast) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &b.b) }

// NewBroadcast creates a new broadcast with the passed in parameters
func NewBroadcast(
	orgID OrgID, id BroadcastID, translations map[envs.Language]*BroadcastTranslation,
	state TemplateState, baseLanguage envs.Language, urns []urns.URN, contactIDs []ContactID, groupIDs []GroupID, ticketID TicketID, broadcastType events.BroadcastType) *Broadcast {

	bcast := &Broadcast{}
	bcast.b.OrgID = orgID
	bcast.b.BroadcastID = id
	bcast.b.Translations = translations
	bcast.b.TemplateState = state
	bcast.b.BaseLanguage = baseLanguage
	bcast.b.URNs = urns
	bcast.b.ContactIDs = contactIDs
	bcast.b.GroupIDs = groupIDs
	bcast.b.TicketID = ticketID
	bcast.b.BroadcastType = broadcastType

	return bcast
}

// InsertChildBroadcast clones the passed in broadcast as a parent, then inserts that broadcast into the DB
func InsertChildBroadcast(ctx context.Context, db Queryer, parent *Broadcast) (*Broadcast, error) {
	child := NewBroadcast(
		parent.OrgID(),
		NilBroadcastID,
		parent.b.Translations,
		parent.b.TemplateState,
		parent.b.BaseLanguage,
		parent.b.URNs,
		parent.b.ContactIDs,
		parent.b.GroupIDs,
		parent.b.TicketID,
		parent.b.BroadcastType,
	)
	// populate our parent id
	child.b.ParentID = parent.ID()

	// populate text from our translations
	child.b.Text.Map = make(map[string]sql.NullString)
	for lang, t := range child.b.Translations {
		child.b.Text.Map[string(lang)] = sql.NullString{String: t.Text, Valid: true}
		if len(t.Attachments) > 0 || len(t.QuickReplies) > 0 {
			return nil, errors.Errorf("cannot clone broadcast with quick replies or attachments")
		}
	}

	// insert our broadcast
	err := BulkQuery(ctx, "inserting broadcast", db, insertBroadcastSQL, []interface{}{&child.b})
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting child broadcast for broadcast: %d", parent.ID())
	}

	// build up all our contact associations
	contacts := make([]interface{}, 0, len(child.b.ContactIDs))
	for _, contactID := range child.b.ContactIDs {
		contacts = append(contacts, &broadcastContact{
			BroadcastID: child.ID(),
			ContactID:   contactID,
		})
	}

	// insert our contacts
	err = BulkQuery(ctx, "inserting broadcast contacts", db, insertBroadcastContactsSQL, contacts)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting contacts for broadcast")
	}

	// build up all our group associations
	groups := make([]interface{}, 0, len(child.b.GroupIDs))
	for _, groupID := range child.b.GroupIDs {
		groups = append(groups, &broadcastGroup{
			BroadcastID: child.ID(),
			GroupID:     groupID,
		})
	}

	// insert our groups
	err = BulkQuery(ctx, "inserting broadcast groups", db, insertBroadcastGroupsSQL, groups)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting groups for broadcast")
	}

	// finally our URNs
	urns := make([]interface{}, 0, len(child.b.URNs))
	for _, urn := range child.b.URNs {
		urnID := GetURNID(urn)
		if urnID == NilURNID {
			return nil, errors.Errorf("attempt to insert new broadcast with URNs that do not have id: %s", urn)
		}
		urns = append(urns, &broadcastURN{
			BroadcastID: child.ID(),
			URNID:       urnID,
		})
	}

	// insert our urns
	err = BulkQuery(ctx, "inserting broadcast urns", db, insertBroadcastURNsSQL, urns)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting URNs for broadcast")
	}

	return child, nil
}

type broadcastURN struct {
	BroadcastID BroadcastID `db:"broadcast_id"`
	URNID       URNID       `db:"contacturn_id"`
}

type broadcastContact struct {
	BroadcastID BroadcastID `db:"broadcast_id"`
	ContactID   ContactID   `db:"contact_id"`
}

type broadcastGroup struct {
	BroadcastID BroadcastID `db:"broadcast_id"`
	GroupID     GroupID     `db:"contactgroup_id"`
}

const insertBroadcastSQL = `
INSERT INTO
	msgs_broadcast( org_id,  parent_id,  ticket_id, created_on, modified_on, status,  text,  base_language, send_all, broadcast_type)
			VALUES(:org_id, :parent_id, :ticket_id, NOW()     , NOW(),       'Q',    :text, :base_language, FALSE, :broadcast_type)
RETURNING
	id
`

const insertBroadcastContactsSQL = `
INSERT INTO
	msgs_broadcast_contacts( broadcast_id,  contact_id)
	                 VALUES(:broadcast_id,     :contact_id)
`

const insertBroadcastGroupsSQL = `
INSERT INTO
	msgs_broadcast_groups( broadcast_id,  contactgroup_id)
	               VALUES(:broadcast_id,     :contactgroup_id)
`

const insertBroadcastURNsSQL = `
INSERT INTO
	msgs_broadcast_urns( broadcast_id,  contacturn_id)
	             VALUES(:broadcast_id, :contacturn_id)
`

// NewBroadcastFromEvent creates a broadcast object from the passed in broadcast event
func NewBroadcastFromEvent(ctx context.Context, tx Queryer, org *OrgAssets, event *events.BroadcastCreatedEvent) (*Broadcast, error) {
	// converst our translations to our type
	translations := make(map[envs.Language]*BroadcastTranslation)
	for l, t := range event.Translations {
		translations[l] = &BroadcastTranslation{
			Text:         t.Text,
			Attachments:  t.Attachments,
			QuickReplies: t.QuickReplies,
		}
	}

	// resolve our contact references
	contactIDs, err := GetContactIDsFromReferences(ctx, tx, org.OrgID(), event.Contacts)
	if err != nil {
		return nil, errors.Wrapf(err, "error resolving contact references")
	}

	// and our groups
	groupIDs := make([]GroupID, 0, len(event.Groups))
	for i := range event.Groups {
		group := org.GroupByUUID(event.Groups[i].UUID)
		if group != nil {
			groupIDs = append(groupIDs, group.ID())
		}
	}

	return NewBroadcast(org.OrgID(), NilBroadcastID, translations, TemplateStateEvaluated, event.BaseLanguage, event.URNs, contactIDs, groupIDs, NilTicketID, event.BroadcastType), nil
}

func (b *Broadcast) CreateBatch(contactIDs []ContactID) *BroadcastBatch {
	batch := &BroadcastBatch{}
	batch.b.BroadcastID = b.b.BroadcastID
	batch.b.BaseLanguage = b.b.BaseLanguage
	batch.b.Translations = b.b.Translations
	batch.b.TemplateState = b.b.TemplateState
	batch.b.OrgID = b.b.OrgID
	batch.b.TicketID = b.b.TicketID
	batch.b.ContactIDs = contactIDs
	return batch
}

// BroadcastBatch represents a batch of contacts that need messages sent for
type BroadcastBatch struct {
	b struct {
		BroadcastID   BroadcastID                             `json:"broadcast_id,omitempty"`
		Translations  map[envs.Language]*BroadcastTranslation `json:"translations"`
		BaseLanguage  envs.Language                           `json:"base_language"`
		TemplateState TemplateState                           `json:"template_state"`
		URNs          map[ContactID]urns.URN                  `json:"urns,omitempty"`
		ContactIDs    []ContactID                             `json:"contact_ids,omitempty"`
		IsLast        bool                                    `json:"is_last"`
		OrgID         OrgID                                   `json:"org_id"`
		TicketID      TicketID                                `json:"ticket_id"`
	}
}

func (b *BroadcastBatch) BroadcastID() BroadcastID            { return b.b.BroadcastID }
func (b *BroadcastBatch) ContactIDs() []ContactID             { return b.b.ContactIDs }
func (b *BroadcastBatch) URNs() map[ContactID]urns.URN        { return b.b.URNs }
func (b *BroadcastBatch) SetURNs(urns map[ContactID]urns.URN) { b.b.URNs = urns }
func (b *BroadcastBatch) OrgID() OrgID                        { return b.b.OrgID }
func (b *BroadcastBatch) TicketID() TicketID                  { return b.b.TicketID }
func (b *BroadcastBatch) Translations() map[envs.Language]*BroadcastTranslation {
	return b.b.Translations
}
func (b *BroadcastBatch) TemplateState() TemplateState { return b.b.TemplateState }
func (b *BroadcastBatch) BaseLanguage() envs.Language  { return b.b.BaseLanguage }
func (b *BroadcastBatch) IsLast() bool                 { return b.b.IsLast }
func (b *BroadcastBatch) SetIsLast(last bool)          { b.b.IsLast = last }

func (b *BroadcastBatch) MarshalJSON() ([]byte, error)    { return json.Marshal(b.b) }
func (b *BroadcastBatch) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &b.b) }

func CreateBroadcastMessages(ctx context.Context, rt *runtime.Runtime, oa *OrgAssets, bcast *BroadcastBatch, extraMetadata map[string]interface{}) ([]*Msg, error) {
	repeatedContacts := make(map[ContactID]bool)
	broadcastURNs := bcast.URNs()

	// build our list of contact ids
	contactIDs := bcast.ContactIDs()

	// build a map of the contacts that are present both in our URN list and our contact id list
	if broadcastURNs != nil {
		for _, id := range contactIDs {
			_, found := broadcastURNs[id]
			if found {
				repeatedContacts[id] = true
			}
		}

		// if we have URN we need to send to, add those contacts as well if not already repeated
		for id := range broadcastURNs {
			if !repeatedContacts[id] {
				contactIDs = append(contactIDs, id)
			}
		}
	}

	// load all our contacts
	contacts, err := LoadContacts(ctx, rt.DB, oa, contactIDs)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading contacts for broadcast")
	}

	channels := oa.SessionAssets().Channels()

	// for each contact, build our message
	msgs := make([]*Msg, 0, len(contacts))

	// utility method to build up our message
	buildMessage := func(c *Contact, forceURN urns.URN) (*Msg, error) {
		if c.Status() != ContactStatusActive {
			return nil, nil
		}

		contact, err := c.FlowContact(oa)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating flow contact")
		}

		urn := urns.NilURN
		var channel *Channel

		// we are forcing to send to a non-preferred URN, find the channel
		if forceURN != urns.NilURN {
			for _, u := range contact.URNs() {
				if u.URN().Identity() == forceURN.Identity() {
					c := channels.GetForURN(u, assets.ChannelRoleSend)
					if c == nil {
						return nil, nil
					}
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		} else {
			// no forced URN, find the first URN we can send to
			for _, u := range contact.URNs() {
				c := channels.GetForURN(u, assets.ChannelRoleSend)
				if c != nil {
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		}

		// no urn and channel? move on
		if channel == nil {
			return nil, nil
		}

		// resolve our translations, the order is:
		//   1) valid contact language
		//   2) org default language
		//   3) broadcast base language
		lang := contact.Language()
		if lang != envs.NilLanguage {
			found := false
			for _, l := range oa.Env().AllowedLanguages() {
				if l == lang {
					found = true
					break
				}
			}
			if !found {
				lang = envs.NilLanguage
			}
		}

		// have a valid contact language, try that
		trans := bcast.Translations()
		t := trans[lang]

		// not found? try org default language
		if t == nil {
			t = trans[oa.Env().DefaultLanguage()]
		}

		// not found? use broadcast base language
		if t == nil {
			t = trans[bcast.BaseLanguage()]
		}

		if t == nil {
			logrus.WithField("base_language", bcast.BaseLanguage()).WithField("translations", trans).Error("unable to find translation for broadcast")
			return nil, nil
		}

		template := ""

		// if this is a legacy template, migrate it forward
		if bcast.TemplateState() == TemplateStateLegacy {
			template, _ = expressions.MigrateTemplate(t.Text, nil)
		} else if bcast.TemplateState() == TemplateStateUnevaluated {
			template = t.Text
		}

		text := t.Text

		// if we have a template, evaluate it
		if template != "" {
			// build up the minimum viable context for templates
			templateCtx := types.NewXObject(map[string]types.XValue{
				"contact": flows.Context(oa.Env(), contact),
				"fields":  flows.Context(oa.Env(), contact.Fields()),
				"globals": flows.Context(oa.Env(), oa.SessionAssets().Globals()),
				"urns":    flows.ContextFunc(oa.Env(), contact.URNs().MapContext),
			})
			text, _ = excellent.EvaluateTemplate(oa.Env(), templateCtx, template, nil)
		}

		// don't do anything if we have no text or attachments
		if text == "" && len(t.Attachments) == 0 {
			return nil, nil
		}

		// create our outgoing message
		out := flows.NewMsgOut(urn, channel.ChannelReference(), text, t.Attachments, t.QuickReplies, nil, flows.NilMsgTopic, "", "", "")
		msg, err := NewOutgoingBroadcastMsg(rt, oa.Org(), channel, c.ID(), out, time.Now(), bcast.BroadcastID(), extraMetadata)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating outgoing message")
		}

		return msg, nil
	}

	// run through all our contacts to create our messages
	for _, c := range contacts {
		// use the preferred URN if present
		urn := broadcastURNs[c.ID()]
		msg, err := buildMessage(c, urn)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating broadcast message")
		}
		if msg != nil {
			msgs = append(msgs, msg)
		}

		// if this is a contact that will receive two messages, calculate that one as well
		if repeatedContacts[c.ID()] {
			m2, err := buildMessage(c, urns.NilURN)
			if err != nil {
				return nil, errors.Wrapf(err, "error creating broadcast message")
			}

			// add this message if it isn't a duplicate
			if m2 != nil && m2.URN() != msg.URN() {
				msgs = append(msgs, m2)
			}
		}
	}

	// allocate a topup for these message if org uses topups
	topup, err := AllocateTopups(ctx, rt.DB, rt.RP, oa.Org(), len(msgs))
	if err != nil {
		return nil, errors.Wrapf(err, "error allocating topup for broadcast messages")
	}

	// if we have an active topup, assign it to our messages
	if topup != NilTopupID {
		for _, m := range msgs {
			m.SetTopup(topup)
		}
	}

	// insert them in a single request
	err = InsertMessages(ctx, rt.DB, msgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting broadcast messages")
	}

	// if the broadcast was a ticket reply, update the ticket
	if bcast.TicketID() != NilTicketID {
		err = updateTicketLastActivity(ctx, rt.DB, []TicketID{bcast.TicketID()}, dates.Now())
		if err != nil {
			return nil, errors.Wrapf(err, "error updating broadcast ticket")
		}
	}

	return msgs, nil
}

type WppBroadcastTemplate struct {
	UUID      assets.TemplateUUID `json:"uuid" validate:"required,uuid"`
	Name      string              `json:"name" validate:"required"`
	Variables []string            `json:"variables,omitempty"`
	Locale    string              `json:"locale,omitempty" validate:"omitempty,bcp47"`
}

type WppBroadcastMessageHeader struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type WppBroadcastCatalogMessage struct {
	Products         []flows.ProductEntry `json:"products,omitempty"`
	ActionButtonText string               `json:"action_button_text,omitempty"`
	SendCatalog      bool                 `json:"send_catalog,omitempty"`
}

type WppBroadcastMessage struct {
	Text             string                     `json:"text,omitempty"`
	Header           WppBroadcastMessageHeader  `json:"header,omitempty"`
	Footer           string                     `json:"footer,omitempty"`
	Attachments      []utils.Attachment         `json:"attachments,omitempty"`
	QuickReplies     []string                   `json:"quick_replies,omitempty"`
	Template         WppBroadcastTemplate       `json:"template,omitempty"`
	InteractionType  string                     `json:"interaction_type,omitempty"`
	OrderDetails     flows.OrderDetailsMessage  `json:"order_details,omitempty"`
	FlowMessage      flows.FlowMessage          `json:"flow_message,omitempty"`
	ListMessage      flows.ListMessage          `json:"list_message,omitempty"`
	CTAMessage       flows.CTAMessage           `json:"cta_message,omitempty"`
	Buttons          []flows.ButtonComponent    `json:"buttons,omitempty"`
	CatalogMessage   WppBroadcastCatalogMessage `json:"catalog_message,omitempty"`
	ActionExternalID string                     `json:"action_external_id,omitempty"`
	ActionType       string                     `json:"action_type,omitempty"`
}

type WppBroadcast struct {
	b struct {
		BroadcastID BroadcastID         `json:"broadcast_id,omitempty" db:"id"`
		URNs        []urns.URN          `json:"urns,omitempty"`
		ContactIDs  []ContactID         `json:"contact_ids,omitempty"`
		GroupIDs    []GroupID           `json:"group_ids,omitempty"`
		OrgID       OrgID               `json:"org_id"                 db:"org_id"`
		ParentID    BroadcastID         `json:"parent_id,omitempty"    db:"parent_id"`
		Msg         WppBroadcastMessage `json:"msg"`
		ChannelID   ChannelID           `json:"channel_id,omitempty"`
	}
}

func (b *WppBroadcast) ID() BroadcastID          { return b.b.BroadcastID }
func (b *WppBroadcast) OrgID() OrgID             { return b.b.OrgID }
func (b *WppBroadcast) ContactIDs() []ContactID  { return b.b.ContactIDs }
func (b *WppBroadcast) GroupIDs() []GroupID      { return b.b.GroupIDs }
func (b *WppBroadcast) URNs() []urns.URN         { return b.b.URNs }
func (b *WppBroadcast) Msg() WppBroadcastMessage { return b.b.Msg }
func (b *WppBroadcast) ChannelID() ChannelID     { return b.b.ChannelID }

func (b *WppBroadcast) MarshalJSON() ([]byte, error)    { return json.Marshal(b.b) }
func (b *WppBroadcast) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &b.b) }

func NewWppBroadcast(orgID OrgID, id BroadcastID, msg WppBroadcastMessage, urns []urns.URN, contactIDs []ContactID, groupIDs []GroupID, channelID ChannelID) *WppBroadcast {
	bcast := &WppBroadcast{}
	bcast.b.OrgID = orgID
	bcast.b.BroadcastID = id
	bcast.b.Msg = msg
	bcast.b.URNs = urns
	bcast.b.ContactIDs = contactIDs
	bcast.b.GroupIDs = groupIDs
	bcast.b.ChannelID = channelID

	return bcast
}

func (b *WppBroadcast) CreateBatch(contactIDs []ContactID) *WppBroadcastBatch {
	batch := &WppBroadcastBatch{}
	batch.b.BroadcastID = b.b.BroadcastID
	batch.b.Msg = b.b.Msg
	batch.b.OrgID = b.b.OrgID
	batch.b.ChannelID = b.b.ChannelID
	batch.b.ContactIDs = contactIDs
	return batch
}

type WppBroadcastBatch struct {
	b struct {
		BroadcastID BroadcastID            `json:"broadcast_id,omitempty"`
		Msg         WppBroadcastMessage    `json:"msg"`
		URNs        map[ContactID]urns.URN `json:"urns,omitempty"`
		ContactIDs  []ContactID            `json:"contact_ids,omitempty"`
		IsLast      bool                   `json:"is_last"`
		OrgID       OrgID                  `json:"org_id"`
		ChannelID   ChannelID              `json:"channel_id,omitempty"`
	}
}

func (b *WppBroadcastBatch) BroadcastID() BroadcastID            { return b.b.BroadcastID }
func (b *WppBroadcastBatch) ContactIDs() []ContactID             { return b.b.ContactIDs }
func (b *WppBroadcastBatch) URNs() map[ContactID]urns.URN        { return b.b.URNs }
func (b *WppBroadcastBatch) SetURNs(urns map[ContactID]urns.URN) { b.b.URNs = urns }
func (b *WppBroadcastBatch) OrgID() OrgID                        { return b.b.OrgID }
func (b *WppBroadcastBatch) Msg() WppBroadcastMessage            { return b.b.Msg }
func (b *WppBroadcastBatch) ChannelID() ChannelID                { return b.b.ChannelID }

func (b *WppBroadcastBatch) IsLast() bool        { return b.b.IsLast }
func (b *WppBroadcastBatch) SetIsLast(last bool) { b.b.IsLast = last }

func (b *WppBroadcastBatch) MarshalJSON() ([]byte, error)    { return json.Marshal(b.b) }
func (b *WppBroadcastBatch) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &b.b) }

func CreateWppBroadcastMessages(ctx context.Context, rt *runtime.Runtime, oa *OrgAssets, bcast *WppBroadcastBatch) ([]*Msg, error) {
	repeatedContacts := make(map[ContactID]bool)
	broadcastURNs := bcast.URNs()

	// build our list of contact ids
	contactIDs := bcast.ContactIDs()

	// build a map of the contacts that are present both in our URN list and our contact id list
	if broadcastURNs != nil {
		for _, id := range contactIDs {
			_, found := broadcastURNs[id]
			if found {
				repeatedContacts[id] = true
			}
		}

		// if we have URN we need to send to, add those contacts as well if not already repeated
		for id := range broadcastURNs {
			if !repeatedContacts[id] {
				contactIDs = append(contactIDs, id)
			}
		}
	}

	// load all our contacts
	contacts, err := LoadContacts(ctx, rt.DB, oa, contactIDs)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading contacts for broadcast")
	}

	channels := oa.SessionAssets().Channels()

	// for each contact, build our message
	msgs := make([]*Msg, 0, len(contacts))

	// Separate regular messages from typing_indicator actions
	regularMsgs := make([]*Msg, 0, len(contacts))
	typingIndicatorMsgs := make([]*Msg, 0)

	// utility method to build up our message
	buildMessage := func(c *Contact, forceURN urns.URN) (*Msg, error) {
		if c.Status() != ContactStatusActive {
			return nil, nil
		}

		contact, err := c.FlowContact(oa)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating flow contact")
		}

		urn := urns.NilURN
		var channel *Channel

		// we are forcing to send to a non-preferred URN, find the channel
		if forceURN != urns.NilURN {
			for _, u := range contact.URNs() {
				if u.URN().Identity() == forceURN.Identity() {
					c := channels.GetForURN(u, assets.ChannelRoleSend)
					if c == nil {
						return nil, nil
					}
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		} else {
			// no forced URN, find the first URN we can send to
			for _, u := range contact.URNs() {
				c := channels.GetForURN(u, assets.ChannelRoleSend)
				if c != nil {
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		}

		if bcast.ChannelID() != NilChannelID {
			channel = oa.ChannelByID(bcast.ChannelID())
		}

		// no urn and channel? move on
		if channel == nil {
			return nil, nil
		}

		// evaluate our message fields
		text := bcast.Msg().Text
		attachments := bcast.Msg().Attachments
		quickReplies := make([]string, len(bcast.Msg().QuickReplies))
		copy(quickReplies, bcast.Msg().QuickReplies)
		headerType := bcast.Msg().Header.Type
		headerText := bcast.Msg().Header.Text
		footerText := bcast.Msg().Footer
		var templating *flows.MsgTemplating = nil
		templateVariables := make([]string, len(bcast.Msg().Template.Variables))
		copy(templateVariables, bcast.Msg().Template.Variables)
		templateLocale := bcast.Msg().Template.Locale

		ctaMessage := bcast.Msg().CTAMessage
		listMessage := bcast.Msg().ListMessage
		flowMessage := bcast.Msg().FlowMessage
		orderDetails := bcast.Msg().OrderDetails

		products := bcast.Msg().CatalogMessage.Products
		actionButtonText := bcast.Msg().CatalogMessage.ActionButtonText
		sendCatalog := bcast.Msg().CatalogMessage.SendCatalog

		actionType := bcast.Msg().ActionType
		actionExternalID := bcast.Msg().ActionExternalID

		// build up the minimum viable context for evaluation
		evaluationCtx := types.NewXObject(map[string]types.XValue{
			"contact": flows.Context(oa.Env(), contact),
			"fields":  flows.Context(oa.Env(), contact.Fields()),
			"globals": flows.Context(oa.Env(), oa.SessionAssets().Globals()),
			"urns":    flows.ContextFunc(oa.Env(), contact.URNs().MapContext),
		})

		// evaluate our text
		text, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, text, nil)

		// evaluate our header text
		headerText, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, headerText, nil)

		// evaluate our footer text
		footerText, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, footerText, nil)

		// evaluate our action text
		actionButtonText, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, actionButtonText, nil)

		// evaluate our quick replies
		for i, qr := range quickReplies {
			quickReplies[i], _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, qr, nil)
		}

		//evaluate our template locale
		templateLocale, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, templateLocale, nil)

		// evaluate our template
		if bcast.Msg().Template.UUID != "" {
			// load our template
			var templateMatch assets.Template = nil
			for _, t := range oa.templates {
				if t.UUID() == bcast.Msg().Template.UUID {
					templateMatch = t
					break
				}
			}
			if templateMatch == nil {
				return nil, errors.Errorf("template not found: %s", bcast.Msg().Template.UUID)
			}

			// looks for a translation in these locales
			locales := []envs.Locale{
				contact.Locale(oa.Env()),
				oa.Env().DefaultLocale(),
			}

			if templateLocale != "" {
				parsedLocale, _ := envs.FromBCP47(templateLocale)
				if parsedLocale != envs.NilLocale {
					locales = append([]envs.Locale{parsedLocale}, locales...)
				}
			}

			translation := oa.SessionAssets().Templates().FindTranslation(bcast.Msg().Template.UUID, channel.ChannelReference(), locales)
			if translation != nil {
				// evaluate our variables
				evaluatedVariables := make([]string, len(templateVariables))
				for i, variable := range templateVariables {
					sub, err := excellent.EvaluateTemplate(oa.Env(), evaluationCtx, variable, nil)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to evaluate template variable")
					}
					evaluatedVariables[i] = sub
				}

				text = translation.Substitute(evaluatedVariables)
				var templateReference = assets.NewTemplateReference(bcast.Msg().Template.UUID, bcast.Msg().Template.Name)
				templating = flows.NewMsgTemplating(templateReference, translation.Language(), translation.Country(), evaluatedVariables, translation.Namespace())
			} else {
				return nil, errors.Errorf("translation not found for template: %s, in channel: %s", bcast.Msg().Template.UUID, channel.UUID())
			}
		}

		// evaluate our buttons
		buttons := make([]flows.ButtonComponent, 0)
		for _, button := range bcast.Msg().Buttons {
			var newButton flows.ButtonComponent
			newButton.SubType, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, button.SubType, nil)

			for _, param := range button.Parameters {
				var newParam flows.ButtonParam
				newParam.Type, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, param.Type, nil)
				newParam.Text, _ = excellent.EvaluateTemplate(oa.Env(), evaluationCtx, param.Text, nil)
				newButton.Parameters = append(newButton.Parameters, newParam)
			}

			buttons = append(buttons, newButton)
		}

		// don't do anything if we have no text or attachments
		if text == "" && len(attachments) == 0 && actionType == "" {
			return nil, nil
		}

		// create our outgoing message
		out := flows.NewMsgWppOut(urn, channel.ChannelReference(), bcast.Msg().InteractionType, headerType, headerText, text, footerText, ctaMessage, listMessage, flowMessage, orderDetails, attachments, quickReplies, buttons, templating, flows.NilMsgTopic, products, actionButtonText, sendCatalog, actionType, actionExternalID)
		msg, err := NewOutgoingWppBroadcastMsg(rt, oa.Org(), channel, c.ID(), out, time.Now(), bcast.BroadcastID())
		if err != nil {
			return nil, errors.Wrapf(err, "error creating outgoing message")
		}

		return msg, nil
	}

	// run through all our contacts to create our messages
	for _, c := range contacts {
		// use the preferred URN if present
		urn := broadcastURNs[c.ID()]
		msg, err := buildMessage(c, urn)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating broadcast message")
		}
		if msg != nil {
			msgs = append(msgs, msg)
		}

		// if this is a contact that will receive two messages, calculate that one as well
		if repeatedContacts[c.ID()] {
			m2, err := buildMessage(c, urns.NilURN)
			if err != nil {
				return nil, errors.Wrapf(err, "error creating broadcast message")
			}

			// add this message if it isn't a duplicate
			if m2 != nil && m2.URN() != msg.URN() {
				msgs = append(msgs, m2)
			}
		}
	}

	// Separate regular messages from typing_indicator actions
	for _, msg := range msgs {
		metadata := msg.Metadata()
		if metadata != nil {
			actionType, ok := metadata["action_type"].(string)
			if ok && actionType == "typing_indicator" {
				// This is a typing_indicator action - won't be saved in the database
				typingIndicatorMsgs = append(typingIndicatorMsgs, msg)
				continue
			}
		}
		// Normal message - will be saved in the database
		regularMsgs = append(regularMsgs, msg)
	}

	// allocate a topup for these message if org uses topups
	if len(regularMsgs) > 0 {
		topup, err := AllocateTopups(ctx, rt.DB, rt.RP, oa.Org(), len(regularMsgs))
		if err != nil {
			return nil, errors.Wrapf(err, "error allocating topup for broadcast messages")
		}

		// if we have an active topup, assign it to our messages
		if topup != NilTopupID {
			for _, m := range regularMsgs {
				m.SetTopup(topup)
			}
		}

		// Insert only regular messages into the database (not typing_indicator)
		err = InsertMessages(ctx, rt.DB, regularMsgs)
		if err != nil {
			return nil, errors.Wrapf(err, "error inserting broadcast messages")
		}
	}

	// Debug log if we have typing_indicators
	if len(typingIndicatorMsgs) > 0 {
		logrus.WithFields(logrus.Fields{
			"typing_indicators": len(typingIndicatorMsgs),
			"regular_msgs":      len(regularMsgs),
		}).Info("separating typing_indicator actions from regular messages")
	}

	// Return all messages (regular + typing_indicator) for courier to process
	allMessages := append(regularMsgs, typingIndicatorMsgs...)
	return allMessages, nil
}

const updateMsgForResendingSQL = `
	UPDATE
		msgs_msg m
	SET
		channel_id = r.channel_id::int,
		topup_id = r.topup_id::int,
		status = 'P',
		error_count = 0,
		failed_reason = NULL,
		queued_on = r.queued_on::timestamp with time zone,
		sent_on = NULL,
		modified_on = NOW()
	FROM (
		VALUES(:id, :channel_id, :topup_id, :queued_on)
	) AS
		r(id, channel_id, topup_id, queued_on)
	WHERE
		m.id = r.id::bigint
`

// ResendMessages prepares messages for resending by reselecting a channel and marking them as PENDING
func ResendMessages(ctx context.Context, db Queryer, rp *redis.Pool, oa *OrgAssets, msgs []*Msg) error {
	channels := oa.SessionAssets().Channels()
	resends := make([]interface{}, len(msgs))

	for i, msg := range msgs {
		// reselect channel for this message's URN
		urn, err := URNForID(ctx, db, oa, *msg.ContactURNID())
		if err != nil {
			return errors.Wrap(err, "error loading URN")
		}
		msg.m.URN = urn // needs to be set for queueing to courier

		contactURN, err := flows.ParseRawURN(channels, urn, assets.IgnoreMissing)
		if err != nil {
			return errors.Wrap(err, "error parsing URN")
		}

		ch := channels.GetForURN(contactURN, assets.ChannelRoleSend)
		if ch != nil {
			channel := oa.ChannelByUUID(ch.UUID())
			msg.m.ChannelID = channel.ID()
			msg.m.ChannelUUID = channel.UUID()
			msg.channel = channel
		} else {
			msg.m.ChannelID = NilChannelID
			msg.m.ChannelUUID = assets.ChannelUUID("")
			msg.channel = nil
		}

		// allocate a new topup for this message if org uses topups
		msg.m.TopupID, err = AllocateTopups(ctx, db, rp, oa.Org(), 1)
		if err != nil {
			return errors.Wrapf(err, "error allocating topup for message resending")
		}

		// mark message as being a resend so it will be queued to courier as such
		msg.m.Status = MsgStatusPending
		msg.m.QueuedOn = dates.Now()
		msg.m.SentOn = nil
		msg.m.ErrorCount = 0
		msg.m.FailedReason = ""
		msg.m.IsResend = true

		resends[i] = msg.m
	}

	// update the messages in the database
	err := BulkQuery(ctx, "updating messages for resending", db, updateMsgForResendingSQL, resends)
	if err != nil {
		return errors.Wrapf(err, "error updating messages for resending")
	}

	return nil
}

// MarkBroadcastSent marks the passed in broadcast as sent
func MarkBroadcastSent(ctx context.Context, db Queryer, id BroadcastID) error {
	// noop if it is a nil id
	if id == NilBroadcastID {
		return nil
	}

	_, err := db.ExecContext(ctx, `UPDATE msgs_broadcast SET status = 'S', modified_on = now() WHERE id = $1`, id)
	if err != nil {
		return errors.Wrapf(err, "error setting broadcast with id %d as sent", id)
	}
	return nil
}

func CreateOutgoingMessages(ctx context.Context, rt *runtime.Runtime, oa *OrgAssets, URNs []urns.URN, msgText string) ([]*Msg, error) {
	// grab our contacts from the passed urns
	urnContactIDs, err := GetOrCreateContactIDsFromURNs(ctx, rt.DB, oa, URNs)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load or create contact ids for urns")
	}

	// create a second map for easier id->urn lookup
	contactIDsUrn := make(map[ContactID]urns.URN)

	// create our contactIDs list removing any duplicates
	repeatedContacts := make(map[ContactID]bool)
	contactIDs := make([]ContactID, 0, len(urnContactIDs))
	for u, id := range urnContactIDs {
		contactIDsUrn[id] = u
		if !repeatedContacts[id] {
			contactIDs = append(contactIDs, id)
			repeatedContacts[id] = true
		}
	}

	// load all our contacts
	contacts, err := LoadContacts(ctx, rt.DB, oa, contactIDs)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading contacts for")
	}

	channels := oa.SessionAssets().Channels()

	// for each contact, build our message
	msgs := make([]*Msg, 0, len(contacts))

	// utility method to build up our message
	buildMessage := func(c *Contact, forceURN urns.URN) (*Msg, error) {
		if c.Status() != ContactStatusActive {
			return nil, nil
		}

		contact, err := c.FlowContact(oa)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating flow contact")
		}

		urn := urns.NilURN
		var channel *Channel

		// we are forcing to send to a non-preferred URN, find the channel
		if forceURN != urns.NilURN {
			for _, u := range contact.URNs() {
				if u.URN().Identity() == forceURN.Identity() {
					c := channels.GetForURN(u, assets.ChannelRoleSend)
					if c == nil {
						return nil, nil
					}
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		} else {
			// no forced URN, find the first URN we can send to
			for _, u := range contact.URNs() {
				c := channels.GetForURN(u, assets.ChannelRoleSend)
				if c != nil {
					urn = u.URN()
					channel = oa.ChannelByUUID(c.UUID())
					break
				}
			}
		}

		// no urn and channel? move on
		if channel == nil {
			return nil, nil
		}

		// create our outgoing message
		out := flows.NewMsgOut(urn, channel.ChannelReference(), msgText, nil, nil, nil, flows.NilMsgTopic, "", "", "")
		msg, err := NewOutgoingMsg(rt, oa.Org(), channel, c.ID(), out, time.Now(), nil)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating outgoing message")
		}

		return msg, nil
	}

	// run through all our contacts to create our messages
	for _, c := range contacts {
		// use the preferred URN if present
		urn := contactIDsUrn[c.ID()]
		msg, err := buildMessage(c, urn)
		if err != nil {
			return nil, errors.Wrapf(err, "error building new message")
		}
		if msg != nil {
			msgs = append(msgs, msg)
		}

		// if this is a contact that will receive two messages, calculate that one as well
		if repeatedContacts[c.ID()] {
			m2, err := buildMessage(c, urns.NilURN)
			if err != nil {
				return nil, errors.Wrapf(err, "error building new message")
			}

			// add this message if it isn't a duplicate
			if m2 != nil && m2.URN() != msg.URN() {
				msgs = append(msgs, m2)
			}
		}
	}

	// allocate a topup for these message if org uses topups
	topup, err := AllocateTopups(ctx, rt.DB, rt.RP, oa.Org(), len(msgs))
	if err != nil {
		return nil, errors.Wrapf(err, "error allocating topup for new messages")
	}

	// if we have an active topup, assign it to our messages
	if topup != NilTopupID {
		for _, m := range msgs {
			m.SetTopup(topup)
		}
	}

	// insert them in a single request
	err = InsertMessages(ctx, rt.DB, msgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting new messages")
	}

	return msgs, nil
}

// NilID implementations

// MarshalJSON marshals into JSON. 0 values will become null
func (i MsgID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

// UnmarshalJSON unmarshals from JSON. null values become 0
func (i *MsgID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

// Value returns the db value, null is returned for 0
func (i MsgID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

// Scan scans from the db value. null values become 0
func (i *MsgID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// MarshalJSON marshals into JSON. 0 values will become null
func (i BroadcastID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

// UnmarshalJSON unmarshals from JSON. null values become 0
func (i *BroadcastID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

// Value returns the db value, null is returned for 0
func (i BroadcastID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

// Scan scans from the db value. null values become 0
func (i *BroadcastID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// Value returns the db value, null is returned for ""
func (s MsgFailedReason) Value() (driver.Value, error) {
	return null.String(s).Value()
}

// Scan scans from the db value. null values become ""
func (s *MsgFailedReason) Scan(value interface{}) error {
	return null.ScanString(value, (*null.String)(s))
}

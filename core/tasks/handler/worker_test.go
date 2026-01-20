package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestToRouter(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)

	channel := testdata.InsertChannel(db, testdata.Org1, "TW", "Router Channel", []string{"tel"}, "SR", map[string]interface{}{"version": 2})
	contact := testdata.InsertContact(db, testdata.Org1, flows.ContactUUID(uuids.New()), "Router Contact", envs.Language("eng"))
	urn := urns.URN("tel:+250700000010")
	urnID := testdata.InsertContactURN(db, testdata.Org1, contact, urn, 1000)

	models.FlushCache()

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)

	modelContact, err := models.LoadContact(ctx, db, oa, contact.ID)
	require.NoError(t, err)
	require.NotNil(t, modelContact)

	flowContact, err := modelContact.FlowContact(oa)
	require.NoError(t, err)

	channelModel := oa.ChannelByID(channel.ID)
	require.NotNil(t, channelModel)

	type capturedRequest struct {
		method string
		path   string
		token  string
		body   []byte
	}

	reqCh := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqCh <- capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			token:  r.URL.Query().Get("token"),
			body:   body,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rt.Config.RouterBaseURL = server.URL
	rt.Config.RouterAuthToken = "router-token"

	metadata := json.RawMessage(`{"foo":"bar"}`)
	event := &MsgEvent{
		ContactID: contact.ID,
		OrgID:     testdata.Org1.ID,
		ChannelID: channel.ID,
		MsgID:     flows.MsgID(123),
		MsgUUID:   flows.MsgUUID(uuids.New()),
		URN:       urn,
		URNID:     urnID,
		Text:      "hello router",
		Metadata:  metadata,
	}

	projectUUID := uuids.New()
	err = requestToRouter(event, rt.Config, flowContact, projectUUID, channelModel)
	require.NoError(t, err)

	captured := <-reqCh
	assert.Equal(t, http.MethodPost, captured.method)
	assert.Equal(t, "/messages", captured.path)
	assert.Equal(t, "router-token", captured.token)

	var payload struct {
		ProjectUUID   string                 `json:"project_uuid"`
		ContactURN    string                 `json:"contact_urn"`
		Text          string                 `json:"text"`
		Metadata      json.RawMessage        `json:"metadata"`
		MsgEvent      json.RawMessage        `json:"msg_event"`
		ContactFields map[string]interface{} `json:"contact_fields"`
		ChannelUUID   string                 `json:"channel_uuid"`
		ChannelType   string                 `json:"channel_type"`
		ContactName   string                 `json:"contact_name"`
		StreamSupport bool                   `json:"stream_support"`
	}
	require.NoError(t, json.Unmarshal(captured.body, &payload))

	assert.Equal(t, string(projectUUID), payload.ProjectUUID)
	assert.Equal(t, string(event.URN.Identity()), payload.ContactURN)
	assert.Equal(t, event.Text, payload.Text)
	assert.Equal(t, string(channelModel.UUID()), payload.ChannelUUID)
	assert.Equal(t, string(channelModel.Type()), payload.ChannelType)
	assert.Equal(t, flowContact.Name(), payload.ContactName)
	assert.True(t, payload.StreamSupport)
	require.NotNil(t, payload.ContactFields)
	assert.Contains(t, payload.ContactFields, "age")
	assert.Contains(t, payload.ContactFields, "district")
	assert.Contains(t, payload.ContactFields, "gender")
	assert.Contains(t, payload.ContactFields, "joined")
	assert.Contains(t, payload.ContactFields, "state")
	assert.Contains(t, payload.ContactFields, "ward")
	assert.Nil(t, payload.ContactFields["age"])
	assert.Nil(t, payload.ContactFields["district"])
	assert.Nil(t, payload.ContactFields["gender"])
	assert.Nil(t, payload.ContactFields["joined"])
	assert.Nil(t, payload.ContactFields["state"])
	assert.Nil(t, payload.ContactFields["ward"])
	assert.JSONEq(t, string(metadata), string(payload.Metadata))

	var msgEvent MsgEvent
	require.NoError(t, json.Unmarshal(payload.MsgEvent, &msgEvent))
	assert.Equal(t, event.ContactID, msgEvent.ContactID)
	assert.Equal(t, event.OrgID, msgEvent.OrgID)
	assert.Equal(t, event.ChannelID, msgEvent.ChannelID)
	assert.Equal(t, event.MsgID, msgEvent.MsgID)
	assert.Equal(t, event.MsgUUID, msgEvent.MsgUUID)
	assert.Equal(t, event.URN, msgEvent.URN)
	assert.Equal(t, event.URNID, msgEvent.URNID)
	assert.Equal(t, event.Text, msgEvent.Text)
	assert.JSONEq(t, string(metadata), string(msgEvent.Metadata))
}

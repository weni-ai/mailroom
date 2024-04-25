package msg

import (
	"context"
	"net/http"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/msgio"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"

	"github.com/pkg/errors"
)

func init() {
	web.RegisterJSONRoute(http.MethodPost, "/mr/msg/resend", web.RequireAuthToken(handleResend))
	web.RegisterJSONRoute(http.MethodPost, "/mr/msg/send", web.RequireAuthToken(handleSend))
}

// Request to resend failed messages.
//
//   {
//     "org_id": 1,
//     "msg_ids": [123456, 345678]
//   }
//
type resendRequest struct {
	OrgID  models.OrgID   `json:"org_id"   validate:"required"`
	MsgIDs []models.MsgID `json:"msg_ids"  validate:"required"`
}

type sendRequest struct {
	UserEmail   string     `json:"user" validate:"required"`
	ProjectUUID string     `json:"project_uuid" validate:"required"`
	URNs        []urns.URN `json:"urns" validate:"required"`
	Text        string     `json:"text" validate:"required"`
}

// handles a request to resend the given messages
func handleResend(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &resendRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrapf(err, "request failed validation"), http.StatusBadRequest, nil
	}

	// grab our org
	oa, err := models.GetOrgAssets(ctx, rt, request.OrgID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org assets")
	}

	msgs, err := models.GetMessagesByID(ctx, rt.DB, request.OrgID, models.DirectionOut, request.MsgIDs)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error loading messages to resend")
	}

	err = models.ResendMessages(ctx, rt.DB, rt.RP, oa, msgs)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error resending messages")
	}

	msgio.SendMessages(ctx, rt, rt.DB, nil, msgs)

	// response is the ids of the messages that were actually resent
	resentMsgIDs := make([]flows.MsgID, len(msgs))
	for i, m := range msgs {
		resentMsgIDs[i] = m.ID()
	}
	return map[string]interface{}{"msg_ids": resentMsgIDs}, http.StatusOK, nil
}

func handleSend(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &sendRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrapf(err, "request failed validation"), http.StatusBadRequest, nil
	}

	// grab our org
	org, err := models.LoadOrgByProjectUUID(ctx, rt.Config, rt.DB, request.ProjectUUID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org for the given project uuid")
	}

	// grab our org assets
	oa, err := models.GetOrgAssets(ctx, rt, org.ID())
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org assets")
	}

	// create this message fot the given contacts
	msgs, err := models.CreateOutgoingMessages(ctx, rt, oa, request.URNs, request.Text)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error sending message")
	}

	msgio.SendMessages(ctx, rt, rt.DB, nil, msgs)

	// response is the ids of the messages that were sent
	msgsIDs := make([]flows.MsgID, len(msgs))
	for i, m := range msgs {
		msgsIDs[i] = m.ID()
	}
	return map[string]interface{}{"msg_ids": msgsIDs}, http.StatusOK, nil
}

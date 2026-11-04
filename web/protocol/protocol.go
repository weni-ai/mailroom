package protocol

import (
	"context"
	"net/http"
	"strconv"

	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
)

func init() {
	web.RegisterJSONRoute(http.MethodPost, "/mr/protocol/resolve", web.RequireAuthToken(handleResolve))
	web.RegisterJSONRoute(http.MethodPost, "/mr/protocol/csat", web.RequireAuthToken(handleCSAT))
	web.RegisterJSONRoute(http.MethodPost, "/mr/protocol/timer", web.RequireAuthToken(handleTimer))
}

type resolveRequest struct {
	ProjectID  string `json:"project_id" validate:"required"`
	URNID      int64  `json:"urn_id" validate:"required"`
	ContactID  int64  `json:"contact_id" validate:"required"`
	ChannelID  int64  `json:"channel_id"`
	ExternalID string `json:"external_id"`
	ProtocolID int64  `json:"protocol_id"`
}

type resolveResponse struct {
	ProtocolID    int64  `json:"protocol_id"`
	Created       bool   `json:"created"`
	PredecessorID *int64 `json:"predecessor_id"`
}

func handleResolve(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &resolveRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrap(err, "request failed validation"), http.StatusBadRequest, nil
	}
	org, err := models.LoadOrgByProjectUUID(ctx, rt.Config, rt.DB, request.ProjectID)
	if err != nil {
		return map[string]string{"error": "forbidden"}, http.StatusForbidden, nil
	}
	out, err := models.ResolveProtocol(ctx, rt.DB, models.ResolveInput{
		OrgID:      org.ID(),
		URNID:      models.URNID(request.URNID),
		ContactID:  models.ContactID(request.ContactID),
		ExternalID: request.ExternalID,
		ProtocolID: request.ProtocolID,
	})
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error resolving protocol")
	}
	return resolveResponse{ProtocolID: out.ProtocolID, Created: out.Created, PredecessorID: out.PredecessorID}, http.StatusOK, nil
}

type protocolRequest struct {
	ProjectID  string `json:"project_id" validate:"required"`
	ProtocolID int64  `json:"protocol_id" validate:"required"`
	Signal     string `json:"signal"`
}

func handleCSAT(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &protocolRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrap(err, "request failed validation"), http.StatusBadRequest, nil
	}
	org, err := models.LoadOrgByProjectUUID(ctx, rt.Config, rt.DB, request.ProjectID)
	if err != nil {
		return map[string]string{"error": "forbidden"}, http.StatusForbidden, nil
	}
	if err := models.CloseAIProtocol(ctx, rt.DB, org.ID(), request.ProtocolID); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]string{"protocol_id": strconv.FormatInt(request.ProtocolID, 10), "state": models.ProtocolClosed}, http.StatusOK, nil
}

func handleTimer(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &protocolRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrap(err, "request failed validation"), http.StatusBadRequest, nil
	}
	org, err := models.LoadOrgByProjectUUID(ctx, rt.Config, rt.DB, request.ProjectID)
	if err != nil {
		return map[string]string{"error": "forbidden"}, http.StatusForbidden, nil
	}
	switch request.Signal {
	case "on_hold":
		err = models.PauseProtocol(ctx, rt.DB, org.ID(), request.ProtocolID)
	case "resume":
		err = models.ResumeProtocol(ctx, rt.DB, org.ID(), request.ProtocolID)
	default:
		return map[string]string{"error": "validation"}, http.StatusBadRequest, nil
	}
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return map[string]string{"protocol_id": strconv.FormatInt(request.ProtocolID, 10), "signal": request.Signal}, http.StatusOK, nil
}

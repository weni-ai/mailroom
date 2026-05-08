package ticket

import (
	"context"
	"net/http"

	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
)

func init() {
	web.RegisterJSONRoute(http.MethodPost, "/mr/ticket/change_ticketer", web.RequireAuthToken(handleChangeTicketer))
}

type changeTicketerRequest struct {
	bulkTicketRequest

	TicketerID models.TicketerID `json:"ticketer_id" validate:"required"`
}

// Changes the ticketer of the tickets with the given ids
//
//   {
//     "org_id": 123,
//     "user_id": 234,
//     "ticket_ids": [1234, 2345],
//     "ticketer_id": 345
//   }
//
func handleChangeTicketer(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &changeTicketerRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrapf(err, "request failed validation"), http.StatusBadRequest, nil
	}

	// grab our org assets
	oa, err := models.GetOrgAssets(ctx, rt, request.OrgID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org assets")
	}

	// validate the target ticketer exists and is active in the org
	if oa.TicketerByID(request.TicketerID) == nil {
		return errors.Errorf("no such ticketer: %d", request.TicketerID), http.StatusBadRequest, nil
	}

	tickets, err := models.LoadTickets(ctx, rt.DB, request.TicketIDs)
	if err != nil {
		return nil, http.StatusBadRequest, errors.Wrapf(err, "error loading tickets for org: %d", request.OrgID)
	}

	evts, err := models.TicketsChangeTicketer(ctx, rt.DB, oa, request.UserID, tickets, request.TicketerID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error changing ticketer of tickets")
	}

	return newBulkResponse(evts), http.StatusOK, nil
}

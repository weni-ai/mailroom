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
	ExternalID *string           `json:"external_id,omitempty"`
}

// Changes the ticketer of the tickets with the given ids. The optional external_id is the
// identifier issued by the new ticketer's external system (e.g. the room UUID in wenichats).
// When provided, every affected ticket's external_id is overwritten with that value (an
// empty string clears it). When omitted, the existing external_id is preserved so the link
// to the external system is not silently lost.
//
//   {
//     "org_id": 123,
//     "user_id": 234,
//     "ticket_ids": [1234, 2345],
//     "ticketer_id": 345,
//     "external_id": "8ecb1e4a-b457-4645-a161-e2b02ddffa88"
//   }
//
func handleChangeTicketer(ctx context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
	request := &changeTicketerRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrapf(err, "request failed validation"), http.StatusBadRequest, nil
	}

	oa, err := models.GetOrgAssets(ctx, rt, request.OrgID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org assets")
	}

	if oa.TicketerByID(request.TicketerID) == nil {
		return errors.Errorf("no such ticketer: %d", request.TicketerID), http.StatusBadRequest, nil
	}

	tickets, err := models.LoadTickets(ctx, rt.DB, request.TicketIDs)
	if err != nil {
		return nil, http.StatusBadRequest, errors.Wrapf(err, "error loading tickets for org: %d", request.OrgID)
	}

	evts, err := models.TicketsChangeTicketer(ctx, rt.DB, oa, request.UserID, tickets, request.TicketerID, request.ExternalID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error changing ticketer of tickets")
	}

	return newBulkResponse(evts), http.StatusOK, nil
}

package hooks

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
)

var SendTicketsHistoryHook models.EventCommitHook = &sendTicketsHistoryHook{}

type sendTicketsHistoryHook struct{}

func (h *sendTicketsHistoryHook) Apply(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scenes map[*models.Scene][]interface{}) error {

	for s, ts := range scenes {
		session := s.Session()
		for _, t := range ts {
			ticket := t.(*models.Ticket)
			ticketer := oa.TicketerByID(ticket.TicketerID())
			if ticketer == nil {
				return errors.Errorf("can't find ticketer with id %d", ticket.TicketerID())
			}

			service, err := ticketer.AsService(rt.Config, flows.NewTicketer(ticketer))
			if err != nil {
				return err
			}

			logger := &models.HTTPLogger{}
			contactID := session.ContactID()
			runs := session.Runs()
			err = service.SendHistory(ticket, contactID, runs, logger.Ticketer(ticketer))
			logger.Insert(ctx, rt.DB)
			if err != nil {
				return err
			}
		}
	}
	return nil

}

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
	tickets := make([]*models.Ticket, 0, len(scenes))

	for _, ts := range scenes {
		for _, t := range ts {
			tickets = append(tickets, t.(*models.Ticket))
		}
	}

	for _, ticket := range tickets {
		ticketer := oa.TicketerByID(ticket.TicketerID())
		if ticketer == nil {
			return errors.Errorf("can't find ticketer with id %d", ticket.TicketerID())
		}

		service, err := ticketer.AsService(rt.Config, flows.NewTicketer(ticketer))
		if err != nil {
			return err
		}

		logger := &models.HTTPLogger{}
		err = service.SendHistory(ticket)
		logger.Insert(ctx, rt.DB)
		if err != nil {
			return err
		}
	}

	return nil

}

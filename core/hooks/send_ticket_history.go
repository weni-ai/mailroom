package hooks

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/tasks/tickets"
	"github.com/nyaruka/mailroom/runtime"
)

var SendTicketsHistoryHook models.EventCommitHook = &sendTicketsHistoryHook{}

type sendTicketsHistoryHook struct{}

func (h *sendTicketsHistoryHook) Apply(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scenes map[*models.Scene][]interface{}) error {

	for s, ts := range scenes {
		session := s.Session()
		for _, t := range ts {
			ticket := t.(*models.Ticket)
			rc := rt.RP.Get()
			defer rc.Close()
			err := tickets.QueueSendHistory(rc, ticket, session.ContactID())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

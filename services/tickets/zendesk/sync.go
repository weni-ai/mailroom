package zendesk

import (
	"context"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/tasks/handler"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/tickets"
	"github.com/pkg/errors"
)

const showManyBatchSize = 100

type syncItem struct {
	UUID          string `json:"uuid"`
	ExternalID    string `json:"external_id,omitempty"`
	ZendeskStatus string `json:"zendesk_status,omitempty"`
}

type syncCounts struct {
	OpenInMailroom    int `json:"open_in_mailroom"`
	InSync            int `json:"in_sync"`
	WouldClose        int `json:"would_close"`
	Closed            int `json:"closed"`
	MissingExternalID int `json:"missing_external_id"`
	NotFound          int `json:"not_found"`
}

type syncReport struct {
	Ticketer          assets.TicketerUUID `json:"ticketer"`
	Sync              bool                `json:"sync"`
	Counts            syncCounts          `json:"counts"`
	InSync            []syncItem          `json:"in_sync"`
	WouldClose        []syncItem          `json:"would_close"`
	Closed            []syncItem          `json:"closed"`
	MissingExternalID []syncItem          `json:"missing_external_id"`
	NotFound          []syncItem          `json:"not_found"`
}

func newSyncReport(ticketer assets.TicketerUUID, sync bool) *syncReport {
	return &syncReport{
		Ticketer:          ticketer,
		Sync:              sync,
		InSync:            []syncItem{},
		WouldClose:        []syncItem{},
		Closed:            []syncItem{},
		MissingExternalID: []syncItem{},
		NotFound:          []syncItem{},
	}
}

func chunkInt64s(ids []int64, size int) [][]int64 {
	if len(ids) == 0 {
		return nil
	}
	var chunks [][]int64
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

func classifyOpenTickets(tickets []*models.Ticket, zendeskStatusByID map[int64]string, report *syncReport) {
	for _, ticket := range tickets {
		ext := string(ticket.ExternalID())
		if ext == "" {
			report.MissingExternalID = append(report.MissingExternalID, syncItem{UUID: string(ticket.UUID())})
			continue
		}
		id, err := ParseNumericID(ext)
		if err != nil {
			report.NotFound = append(report.NotFound, syncItem{UUID: string(ticket.UUID()), ExternalID: ext})
			continue
		}
		status, ok := zendeskStatusByID[id]
		if !ok {
			report.NotFound = append(report.NotFound, syncItem{UUID: string(ticket.UUID()), ExternalID: ext})
			continue
		}
		item := syncItem{UUID: string(ticket.UUID()), ExternalID: ext, ZendeskStatus: status}
		if zendeskStatusIsClosed(status) {
			report.WouldClose = append(report.WouldClose, item)
		} else {
			report.InSync = append(report.InSync, item)
		}
	}
	report.Counts.OpenInMailroom = len(tickets)
	report.Counts.InSync = len(report.InSync)
	report.Counts.WouldClose = len(report.WouldClose)
	report.Counts.Closed = len(report.Closed)
	report.Counts.MissingExternalID = len(report.MissingExternalID)
	report.Counts.NotFound = len(report.NotFound)
}

func handleSync(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketerUUID := assets.TicketerUUID(chi.URLParam(r, "ticketer"))
	sync := r.URL.Query().Get("sync") == "true"

	ticketer, svc, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, typeZendesk)
	if err != nil || ticketer == nil {
		return errors.Errorf("no such ticketer %s", ticketerUUID), http.StatusNotFound, nil
	}

	zd := svc.(*service)

	open, err := models.LoadOpenTicketsForTicketer(ctx, rt.DB, ticketer.ID())
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "error loading open tickets")
	}

	// Fetch every parseable external ID before any local close. Empty and
	// unparseable IDs stay out of Zendesk and are classified from the partial map.
	ids := make([]int64, 0, len(open))
	for _, ticket := range open {
		ext := string(ticket.ExternalID())
		if ext == "" {
			continue
		}
		id, parseErr := ParseNumericID(ext)
		if parseErr != nil {
			continue
		}
		ids = append(ids, id)
	}

	zendeskStatusByID := map[int64]string{}
	for _, chunk := range chunkInt64s(ids, showManyBatchSize) {
		fetched, trace, fetchErr := zd.restClient.ShowManyTickets(chunk)
		if trace != nil {
			l.Ticketer(ticketer)(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, zd.redactor))
		}
		if fetchErr != nil {
			return errors.Wrap(fetchErr, "zendesk show_many"), http.StatusBadGateway, nil
		}
		for _, zt := range fetched {
			if zt == nil {
				continue
			}
			zendeskStatusByID[zt.ID] = zt.Status
		}
	}

	report := newSyncReport(ticketer.UUID(), sync)
	classifyOpenTickets(open, zendeskStatusByID, report)

	if sync && len(report.WouldClose) > 0 {
		openByUUID := make(map[string]*models.Ticket, len(open))
		for _, ticket := range open {
			openByUUID[string(ticket.UUID())] = ticket
		}

		toClose := make([]*models.Ticket, 0, len(report.WouldClose))
		for _, item := range report.WouldClose {
			ticket, ok := openByUUID[item.UUID]
			if !ok {
				return nil, http.StatusInternalServerError, errors.Errorf("open ticket %s missing for sync close", item.UUID)
			}
			toClose = append(toClose, ticket)
		}

		oa, err := models.GetOrgAssets(ctx, rt, ticketer.OrgID())
		if err != nil {
			return nil, http.StatusInternalServerError, errors.Wrap(err, "error loading org assets")
		}

		evts, err := models.CloseTickets(ctx, rt, oa, models.NilUserID, toClose, false, false, l, "")
		if err != nil {
			return nil, http.StatusInternalServerError, errors.Wrap(err, "error closing tickets")
		}

		rc := rt.RP.Get()
		defer rc.Close()

		for _, ticket := range toClose {
			evt, ok := evts[ticket]
			if !ok {
				return nil, http.StatusInternalServerError, errors.Errorf("missing closed event for ticket %s", ticket.UUID())
			}
			if err := handler.QueueTicketEvent(rc, ticket.ContactID(), evt); err != nil {
				return nil, http.StatusInternalServerError, errors.Wrapf(err, "error queueing ticket event for ticket %d", ticket.ID())
			}
		}

		report.Closed = append(report.Closed, report.WouldClose...)
		report.WouldClose = []syncItem{}
		report.Counts.WouldClose = 0
		report.Counts.Closed = len(report.Closed)
	}

	return report, http.StatusOK, nil
}

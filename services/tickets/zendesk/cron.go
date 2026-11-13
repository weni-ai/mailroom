package zendesk

import (
	"context"
	"sync"
	"time"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/utils/cron"

	"github.com/sirupsen/logrus"
)

const (
	zendeskSyncLock         = "zendesk_ticket_sync"
	defaultZendeskSyncHours = 12
	zendeskSyncRunTimeout   = time.Minute * 5
)

func init() {
	mailroom.AddInitFunction(startZendeskSyncCron)
}

// startZendeskSyncCron closes mailroom tickets that Zendesk has already solved or closed.
// The job is a no-op unless MAILROOM_ZENDESK_TICKET_SYNC is true.
// cron.Start uses a shared Redis lock (allInstances=false), so only one mailroom instance runs it at a time.
func startZendeskSyncCron(rt *runtime.Runtime, wg *sync.WaitGroup, quit chan bool) error {
	cron.Start(quit, rt, zendeskSyncLock, zendeskSyncInterval(rt), false,
		func() error {
			ctx, cancel := context.WithTimeout(context.Background(), zendeskSyncRunTimeout)
			defer cancel()
			return syncZendeskTickets(ctx, rt)
		},
	)
	return nil
}

func zendeskSyncInterval(rt *runtime.Runtime) time.Duration {
	hours := defaultZendeskSyncHours
	if rt != nil && rt.Config != nil && rt.Config.ZendeskTicketSyncInterval > 0 {
		hours = rt.Config.ZendeskTicketSyncInterval
	}
	return time.Duration(hours) * time.Hour
}

func syncZendeskTickets(ctx context.Context, rt *runtime.Runtime) error {
	if rt.Config == nil || !rt.Config.ZendeskTicketSync {
		return nil
	}

	log := logrus.WithField("comp", "zendesk_ticket_sync")
	ticketers, err := models.LoadActiveTicketersByType(ctx, rt.DB, typeZendesk)
	if err != nil {
		return err
	}

	for _, ticketer := range ticketers {
		tl := log.WithField("ticketer", ticketer.UUID())

		svc, err := ticketer.AsService(rt.Config, flows.NewTicketer(ticketer), ctx, rt.DB)
		if err != nil {
			tl.WithError(err).Error("error loading zendesk ticketer service")
			continue
		}
		zd, ok := svc.(*service)
		if !ok {
			tl.Error("ticketer service is not zendesk")
			continue
		}

		logger := &models.HTTPLogger{}
		report, status, syncErr := syncTicketer(ctx, rt, ticketer, zd, true, logger)
		if insertErr := logger.Insert(ctx, rt.DB); insertErr != nil {
			tl.WithError(insertErr).Error("error writing zendesk sync HTTP logs")
		}
		if syncErr != nil {
			tl.WithError(syncErr).WithField("status", status).Error("error syncing zendesk tickets")
			continue
		}

		tl.WithField("open", report.Counts.OpenInMailroom).
			WithField("closed", report.Counts.Closed).
			WithField("in_sync", report.Counts.InSync).
			WithField("missing_external_id", report.Counts.MissingExternalID).
			WithField("not_found", report.Counts.NotFound).
			Info("synced zendesk tickets")
	}

	return nil
}

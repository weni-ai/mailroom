package handlers

import (
	"context"
	"time"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/hooks"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
)

func init() {
	models.RegisterEventHandler(events.TypeMetaCalled, handleMetaCalled)
}

// handleServiceCalled is called for each service called event
func handleMetaCalled(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.MetaCalledEvent)

	run, _ := scene.Session().FindStep(e.StepUUID())
	flow, _ := oa.Flow(run.FlowReference().UUID)

	if flow != nil {
		// create a log for each HTTP call
		for _, httpLog := range event.HTTPLogs {
			logrus.WithFields(logrus.Fields{
				"contact_uuid": scene.ContactUUID(),
				"session_id":   scene.SessionID(),
				"url":          httpLog.URL,
				"status":       httpLog.Status,
				"elapsed_ms":   httpLog.ElapsedMS,
			}).Debug("meta called")

			log := models.NewWebhookCalledLog(
				oa.OrgID(),
				flow.(*models.Flow).ID(),
				httpLog.URL,
				httpLog.StatusCode,
				httpLog.Request,
				httpLog.Response,
				httpLog.Status != flows.CallStatusSuccess,
				time.Millisecond*time.Duration(httpLog.ElapsedMS),
				httpLog.Retries,
				event.CreatedOn(),
				scene.ContactID(),
			)
			scene.AppendToEventPreCommitHook(hooks.InsertHTTPLogsHook, log)
		}
	}

	return nil
}

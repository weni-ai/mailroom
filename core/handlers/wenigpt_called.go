package handlers

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/hooks"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
)

func init() {
	models.RegisterEventHandler(events.TypeWeniGPTCalled, handleWeniGPTCalled)
}

type WeniGPTCall struct {
	NodeUUID flows.NodeUUID
	Event    *events.WeniGPTCalledEvent
}

func handleWeniGPTCalled(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.WeniGPTCalledEvent)
	logrus.WithFields(logrus.Fields{
		"contact_uuid": scene.ContactUUID(),
		"session_id":   scene.SessionID(),
		"url":          event.URL,
		"status":       event.Status,
		"elapsed_ms":   event.ElapsedMS,
		"extraction":   event.Extraction,
	}).Debug("wenigpt called")

	_, step := scene.Session().FindStep(e.StepUUID())

	// pass node and response time to the hook that monitors webhook health
	scene.AppendToEventPreCommitHook(hooks.MonitorWebhooks, WeniGPTCall{NodeUUID: step.NodeUUID(), Event: event})

	return nil
}

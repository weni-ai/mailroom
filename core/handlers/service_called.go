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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	models.RegisterEventHandler(events.TypeServiceCalled, handleServiceCalled)
}

// handleServiceCalled is called for each service called event
func handleServiceCalled(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.ServiceCalledEvent)
	var classifier *models.Classifier
	var ticketer *models.Ticketer
	var externalService *models.ExternalService

	if event.Service == "classifier" {
		classifier = oa.ClassifierByUUID(event.Classifier.UUID)
		if classifier == nil {
			return errors.Errorf("unable to find classifier with UUID: %s", event.Classifier.UUID)
		}
	} else if event.Service == "ticketer" {
		ticketer = oa.TicketerByUUID(event.Ticketer.UUID)
		if ticketer == nil {
			return errors.Errorf("unable to find ticketer with UUID: %s", event.Ticketer.UUID)
		}
	} else if event.Service == "external_service" {
		externalService = oa.ExternalServiceByUUID(event.ExternalService.UUID)
		if externalService == nil {
			return errors.Errorf("unable to find external service with UUID: %s", event.ExternalService.UUID)
		}
	}

	// create a log for each HTTP call
	for _, httpLog := range event.HTTPLogs {
		logrus.WithFields(logrus.Fields{
			"contact_uuid": scene.ContactUUID(),
			"session_id":   scene.SessionID(),
			"url":          httpLog.URL,
			"status":       httpLog.Status,
			"elapsed_ms":   httpLog.ElapsedMS,
		}).Debug("service called")

		var log *models.HTTPLog

		if event.Service == "classifier" {
			log = models.NewClassifierCalledLog(
				oa.OrgID(),
				classifier.ID(),
				httpLog.URL,
				httpLog.StatusCode,
				httpLog.Request,
				httpLog.Response,
				httpLog.Status != flows.CallStatusSuccess,
				time.Duration(httpLog.ElapsedMS)*time.Millisecond,
				httpLog.Retries,
				httpLog.CreatedOn,
			)
		} else if event.Service == "ticketer" {
			log = models.NewTicketerCalledLog(
				oa.OrgID(),
				ticketer.ID(),
				httpLog.URL,
				httpLog.StatusCode,
				httpLog.Request,
				httpLog.Response,
				httpLog.Status != flows.CallStatusSuccess,
				time.Duration(httpLog.ElapsedMS)*time.Millisecond,
				httpLog.Retries,
				httpLog.CreatedOn,
			)
		} // else if event.Service == "external_service" {
		// 	log = models.NewExternalServiceCalledLog(
		// 		oa.OrgID(),
		// 		externalService.ID(),
		// 		httpLog.URL,
		// 		httpLog.StatusCode,
		// 		httpLog.Request,
		// 		httpLog.Response,
		// 		httpLog.Status != flows.CallStatusSuccess,
		// 		time.Duration(httpLog.ElapsedMS)*time.Millisecond,
		// 		httpLog.Retries,
		// 		httpLog.CreatedOn,
		// 	)
		// }

		scene.AppendToEventPreCommitHook(hooks.InsertHTTPLogsHook, log)
	}

	return nil
}

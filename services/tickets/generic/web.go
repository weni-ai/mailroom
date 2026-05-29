package generic

import (
	"context"
	"net/http"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"
)

func init() {
	base := "/mr/tickets/types/" + typeGeneric + "/event_callback/{ticketer:[a-f0-9\\-]+}"

	web.RegisterJSONRoute(http.MethodPost, base+"/messages", web.WithHTTPLogs(handleAgentMessage))
	web.RegisterJSONRoute(http.MethodPost, base+"/tickets/close", web.WithHTTPLogs(handleCloseTicket))
	web.RegisterJSONRoute(http.MethodPost, base+"/tickets/reopen", web.WithHTTPLogs(handleReopenTicket))
}

func handleAgentMessage(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	return map[string]string{"status": "not_implemented"}, http.StatusNotImplemented, nil
}

func handleCloseTicket(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	return map[string]string{"status": "not_implemented"}, http.StatusNotImplemented, nil
}

func handleReopenTicket(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	return map[string]string{"status": "not_implemented"}, http.StatusNotImplemented, nil
}

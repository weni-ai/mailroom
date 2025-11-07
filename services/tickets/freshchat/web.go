package freshchat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/tickets"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
)

func init() {
	base := "/mr/tickets/types/freshchat"

	web.RegisterJSONRoute(http.MethodPost, base+`/webhook/{ticketer:[a-f0-9\-]+}`, web.WithHTTPLogs(handleEventCallback))
}

type webhookRequest struct {
	RequestID string `json:"request_id"`
	Actor     struct {
		ActorType string `json:"actor_type"`
		ActorID   string `json:"actor_id"`
	}
	Action     string `json:"action"`
	ActionTime string `json:"action_time"`
	Data       struct {
		Message *Message `json:"message"`
		Resolve *Resolve `json:"resolve"`
		Reopen  *Reopen  `json:"reopen"`
	} `json:"data"`
}

type Resolve struct {
	Resolver      string        `json:"resolver"`
	Conversation  *Conversation `json:"conversation"`
	Users         User          `json:"users"`
	InteractionID string        `json:"interaction_id"`
}

type Reopen struct {
	Reopener      string        `json:"reopener"`
	ReopenerID    string        `json:"reopener_id"`
	Conversation  *Conversation `json:"conversation"`
	Users         User          `json:"users"`
	InteractionID string        `json:"interaction_id"`
}

func handleEventCallback(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketerUUID := assets.TicketerUUID(chi.URLParam(r, "ticketer"))

	ticketer, _, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, typeFreshchat)
	if err != nil {
		return errors.Errorf("no such ticketer %s", ticketerUUID), http.StatusNotFound, nil
	}

	request := &webhookRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return err, http.StatusBadRequest, nil
	}

	// validate action type first
	action := strings.ToLower(request.Action)
	if action != "conversation_resolution" && action != "conversation_reopen" && action != "message_created" {
		return map[string]string{"error": "invalid event type"}, http.StatusBadRequest, nil
	}

	externalID := ""
	switch request.Action {
	case "conversation_resolution":
		if request.Data.Resolve != nil && request.Data.Resolve.Conversation != nil {
			externalID = request.Data.Resolve.Conversation.ConversationID
		}
	case "conversation_reopen":
		if request.Data.Reopen != nil && request.Data.Reopen.Conversation != nil {
			externalID = request.Data.Reopen.Conversation.ConversationID
		}
	case "message_created":
		if request.Data.Message != nil {
			externalID = request.Data.Message.ConversationID
		}
	}

	// lookup ticket
	ticket, err := models.LookupTicketByExternalID(ctx, rt.DB, ticketer.ID(), externalID)
	if err != nil || ticket == nil {
		// we don't return an error here, because ticket might just belong to a different ticketer
		return map[string]string{"status": "ignored"}, http.StatusOK, nil
	}

	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		return err, http.StatusBadRequest, nil
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "error marshalling request"), http.StatusBadRequest, nil
	}

	switch action {
	case "conversation_resolution":
		err = tickets.Close(ctx, rt, oa, ticket, false, l, string(requestJSON))
	case "conversation_reopen":
		err = tickets.Reopen(ctx, rt, oa, ticket, false, l)
	case "message_created":
		if len(request.Data.Message.MessageParts) > 0 && request.Actor.ActorType == "agent" { //only process messages from agents
			var textMsg string
			var files []*tickets.File

			for _, part := range request.Data.Message.MessageParts {
				if part.Text != nil {
					if strings.TrimSpace(part.Text.Content) != "" {
						if textMsg != "" {
							textMsg += "\n"
						}
						textMsg += part.Text.Content
					}
				} else if part.Image != nil {
					file, err := tickets.FetchFileWithMaxSize(part.Image.URL, nil, 100*1024*1024)
					if err != nil {
						return errors.Wrapf(err, "error fetching ticket file '%s'", part.Image.URL), http.StatusBadRequest, nil
					}
					file.ContentType = "image/jpeg" //default content type for images
					files = append(files, file)
				} else if part.Video != nil {
					file, err := tickets.FetchFileWithMaxSize(part.Video.URL, nil, 100*1024*1024)
					if err != nil {
						return errors.Wrapf(err, "error fetching ticket file '%s'", part.Video.URL), http.StatusBadRequest, nil
					}
					file.ContentType = part.Video.ContentType
					files = append(files, file)
				} else if part.File != nil {
					file, err := tickets.FetchFileWithMaxSize(part.File.URL, nil, 100*1024*1024)
					if err != nil {
						return errors.Wrapf(err, "error fetching ticket file '%s'", part.File.URL), http.StatusBadRequest, nil
					}
					file.ContentType = part.File.ContentType
					files = append(files, file)
				}
			}

			if textMsg != "" || len(files) > 0 {
				_, err := tickets.SendReply(ctx, rt, ticket, textMsg, files, nil)
				if err != nil {
					return errors.Wrap(err, "error on send ticket reply"), http.StatusBadRequest, nil
				}
			}
		} else {
			return map[string]string{"status": "ignored"}, http.StatusOK, nil
		}
	}

	if err != nil {
		return err, http.StatusBadRequest, nil
	}

	return map[string]string{"status": "handled"}, http.StatusOK, nil
}

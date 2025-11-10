package freshchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	fmt.Printf("[Freshchat Debug] Webhook received - ticketer: %s, method: %s, url: %s\n", ticketerUUID, r.Method, r.URL.String())

	// Read body for debugging
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	fmt.Printf("[Freshchat Debug] Body: %s\n", string(bodyBytes))

	ticketer, _, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, typeFreshchat)
	if err != nil {
		fmt.Printf("[Freshchat Debug] ERROR: Failed to find ticketer: %v\n", err)
		return errors.Errorf("no such ticketer %s", ticketerUUID), http.StatusNotFound, nil
	}
	fmt.Printf("[Freshchat Debug] Ticketer found - ID: %d\n", ticketer.ID())

	request := &webhookRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		fmt.Printf("[Freshchat Debug] ERROR: Failed to unmarshal request: %v\n", err)
		return err, http.StatusBadRequest, nil
	}
	fmt.Printf("[Freshchat Debug] Request parsed - action: %s, request_id: %s, actor_type: %s\n", request.Action, request.RequestID, request.Actor.ActorType)

	// validate action type first
	action := strings.ToLower(request.Action)
	if action != "conversation_resolution" && action != "conversation_reopen" && action != "message_create" {
		fmt.Printf("[Freshchat Debug] ERROR: Invalid event type: %s\n", action)
		return map[string]string{"error": "invalid event type"}, http.StatusBadRequest, nil
	}

	externalID := ""
	switch request.Action {
	case "conversation_resolution":
		if request.Data.Resolve != nil && request.Data.Resolve.Conversation != nil {
			externalID = request.Data.Resolve.Conversation.ConversationID
		} else {
			fmt.Printf("[Freshchat Debug] WARN: Resolve data or conversation is nil for action: %s\n", request.Action)
		}
	case "conversation_reopen":
		if request.Data.Reopen != nil && request.Data.Reopen.Conversation != nil {
			externalID = request.Data.Reopen.Conversation.ConversationID
		} else {
			fmt.Printf("[Freshchat Debug] WARN: Reopen data or conversation is nil for action: %s\n", request.Action)
		}
	case "message_create":
		if request.Data.Message != nil {
			externalID = request.Data.Message.ConversationID
		} else {
			fmt.Printf("[Freshchat Debug] WARN: Message data is nil for action: %s\n", request.Action)
		}
	}
	fmt.Printf("[Freshchat Debug] Extracted external_id: %s\n", externalID)

	// lookup ticket
	ticket, err := models.LookupTicketByExternalID(ctx, rt.DB, ticketer.ID(), externalID)
	if err != nil || ticket == nil {
		fmt.Printf("[Freshchat Debug] WARN: Ticket not found - ticketer_id: %d, external_id: %s, error: %v\n", ticketer.ID(), externalID, err)
		// we don't return an error here, because ticket might just belong to a different ticketer
		return map[string]string{"status": "ignored"}, http.StatusOK, nil
	}
	fmt.Printf("[Freshchat Debug] Ticket found - ID: %d, UUID: %s\n", ticket.ID(), ticket.UUID())

	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		fmt.Printf("[Freshchat Debug] ERROR: Failed to get org assets - org_id: %d, error: %v\n", ticket.OrgID(), err)
		return err, http.StatusBadRequest, nil
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("[Freshchat Debug] ERROR: Failed to marshal request: %v\n", err)
		return errors.Wrap(err, "error marshalling request"), http.StatusBadRequest, nil
	}

	fmt.Printf("[Freshchat Debug] Processing webhook action: %s\n", action)
	switch action {
	case "conversation_resolution":
		fmt.Printf("[Freshchat Debug] Closing ticket\n")
		err = tickets.Close(ctx, rt, oa, ticket, false, l, string(requestJSON))
		if err != nil {
			fmt.Printf("[Freshchat Debug] ERROR: Failed to close ticket: %v\n", err)
		}
	case "conversation_reopen":
		fmt.Printf("[Freshchat Debug] Reopening ticket\n")
		err = tickets.Reopen(ctx, rt, oa, ticket, false, l)
		if err != nil {
			fmt.Printf("[Freshchat Debug] ERROR: Failed to reopen ticket: %v\n", err)
		}
	case "message_create":
		fmt.Printf("[Freshchat Debug] Processing message_create - message_parts_count: %d, actor_type: %s\n", len(request.Data.Message.MessageParts), request.Actor.ActorType)
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
				fmt.Printf("[Freshchat Debug] Sending reply - text_length: %d, files_count: %d\n", len(textMsg), len(files))
				_, err := tickets.SendReply(ctx, rt, ticket, textMsg, files, nil)
				if err != nil {
					fmt.Printf("[Freshchat Debug] ERROR: Failed to send reply: %v\n", err)
					return errors.Wrap(err, "error on send ticket reply"), http.StatusBadRequest, nil
				}
			} else {
				fmt.Printf("[Freshchat Debug] No text or files to send, ignoring\n")
			}
		} else {
			fmt.Printf("[Freshchat Debug] Ignoring message - message_parts_count: %d, actor_type: %s\n", len(request.Data.Message.MessageParts), request.Actor.ActorType)
			return map[string]string{"status": "ignored"}, http.StatusOK, nil
		}
	}

	if err != nil {
		fmt.Printf("[Freshchat Debug] ERROR: Error processing webhook: %v\n", err)
		return err, http.StatusBadRequest, nil
	}

	fmt.Printf("[Freshchat Debug] Webhook processed successfully\n")
	return map[string]string{"status": "handled"}, http.StatusOK, nil
}

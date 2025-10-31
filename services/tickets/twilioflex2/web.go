package twilioflex2

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/tickets"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	base := "/mr/tickets/types/twilioflex2"
	web.RegisterJSONRoute(http.MethodPost, base+"/interaction_callback/{ticketer:[a-f0-9\\-]+}/{ticket:[a-f0-9\\-]+}", web.WithHTTPLogs(handleInteractionCallback))
	web.RegisterJSONRoute(http.MethodPost, base+"/conversation_callback/{ticketer:[a-f0-9\\-]+}/{ticket:[a-f0-9\\-]+}", web.WithHTTPLogs(handleConversationCallback))
}

// interactionCallbackRequest represents incoming interaction webhook events
type interactionCallbackRequest struct {
	AccountSid      string `json:"AccountSid,omitempty"`
	ChannelSid      string `json:"ChannelSid,omitempty"`
	ChannelStatus   string `json:"ChannelStatus,omitempty"`
	EventType       string `json:"EventType,omitempty"`
	FlexInstanceSid string `json:"FlexInstanceSid,omitempty"`
	InteractionSid  string `json:"InteractionSid,omitempty"`
	MediaChannelSid string `json:"MediaChannelSid,omitempty"`
}

// conversationCallbackRequest represents incoming conversation webhook events
type conversationCallbackRequest struct {
	AccountSid      string      `json:"AccountSid,omitempty"`
	EventType       string      `json:"EventType,omitempty"`
	ConversationSid string      `json:"ConversationSid,omitempty"`
	Author          string      `json:"Author,omitempty"`
	Body            string      `json:"Body,omitempty"`
	ParticipantSid  string      `json:"ParticipantSid,omitempty"`
	MessageSid      string      `json:"MessageSid,omitempty"`
	Media           []mediaData `json:"Media,omitempty"`
	Attributes      string      `json:"Attributes,omitempty"`
	DateCreated     *time.Time  `json:"DateCreated,omitempty"`
	Index           int         `json:"Index,omitempty"`
	Source          string      `json:"Source,omitempty"`
	WebhookSid      string      `json:"WebhookSid,omitempty"`
	ChatServiceSid  string      `json:"ChatServiceSid,omitempty"`
}

// participantData represents participant information in an interaction
type participantData struct {
	Sid      string `json:"Sid,omitempty"`
	Identity string `json:"Identity,omitempty"`
	Type     string `json:"Type,omitempty"`
}

// mediaData represents media attachments in a message
type mediaData struct {
	Sid         string `json:"Sid,omitempty"`
	Size        string `json:"Size,omitempty"`
	ContentType string `json:"ContentType,omitempty"`
	Filename    string `json:"Filename,omitempty"`
}

// handleInteractionCallback processes interaction webhook events
func handleInteractionCallback(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketerUUID := assets.TicketerUUID(chi.URLParam(r, "ticketer"))
	ticketUUID := uuids.UUID(chi.URLParam(r, "ticket"))

	request := &interactionCallbackRequest{}
	if err := web.DecodeAndValidateForm(request, r); err != nil {
		return errors.Wrapf(err, "error decoding form"), http.StatusBadRequest, nil
	}

	ticketer, _, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, "twilioflex2")
	if err != nil {
		return errors.Errorf("no such ticketer %s", ticketerUUID), http.StatusNotFound, nil
	}

	accountSid := ticketer.Config("account_sid")
	if accountSid != request.AccountSid {
		return map[string]string{"status": "unauthorized"}, http.StatusUnauthorized, nil
	}

	ticket, _, _, err := tickets.FromTicketUUID(ctx, rt, flows.TicketUUID(ticketUUID), "twilioflex2")
	if err != nil {
		return errors.Errorf("no such ticket %s", ticketUUID), http.StatusNotFound, nil
	}

	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		return err, http.StatusBadRequest, nil
	}

	// process based on event type
	switch request.EventType {
	case "onChannelStatusUpdated":
		if err := handleChannelStatusChange(ctx, rt, oa, ticket, request); err != nil {
			return err, http.StatusBadRequest, nil
		}
	}

	return map[string]string{"status": "handled"}, http.StatusOK, nil
}

// handleConversationCallback processes conversation webhook events
func handleConversationCallback(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketerUUID := assets.TicketerUUID(chi.URLParam(r, "ticketer"))
	ticketUUID := uuids.UUID(chi.URLParam(r, "ticket"))

	request := &conversationCallbackRequest{}
	if err := web.DecodeAndValidateForm(request, r); err != nil {
		return errors.Wrapf(err, "error decoding form"), http.StatusBadRequest, nil
	}

	ticketer, _, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, "twilioflex2")
	if err != nil {
		return errors.Errorf("no such ticketer %s", ticketerUUID), http.StatusNotFound, nil
	}

	accountSid := ticketer.Config("account_sid")
	if accountSid != request.AccountSid {
		return map[string]string{"status": "unauthorized"}, http.StatusUnauthorized, nil
	}

	ticket, _, _, err := tickets.FromTicketUUID(ctx, rt, flows.TicketUUID(ticketUUID), "twilioflex2")
	if err != nil {
		return errors.Errorf("no such ticket %s", ticketUUID), http.StatusNotFound, nil
	}

	// generate identity to prevent echo messages
	identity := fmt.Sprintf("%d_%s", ticket.ContactID(), ticket.UUID())

	// process based on event type
	switch request.EventType {
	case "onMessageAdded":
		if err := handleMessageAdded(ctx, rt, ticket, ticketer, request, identity); err != nil {
			return err, http.StatusBadRequest, nil
		}
	}

	return map[string]string{"status": "handled"}, http.StatusOK, nil
}

// handleChannelStatusChange processes channel status change events
func handleChannelStatusChange(ctx context.Context, rt *runtime.Runtime, oa *models.OrgAssets, ticket *models.Ticket, request *interactionCallbackRequest) error {
	// Handle channel closure which should close the ticket
	if strings.ToLower(request.ChannelStatus) == "inactive" || strings.ToLower(request.ChannelStatus) == "closed" {
		return tickets.Close(ctx, rt, oa, ticket, false, nil, "")
	}
	return nil
}

// handleMessageAdded processes new message events from conversations
func handleMessageAdded(ctx context.Context, rt *runtime.Runtime, ticket *models.Ticket, ticketer *models.Ticketer, request *conversationCallbackRequest, identity string) error {
	// Prevent echo messages by checking if message is from our own contact
	if request.Author == identity {
		return nil
	}

	if len(request.Media) > 0 {
		for _, media := range request.Media {
			config := ticketer.Config
			authToken := config(configurationAuthToken)
			accountSid := config(configurationAccountSid)
			conversationServiceSid := config(configurationConversationServiceSid)

			client := NewClient(http.DefaultClient, nil, authToken, accountSid)

			mediaContent, _, err := client.FetchMedia(conversationServiceSid, media.Sid)
			if err != nil {
				logrus.WithError(err).Warnf("error fetching media file %s", media.Sid)
				continue
			}
			file, err := tickets.FetchFile(mediaContent.Links.ContentDirectTemporary, nil)
			if err != nil {
				logrus.WithError(err).Warnf("error fetching media file %s", mediaContent.Links.ContentDirectTemporary)
				continue
			}
			file.ContentType = mediaContent.ContentType
			_, err = tickets.SendReply(ctx, rt, ticket, request.Body, []*tickets.File{file}, nil)
			if err != nil {
				logrus.WithError(err).Warnf("error sending reply with media file %s", media.Sid)
				continue
			}
		}
	}

	// Send the reply to the ticket
	_, err := tickets.SendReply(ctx, rt, ticket, request.Body, []*tickets.File{}, nil)
	return err
}

package handlers

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/hooks"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	models.RegisterEventPreWriteHandler(events.TypeMsgWppCreated, handlePreMsgWppCreated)
	models.RegisterEventHandler(events.TypeMsgWppCreated, handleMsgWppCreated)
}

// handlePreMsgWppCreated clears our timeout on our session so that courier can send it when the message is sent, that will be set by courier when sent
func handlePreMsgWppCreated(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.MsgWppCreatedEvent)

	// we only clear timeouts on messaging flows
	if scene.Session().SessionType() != models.FlowTypeMessaging {
		return nil
	}

	// get our channel
	var channel *models.Channel

	if event.Msg.Channel() != nil {
		channel = oa.ChannelByUUID(event.Msg.Channel().UUID)
		if channel == nil {
			return errors.Errorf("unable to load channel with uuid: %s", event.Msg.Channel().UUID)
		}
	}

	// no channel? this is a no-op
	if channel == nil {
		return nil
	}

	// android channels get normal timeouts
	if channel.Type() == models.ChannelTypeAndroid {
		return nil
	}

	// everybody else gets their timeout cleared, will be set by courier
	scene.Session().ClearTimeoutOn()

	return nil
}

// handleMsgWppCreated creates the db msg for the passed in event
func handleMsgWppCreated(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.MsgWppCreatedEvent)

	// must be in a session
	if scene.Session() == nil {
		return errors.Errorf("cannot handle msg created event without session")
	}

	logrus.WithFields(logrus.Fields{
		"contact_uuid": scene.ContactUUID(),
		"session_id":   scene.SessionID(),
		"text":         event.Msg.Text(),
		"urn":          event.Msg.URN(),
	}).Debug("msg created event")

	// messages in messaging flows must have urn id set on them, if not, go look it up
	if scene.Session().SessionType() == models.FlowTypeMessaging && event.Msg.URN() != urns.NilURN {
		urn := event.Msg.URN()
		if models.GetURNInt(urn, "id") == 0 {
			urn, err := models.GetOrCreateURN(ctx, tx, oa, scene.ContactID(), event.Msg.URN())
			if err != nil {
				return errors.Wrapf(err, "unable to get or create URN: %s", event.Msg.URN())
			}
			// update our Msg with our full URN
			event.Msg.SetURN(urn)
		}
	}

	// get our channel
	var channel *models.Channel
	if event.Msg.Channel() != nil {
		channel = oa.ChannelByUUID(event.Msg.Channel().UUID)
		if channel == nil {
			return errors.Errorf("unable to load channel with uuid: %s", event.Msg.Channel().UUID)
		} else {
			if fmt.Sprint(channel.Type()) == "WAC" || fmt.Sprint(channel.Type()) == "WA" {
				country := envs.DeriveCountryFromTel("+" + event.Msg.URN().Path())
				locale := envs.NewLocale(scene.Contact().Language(), country)
				languageCode := locale.ToBCP47()

				if _, valid := validLanguageCodes[languageCode]; !valid {
					languageCode = ""
				}

				event.Msg.TextLanguage = envs.Language(languageCode)
			}
		}
	}

	// get our whatsapp flow status
	if channel.Type() == "WAC" {
		if event.Msg.InteractionType() == "flow_msg" && event.Msg.FlowMessage().FlowID != "" {
			whatsAppFlow, err := models.GetActiveWhatsAppFlowFromFacebookIDAndChannel(ctx, tx, channel.ID(), event.Msg.FlowMessage().FlowID)
			if err != nil {
				return errors.Wrapf(err, "unable to load whatsapp flow with id: %s, for channel %d", event.Msg.FlowMessage().FlowID, channel.ID())
			}

			if whatsAppFlow != nil {
				event.Msg.FlowMessage_.FlowMode = whatsAppFlow.Status()
			}
		}
	}

	msg, err := models.NewOutgoingFlowMsgWpp(rt, oa.Org(), channel, scene.Session(), event.Msg, event.CreatedOn())
	if err != nil {
		return errors.Wrapf(err, "error creating outgoing message to %s", event.Msg.URN())
	}

	// register to have this message committed
	scene.AppendToEventPreCommitHook(hooks.CommitMessagesHook, msg)

	// don't send messages for surveyor flows
	if scene.Session().SessionType() != models.FlowTypeSurveyor {
		scene.AppendToEventPostCommitHook(hooks.SendMessagesHook, msg)
	}

	return nil
}

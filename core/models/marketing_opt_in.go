package models

import (
	"context"
	"strings"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const MarketingOptInFieldKey = "marketing_opt_in"

// IsMarketingTemplateCategory reports whether a template category is MARKETING.
func IsMarketingTemplateCategory(category string) bool {
	return strings.EqualFold(strings.TrimSpace(category), "marketing")
}

// ContactAllowsMarketing returns whether the contact may receive marketing template messages.
// Missing or empty field values are treated as opt-in (allow marketing).
func ContactAllowsMarketing(c *Contact) bool {
	if c == nil {
		return true
	}
	fieldVal, ok := c.Fields()[MarketingOptInFieldKey]
	if !ok || fieldVal == nil {
		return true
	}
	return !isMarketingOptOutValue(fieldVal)
}

func isMarketingOptOutValue(v *flows.Value) bool {
	if v == nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(v.Text.Native()))
	switch s {
	case "false", "no", "0", "n":
		return true
	default:
		return false
	}
}

// applyMarketingOptOutFailure marks the outgoing message as failed when the contact opted out of marketing templates.
// contact may be passed when the caller already loaded it (e.g. WPP broadcast batch); otherwise it is loaded from the DB.
func applyMarketingOptOutFailure(ctx context.Context, rt *runtime.Runtime, org *Org, msg *Msg, category string, contact *Contact) error {
	if msg.m.Status != MsgStatusQueued || category == "" || !IsMarketingTemplateCategory(category) {
		return nil
	}

	c := contact
	if c == nil {
		oa, err := GetOrgAssets(ctx, rt, org.ID())
		if err != nil {
			return errors.Wrap(err, "error loading org assets for marketing opt-in check")
		}

		contacts, err := LoadContactsBasic(ctx, rt.DB, oa, []ContactID{msg.m.ContactID})
		if err != nil {
			return errors.Wrap(err, "error loading contact for marketing opt-in check")
		}
		if len(contacts) == 0 {
			return nil
		}
		c = contacts[0]
	}

	if ContactAllowsMarketing(c) {
		return nil
	}

	msg.m.Status = MsgStatusFailed
	msg.m.FailedReason = MsgFailedMarketingOptOut
	logrus.WithFields(logrus.Fields{
		"org_id":       org.ID(),
		"contact_id":   msg.m.ContactID,
		"broadcast_id": msg.m.BroadcastID,
	}).Info("marketing template not sent due to contact opt-out")
	return nil
}

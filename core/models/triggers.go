package models

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/triggers"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/utils/dbutil"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TriggerType is the type of a trigger
type TriggerType string

// MatchType is used for keyword triggers to specify how they should match
type MatchType string

// TriggerID is the type for trigger database IDs
type TriggerID int

// trigger type constants
const (
	CatchallTriggerType        = TriggerType("C")
	KeywordTriggerType         = TriggerType("K")
	MissedCallTriggerType      = TriggerType("M")
	NewConversationTriggerType = TriggerType("N")
	ReferralTriggerType        = TriggerType("R")
	IncomingCallTriggerType    = TriggerType("V")
	ScheduleTriggerType        = TriggerType("S")
	TicketClosedTriggerType    = TriggerType("T")
)

// match type constants
const (
	MatchFirst MatchType = "F"
	MatchOnly  MatchType = "O"
)

// NilTriggerID is the nil value for trigger IDs
const NilTriggerID = TriggerID(0)

// Trigger represents a trigger in an organization
type Trigger struct {
	t struct {
		ID              TriggerID   `json:"id"`
		FlowID          FlowID      `json:"flow_id"`
		TriggerType     TriggerType `json:"trigger_type"`
		Keyword         string      `json:"keyword"`
		MatchType       MatchType   `json:"match_type"`
		ChannelID       ChannelID   `json:"channel_id"`
		ReferrerID      string      `json:"referrer_id"`
		IncludeGroupIDs []GroupID   `json:"include_group_ids"`
		ExcludeGroupIDs []GroupID   `json:"exclude_group_ids"`
		ContactIDs      []ContactID `json:"contact_ids,omitempty"`
		CreatedOn       time.Time   `json:"created_on"`
	}
}

// ID returns the id of this trigger
func (t *Trigger) ID() TriggerID { return t.t.ID }

func (t *Trigger) FlowID() FlowID             { return t.t.FlowID }
func (t *Trigger) TriggerType() TriggerType   { return t.t.TriggerType }
func (t *Trigger) Keyword() string            { return t.t.Keyword }
func (t *Trigger) MatchType() MatchType       { return t.t.MatchType }
func (t *Trigger) ChannelID() ChannelID       { return t.t.ChannelID }
func (t *Trigger) ReferrerID() string         { return t.t.ReferrerID }
func (t *Trigger) IncludeGroupIDs() []GroupID { return t.t.IncludeGroupIDs }
func (t *Trigger) ExcludeGroupIDs() []GroupID { return t.t.ExcludeGroupIDs }
func (t *Trigger) ContactIDs() []ContactID    { return t.t.ContactIDs }
func (t *Trigger) CreatedOn() time.Time       { return t.t.CreatedOn }
func (t *Trigger) KeywordMatchType() triggers.KeywordMatchType {
	if t.t.MatchType == MatchFirst {
		return triggers.KeywordMatchTypeFirstWord
	}
	return triggers.KeywordMatchTypeOnlyWord
}

// Match returns the match for this trigger, if any
func (t *Trigger) Match() *triggers.KeywordMatch {
	if t.Keyword() != "" {
		return &triggers.KeywordMatch{
			Type:    t.KeywordMatchType(),
			Keyword: t.Keyword(),
		}
	}
	return nil
}

// loadTriggers loads all non-schedule triggers for the passed in org
func loadTriggers(ctx context.Context, db Queryer, orgID OrgID) ([]*Trigger, error) {
	start := time.Now()

	rows, err := db.QueryxContext(ctx, selectTriggersSQL, orgID)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying triggers for org: %d", orgID)
	}
	defer rows.Close()

	triggers := make([]*Trigger, 0, 10)
	for rows.Next() {
		trigger := &Trigger{}
		err = dbutil.ReadJSONRow(rows, &trigger.t)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning label row")
		}

		triggers = append(triggers, trigger)
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("org_id", orgID).WithField("count", len(triggers)).Debug("loaded triggers")

	return triggers, nil
}

// FindMatchingMsgTrigger finds the best match trigger for an incoming message from the given contact
func FindMatchingMsgTrigger(oa *OrgAssets, contact *flows.Contact, text string) *Trigger {
	// determine our message keyword
	words := utils.TokenizeString(text)
	keyword := ""
	only := false
	if len(words) > 0 {
		// our keyword is our first word
		keyword = strings.ToLower(words[0])
		only = len(words) == 1
	}

	candidates := findTriggerCandidates(oa, KeywordTriggerType, func(t *Trigger) bool {
		return t.Keyword() == keyword && (t.MatchType() == MatchFirst || (t.MatchType() == MatchOnly && only))
	})

	// if we have a matching keyword trigger return that, otherwise we move on to catchall triggers..
	byKeyword := findBestTriggerMatch(candidates, nil, contact)
	if byKeyword != nil {
		return byKeyword
	}

	candidates = findTriggerCandidates(oa, CatchallTriggerType, nil)

	return findBestTriggerMatch(candidates, nil, contact)
}

// FindMatchingIncomingCallTrigger finds the best match trigger for incoming calls
func FindMatchingIncomingCallTrigger(oa *OrgAssets, contact *flows.Contact) *Trigger {
	candidates := findTriggerCandidates(oa, IncomingCallTriggerType, nil)

	return findBestTriggerMatch(candidates, nil, contact)
}

// FindMatchingMissedCallTrigger finds the best match trigger for missed incoming calls
func FindMatchingMissedCallTrigger(oa *OrgAssets) *Trigger {
	candidates := findTriggerCandidates(oa, MissedCallTriggerType, nil)

	return findBestTriggerMatch(candidates, nil, nil)
}

// FindMatchingNewConversationTrigger finds the best match trigger for new conversation channel events
func FindMatchingNewConversationTrigger(oa *OrgAssets, channel *Channel) *Trigger {
	candidates := findTriggerCandidates(oa, NewConversationTriggerType, nil)

	return findBestTriggerMatch(candidates, channel, nil)
}

// FindMatchingReferralTrigger finds the best match trigger for referral click channel events
func FindMatchingReferralTrigger(oa *OrgAssets, channel *Channel, referrerID string) *Trigger {
	// first try to find matching referrer ID
	candidates := findTriggerCandidates(oa, ReferralTriggerType, func(t *Trigger) bool {
		return strings.EqualFold(t.ReferrerID(), referrerID)
	})

	match := findBestTriggerMatch(candidates, channel, nil)
	if match != nil {
		return match
	}

	// if that didn't work look for an empty referrer ID
	candidates = findTriggerCandidates(oa, ReferralTriggerType, func(t *Trigger) bool {
		return t.ReferrerID() == ""
	})

	return findBestTriggerMatch(candidates, channel, nil)
}

// FindMatchingTicketClosedTrigger finds the best match trigger for ticket closed events
func FindMatchingTicketClosedTrigger(oa *OrgAssets, contact *flows.Contact) *Trigger {
	candidates := findTriggerCandidates(oa, TicketClosedTriggerType, nil)

	return findBestTriggerMatch(candidates, nil, contact)
}

// finds trigger candidates based on type and optional filter
func findTriggerCandidates(oa *OrgAssets, type_ TriggerType, filter func(*Trigger) bool) []*Trigger {
	candidates := make([]*Trigger, 0, 10)

	for _, t := range oa.Triggers() {
		if t.TriggerType() == type_ && (filter == nil || filter(t)) {
			candidates = append(candidates, t)
		}
	}

	return candidates
}

type triggerMatch struct {
	trigger *Trigger
	score   int
}

// matching triggers are given a score based on how they matched, and this score is used to select the most
// specific match:
//
// channel (4) + include (2) + exclude (1) = 7
// channel (4) + include (2) = 6
// channel (4) + exclude (1) = 5
// channel (4) = 4
// include (2) + exclude (1) = 3
// include (2) = 2
// exclude (1) = 1
const triggerScoreByChannel = 4
const triggerScoreByInclusion = 2
const triggerScoreByExclusion = 1

func findBestTriggerMatch(candidates []*Trigger, channel *Channel, contact *flows.Contact) *Trigger {
	matches := make([]*triggerMatch, 0, len(candidates))

	var groupIDs map[GroupID]bool

	if contact != nil {
		// build a set of the groups this contact is in
		groupIDs = make(map[GroupID]bool, 10)
		for _, g := range contact.Groups().All() {
			groupIDs[g.Asset().(*Group).ID()] = true
		}
	}

	for _, t := range candidates {
		matched, score := triggerMatchQualifiers(t, channel, groupIDs)
		if matched {
			matches = append(matches, &triggerMatch{t, score})
		}
	}

	if len(matches) == 0 {
		return nil
	}

	// sort the matches to get them in descending order of score
	// if scores are equal, use the most recent created_on date
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].trigger.CreatedOn().After(matches[j].trigger.CreatedOn())
		}
		return matches[i].score > matches[j].score
	})

	return matches[0].trigger
}

// matches against the qualifiers (inclusion groups, exclusion groups, channel) on this trigger and returns a score
func triggerMatchQualifiers(t *Trigger, channel *Channel, contactGroups map[GroupID]bool) (bool, int) {
	score := 0

	if channel != nil && t.ChannelID() != NilChannelID {
		if t.ChannelID() == channel.ID() {
			score += triggerScoreByChannel
		} else {
			return false, 0
		}
	}

	if len(t.IncludeGroupIDs()) > 0 {
		inGroup := false
		// if contact is in any of the groups to include that's a match by inclusion
		for _, g := range t.IncludeGroupIDs() {
			if contactGroups[g] {
				inGroup = true
				score += triggerScoreByInclusion
				break
			}
		}
		if !inGroup {
			return false, 0
		}
	}

	if len(t.ExcludeGroupIDs()) > 0 {
		// if contact is in none of the groups to exclude that's a match by exclusion
		for _, g := range t.ExcludeGroupIDs() {
			if contactGroups[g] {
				return false, 0
			}
		}
		score += triggerScoreByExclusion
	}

	return true, score
}

const selectTriggersSQL = `
SELECT ROW_TO_JSON(r) FROM (SELECT
	t.id as id, 
	t.flow_id as flow_id,
	t.trigger_type as trigger_type,
	t.keyword as keyword,
	t.match_type as match_type,
	t.channel_id as channel_id,
	COALESCE(t.referrer_id, '') as referrer_id,
	t.created_on as created_on,
	ARRAY_REMOVE(ARRAY_AGG(DISTINCT ig.contactgroup_id), NULL) as include_group_ids,
	ARRAY_REMOVE(ARRAY_AGG(DISTINCT eg.contactgroup_id), NULL) as exclude_group_ids
FROM 
	triggers_trigger t
	LEFT OUTER JOIN triggers_trigger_groups ig ON t.id = ig.trigger_id
	LEFT OUTER JOIN triggers_trigger_exclude_groups eg ON t.id = eg.trigger_id
WHERE 
	t.org_id = $1 AND 
	t.is_active = TRUE AND
	t.is_archived = FALSE AND
	t.trigger_type != 'S'
GROUP BY 
	t.id,
	t.created_on
) r;
`

const selectTriggersByContactIDsSQL = `
SELECT 
	t.id AS id
FROM
	triggers_trigger t
INNER JOIN 
	triggers_trigger_contacts tc ON tc.trigger_id = t.id
WHERE
	tc.contact_id = ANY($1) AND
	is_archived = FALSE
`

const deleteContactTriggersForIDsSQL = `
DELETE FROM
	triggers_trigger_contacts
WHERE
	contact_id = ANY($1)
`

const archiveEmptyTriggersSQL = `
UPDATE 
	triggers_trigger
SET 
	is_archived = TRUE
WHERE
	id = ANY($1) AND
	NOT EXISTS (SELECT * FROM triggers_trigger_contacts WHERE trigger_id = triggers_trigger.id) AND
	NOT EXISTS (SELECT * FROM triggers_trigger_groups WHERE trigger_id = triggers_trigger.id) AND
	NOT EXISTS (SELECT * FROM triggers_trigger_exclude_groups WHERE trigger_id = triggers_trigger.id)
`

// ArchiveContactTriggers removes the given contacts from any triggers and archives any triggers
// which reference only those contacts
func ArchiveContactTriggers(ctx context.Context, tx Queryer, contactIDs []ContactID) error {
	// start by getting all the active triggers that reference these contacts
	rows, err := tx.QueryxContext(ctx, selectTriggersByContactIDsSQL, pq.Array(contactIDs))
	if err != nil {
		return errors.Wrapf(err, "error finding triggers for contacts")
	}
	defer rows.Close()

	triggerIDs := make([]TriggerID, 0)
	for rows.Next() {
		var triggerID TriggerID
		err := rows.Scan(&triggerID)
		if err != nil {
			return errors.Wrapf(err, "error reading trigger ID")
		}
		triggerIDs = append(triggerIDs, triggerID)
	}

	// remove any references to these contacts in triggers
	_, err = tx.ExecContext(ctx, deleteContactTriggersForIDsSQL, pq.Array(contactIDs))
	if err != nil {
		return errors.Wrapf(err, "error removing contacts from triggers")
	}

	// archive any of the original triggers which are now not referencing any contact or group
	_, err = tx.ExecContext(ctx, archiveEmptyTriggersSQL, pq.Array(triggerIDs))
	if err != nil {
		return errors.Wrapf(err, "error archiving empty triggers")
	}

	return nil
}

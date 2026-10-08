package models

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/pkg/errors"
)

const (
	ProtocolOpen   = "open"
	ProtocolClosed = "closed"

	ProtocolCloseAICSAT          = "ai_csat"
	ProtocolCloseAIInactivity    = "ai_inactivity"
	ProtocolCloseHumanInactivity = "human_inactivity"
	ProtocolCloseAttendant       = "attendant"
	ProtocolCloseTicket          = "ticket_closed"

	ProtocolTimerAI    = "ai"
	ProtocolTimerHuman = "human"

	defaultAIInactivityHours    = 1
	defaultHumanInactivityHours = 96
)

type turnProtocolKey struct{}

// WithTurnProtocolID stores the inbound protocol on the context that builds flow replies.
func WithTurnProtocolID(ctx context.Context, id int64) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, turnProtocolKey{}, id)
}

// TurnProtocolIDFromContext returns the protocol bound to the current inbound turn.
func TurnProtocolIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(turnProtocolKey{}).(int64)
	return id
}

// ResolveInput is one inbound message asking which protocol it belongs to.
type ResolveInput struct {
	OrgID      OrgID
	URNID      URNID
	ContactID  ContactID
	ExternalID string
	ProtocolID int64
}

// ResolveOutput is the protocol a message must be stored on.
type ResolveOutput struct {
	ProtocolID    int64
	Created       bool
	PredecessorID *int64
}

type protocolHit struct {
	ID            int64         `db:"id"`
	State         string        `db:"state"`
	PredecessorID sql.NullInt64 `db:"predecessor_id"`
}

// ResolveProtocol returns the open protocol for a URN, or opens a follow-up.
// The same external message id returns the protocol already bound to it.
func ResolveProtocol(ctx context.Context, db QueryerWithTx, in ResolveInput) (ResolveOutput, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return ResolveOutput{}, errors.Wrap(err, "error starting protocol resolve")
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT id FROM contacts_contacturn WHERE id = $1 AND org_id = $2 FOR UPDATE`, in.URNID, in.OrgID); err != nil {
		return ResolveOutput{}, errors.Wrap(err, "error locking channel identity")
	}

	if in.ExternalID != "" {
		hit, err := findProtocol(ctx, tx, `SELECT id, state, predecessor_id FROM msgs_protocol WHERE org_id = $1 AND urn_id = $2 AND external_id = $3`, in.OrgID, in.URNID, in.ExternalID)
		if err != nil {
			return ResolveOutput{}, err
		}
		if hit != nil {
			if err := tx.Commit(); err != nil {
				return ResolveOutput{}, err
			}
			return hit.output(false), nil
		}
	}

	var predecessor *int64
	if in.ProtocolID != 0 {
		hit, err := findProtocol(ctx, tx, `SELECT id, state, predecessor_id FROM msgs_protocol WHERE id = $1 AND org_id = $2`, in.ProtocolID, in.OrgID)
		if err != nil {
			return ResolveOutput{}, err
		}
		if hit != nil && hit.State == ProtocolOpen {
			if err := tx.Commit(); err != nil {
				return ResolveOutput{}, err
			}
			return hit.output(false), nil
		}
		if hit != nil && hit.State == ProtocolClosed {
			predecessor = &hit.ID
		}
	}

	openHit, err := findProtocol(ctx, tx, `SELECT id, state, predecessor_id FROM msgs_protocol WHERE org_id = $1 AND urn_id = $2 AND state = 'open' ORDER BY id DESC LIMIT 1`, in.OrgID, in.URNID)
	if err != nil {
		return ResolveOutput{}, err
	}
	if openHit != nil {
		if err := tx.Commit(); err != nil {
			return ResolveOutput{}, err
		}
		return openHit.output(false), nil
	}

	if predecessor == nil {
		closedHit, err := findProtocol(ctx, tx, `SELECT id, state, predecessor_id FROM msgs_protocol WHERE org_id = $1 AND urn_id = $2 AND state = 'closed' ORDER BY id DESC LIMIT 1`, in.OrgID, in.URNID)
		if err != nil {
			return ResolveOutput{}, err
		}
		if closedHit != nil {
			predecessor = &closedHit.ID
		}
	}

	hours := inactivityHours(ctx, tx, in.OrgID, "ai_inactivity_hours", defaultAIInactivityHours, 1, 24)
	var pred interface{}
	if predecessor != nil {
		pred = *predecessor
	}
	var external interface{}
	if in.ExternalID != "" {
		external = in.ExternalID
	}
	var id int64
	err = tx.GetContext(ctx, &id, `
INSERT INTO msgs_protocol (
	uuid, org_id, contact_id, urn_id, state, opened_on, external_id,
	predecessor_id, idle_accumulated, timer_paused, timer_kind, timer_deadline
) VALUES (
	$1, $2, $3, $4, 'open', NOW(), $5,
	$6, 0, false, 'ai', NOW() + make_interval(hours => $7)
) RETURNING id`,
		string(uuids.New()), in.OrgID, in.ContactID, in.URNID, external, pred, hours,
	)
	if err != nil {
		if isUniqueViolation(err) && in.ExternalID != "" {
			hit, findErr := findProtocol(ctx, tx, `SELECT id, state, predecessor_id FROM msgs_protocol WHERE org_id = $1 AND urn_id = $2 AND external_id = $3`, in.OrgID, in.URNID, in.ExternalID)
			if findErr != nil {
				return ResolveOutput{}, findErr
			}
			if hit != nil {
				if err := tx.Commit(); err != nil {
					return ResolveOutput{}, err
				}
				return hit.output(false), nil
			}
		}
		return ResolveOutput{}, errors.Wrap(err, "error opening protocol")
	}
	if err := tx.Commit(); err != nil {
		return ResolveOutput{}, err
	}
	return ResolveOutput{ProtocolID: id, Created: true, PredecessorID: predecessor}, nil
}

// EnsureOpenProtocol opens a protocol when an inbound message arrived without one.
func EnsureOpenProtocol(ctx context.Context, db QueryerWithTx, orgID OrgID, contactID ContactID, urnID URNID) (int64, error) {
	out, err := ResolveProtocol(ctx, db, ResolveInput{OrgID: orgID, ContactID: contactID, URNID: urnID})
	if err != nil {
		return 0, err
	}
	return out.ProtocolID, nil
}

// ProtocolState returns open or closed. An unknown id returns an empty state.
func ProtocolState(ctx context.Context, db Queryer, protocolID int64) (string, error) {
	var state string
	err := db.GetContext(ctx, &state, `SELECT state FROM msgs_protocol WHERE id = $1`, protocolID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return state, err
}

// BindMessageProtocol stores protocol_id on a message that was inserted without the column.
func BindMessageProtocol(ctx context.Context, db Queryer, msgID int64, protocolID int64) error {
	if protocolID == 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `UPDATE msgs_msg SET protocol_id = $2 WHERE id = $1`, msgID, protocolID)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// CloseProtocol closes one open protocol. Closing it again is a no-op.
func CloseProtocol(ctx context.Context, db Queryer, protocolID int64, reason string) error {
	_, err := db.ExecContext(ctx, `
UPDATE msgs_protocol
SET state = 'closed', closed_on = NOW(), close_reason = $2, timer_deadline = NULL, timer_paused = false
WHERE id = $1 AND state = 'open'`, protocolID, reason)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// CloseOpenProtocolsForContact closes every open protocol of a contact once.
func CloseOpenProtocolsForContact(ctx context.Context, db Queryer, contactID ContactID, reason string) error {
	_, err := db.ExecContext(ctx, `
UPDATE msgs_protocol
SET state = 'closed', closed_on = NOW(), close_reason = $2, timer_deadline = NULL, timer_paused = false
WHERE contact_id = $1 AND state = 'open'`, contactID, reason)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// CloseAIProtocol closes an AI-phase protocol when CSAT is answered.
func CloseAIProtocol(ctx context.Context, db Queryer, orgID OrgID, protocolID int64) error {
	_, err := db.ExecContext(ctx, `
UPDATE msgs_protocol
SET state = 'closed', closed_on = NOW(), close_reason = $3, timer_deadline = NULL, timer_paused = false
WHERE id = $1 AND org_id = $2 AND state = 'open' AND (timer_kind IS NULL OR timer_kind = 'ai')`,
		protocolID, orgID, ProtocolCloseAICSAT)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// StartHumanTimer replaces the AI timer when a human room is opened.
func StartHumanTimer(ctx context.Context, db Queryer, contactUUID string) error {
	var hours int
	err := db.GetContext(ctx, &hours, `
SELECT COALESCE(NULLIF(COALESCE(o.config, '{}')::json->>'human_inactivity_hours', '')::int, $2)
FROM contacts_contact c
JOIN orgs_org o ON o.id = c.org_id
WHERE c.uuid = $1`, contactUUID, defaultHumanInactivityHours)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	if err != nil {
		return err
	}
	hours = clamp(hours, 1, 672, defaultHumanInactivityHours)
	_, err = db.ExecContext(ctx, `
UPDATE msgs_protocol p
SET timer_kind = 'human', timer_paused = false, idle_accumulated = 0,
    timer_deadline = NOW() + make_interval(hours => $2)
FROM contacts_contact c
WHERE p.contact_id = c.id AND c.uuid = $1 AND p.state = 'open'`, contactUUID, hours)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// PauseProtocol freezes the human timer and keeps the idle time already spent.
func PauseProtocol(ctx context.Context, db Queryer, orgID OrgID, protocolID int64) error {
	return shiftProtocolTimer(ctx, db, orgID, protocolID, true)
}

// ResumeProtocol continues the human timer from the idle time accumulated before the pause.
func ResumeProtocol(ctx context.Context, db Queryer, orgID OrgID, protocolID int64) error {
	return shiftProtocolTimer(ctx, db, orgID, protocolID, false)
}

func shiftProtocolTimer(ctx context.Context, db Queryer, orgID OrgID, protocolID int64, pause bool) error {
	var row struct {
		TimerDeadline   *time.Time `db:"timer_deadline"`
		IdleAccumulated int        `db:"idle_accumulated"`
		TimerPaused     bool       `db:"timer_paused"`
		State           string     `db:"state"`
		TimerKind       *string    `db:"timer_kind"`
	}
	err := db.GetContext(ctx, &row, `
SELECT timer_deadline, idle_accumulated, timer_paused, state, timer_kind
FROM msgs_protocol WHERE id = $1 AND org_id = $2`, protocolID, orgID)
	if err == sql.ErrNoRows || IsMissingProtocolSchema(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.State != ProtocolOpen {
		return nil
	}
	limit := time.Duration(inactivityHours(ctx, db, orgID, "human_inactivity_hours", defaultHumanInactivityHours, 1, 672)) * time.Hour
	accumulated := time.Duration(row.IdleAccumulated) * time.Second
	now := time.Now()
	if pause {
		if row.TimerPaused || row.TimerDeadline == nil {
			return nil
		}
		accumulated = ApplyPause(now, *row.TimerDeadline, limit)
		_, err = db.ExecContext(ctx, `
UPDATE msgs_protocol
SET timer_paused = true, timer_deadline = NULL, idle_accumulated = $2
WHERE id = $1 AND state = 'open' AND timer_paused = false`, protocolID, int(accumulated/time.Second))
		return err
	}
	if !row.TimerPaused {
		return nil
	}
	deadline := ApplyResume(now, accumulated, limit)
	_, err = db.ExecContext(ctx, `
UPDATE msgs_protocol
SET timer_paused = false, timer_deadline = $2
WHERE id = $1 AND state = 'open' AND timer_paused = true`, protocolID, deadline)
	return err
}

// ApplyPause converts a future deadline into idle seconds already spent inside the limit.
func ApplyPause(now, deadline time.Time, limit time.Duration) time.Duration {
	remaining := deadline.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	if remaining > limit {
		remaining = limit
	}
	spent := limit - remaining
	if spent < 0 {
		return 0
	}
	return spent
}

// ApplyResume schedules the deadline for the idle time that is still left.
func ApplyResume(now time.Time, accumulated, limit time.Duration) time.Time {
	remaining := limit - accumulated
	if remaining < 0 {
		remaining = 0
	}
	return now.Add(remaining)
}

// ExpireDueProtocols closes protocols whose deadline has passed. It does not close paused timers.
func ExpireDueProtocols(ctx context.Context, db Queryer) error {
	_, err := db.ExecContext(ctx, `
UPDATE msgs_protocol
SET state = 'closed',
    closed_on = NOW(),
    close_reason = CASE WHEN timer_kind = 'human' THEN 'human_inactivity' ELSE 'ai_inactivity' END,
    timer_deadline = NULL
WHERE state = 'open' AND timer_paused = false AND timer_deadline IS NOT NULL AND timer_deadline <= NOW()`)
	if IsMissingProtocolSchema(err) {
		return nil
	}
	return err
}

// IsMissingProtocolSchema reports that the shared database does not have the protocol objects yet.
func IsMissingProtocolSchema(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && (string(pqErr.Code) == "42P01" || string(pqErr.Code) == "42703") {
		return true
	}
	return false
}

func (h protocolHit) output(created bool) ResolveOutput {
	out := ResolveOutput{ProtocolID: h.ID, Created: created}
	if h.PredecessorID.Valid {
		id := h.PredecessorID.Int64
		out.PredecessorID = &id
	}
	return out
}

func findProtocol(ctx context.Context, db Queryer, query string, args ...interface{}) (*protocolHit, error) {
	var hit protocolHit
	err := db.GetContext(ctx, &hit, query, args...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &hit, nil
}

func inactivityHours(ctx context.Context, db Queryer, orgID OrgID, key string, def, min, max int) int {
	var raw sql.NullString
	err := db.GetContext(ctx, &raw, `SELECT NULLIF(COALESCE(config, '{}')::json->>$2, '') FROM orgs_org WHERE id = $1`, orgID, key)
	if err != nil || !raw.Valid {
		return def
	}
	hours, err := strconv.Atoi(raw.String)
	if err != nil {
		return def
	}
	return clamp(hours, min, max, def)
}

func clamp(value, min, max, def int) int {
	if value < min || value > max {
		return def
	}
	return value
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && string(pqErr.Code) == "23505"
}

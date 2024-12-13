package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/null"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

type FlowRunID int64

const NilFlowRunID = FlowRunID(0)

type RunStatus string

const (
	RunStatusActive      = "A"
	RunStatusWaiting     = "W"
	RunStatusCompleted   = "C"
	RunStatusExpired     = "X"
	RunStatusInterrupted = "I"
	RunStatusFailed      = "F"
)

var runStatusMap = map[flows.RunStatus]RunStatus{
	flows.RunStatusActive:    RunStatusActive,
	flows.RunStatusWaiting:   RunStatusWaiting,
	flows.RunStatusCompleted: RunStatusCompleted,
	flows.RunStatusExpired:   RunStatusExpired,
	flows.RunStatusFailed:    RunStatusFailed,
}

type ExitType = null.String

var (
	ExitInterrupted = ExitType("I")
	ExitCompleted   = ExitType("C")
	ExitExpired     = ExitType("E")
	ExitFailed      = ExitType("F")
)

var exitToSessionStatusMap = map[ExitType]SessionStatus{
	ExitInterrupted: SessionStatusInterrupted,
	ExitCompleted:   SessionStatusCompleted,
	ExitExpired:     SessionStatusExpired,
	ExitFailed:      SessionStatusFailed,
}

var exitToRunStatusMap = map[ExitType]RunStatus{
	ExitInterrupted: RunStatusInterrupted,
	ExitCompleted:   RunStatusCompleted,
	ExitExpired:     RunStatusExpired,
	ExitFailed:      RunStatusFailed,
}

var keptEvents = map[string]bool{
	events.TypeMsgCreated:        true,
	events.TypeMsgCatalogCreated: true,
	events.TypeMsgReceived:       true,
}

// FlowRun is the mailroom type for a FlowRun
type FlowRun struct {
	r struct {
		ID         FlowRunID     `db:"id"`
		UUID       flows.RunUUID `db:"uuid"`
		Status     RunStatus     `db:"status"`
		IsActive   bool          `db:"is_active"`
		CreatedOn  time.Time     `db:"created_on"`
		ModifiedOn time.Time     `db:"modified_on"`
		ExitedOn   *time.Time    `db:"exited_on"`
		ExitType   ExitType      `db:"exit_type"`
		ExpiresOn  *time.Time    `db:"expires_on"`
		Responded  bool          `db:"responded"`

		// TODO: should this be a complex object that can read / write iself to the DB as JSON?
		Results string `db:"results"`

		// TODO: should this be a complex object that can read / write iself to the DB as JSON?
		Path string `db:"path"`

		// TODO: should this be a complex object that can read / write iself to the DB as JSON?
		Events string `db:"events"`

		CurrentNodeUUID null.String     `db:"current_node_uuid"`
		ContactID       flows.ContactID `db:"contact_id"`
		FlowID          FlowID          `db:"flow_id"`
		OrgID           OrgID           `db:"org_id"`
		ParentUUID      *flows.RunUUID  `db:"parent_uuid"`
		SessionID       SessionID       `db:"session_id"`
		StartID         StartID         `db:"start_id"`
		ConnectionID    *ConnectionID   `db:"connection_id"`
	}

	// we keep a reference to model run as well
	run flows.FlowRun
}

func (r *FlowRun) SetSessionID(sessionID SessionID)     { r.r.SessionID = sessionID }
func (r *FlowRun) SetConnectionID(connID *ConnectionID) { r.r.ConnectionID = connID }
func (r *FlowRun) SetStartID(startID StartID)           { r.r.StartID = startID }
func (r *FlowRun) UUID() flows.RunUUID                  { return r.r.UUID }
func (r *FlowRun) ModifiedOn() time.Time                { return r.r.ModifiedOn }

// MarshalJSON is our custom marshaller so that our inner struct get output
func (r *FlowRun) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.r)
}

// UnmarshalJSON is our custom marshaller so that our inner struct get output
func (r *FlowRun) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &r.r)
}

// Step represents a single step in a run, this struct is used for serialization to the steps
type Step struct {
	UUID      flows.StepUUID `json:"uuid"`
	NodeUUID  flows.NodeUUID `json:"node_uuid"`
	ArrivedOn time.Time      `json:"arrived_on"`
	ExitUUID  flows.ExitUUID `json:"exit_uuid,omitempty"`
}

// newRun writes the passed in flow run to our database, also applying any events in those runs as
// appropriate. (IE, writing db messages etc..)
func newRun(ctx context.Context, tx *sqlx.Tx, org *OrgAssets, session *Session, fr flows.FlowRun) (*FlowRun, error) {
	// build our path elements
	path := make([]Step, len(fr.Path()))
	for i, p := range fr.Path() {
		path[i].UUID = p.UUID()
		path[i].NodeUUID = p.NodeUUID()
		path[i].ArrivedOn = p.ArrivedOn()
		path[i].ExitUUID = p.ExitUUID()
	}
	pathJSON, err := json.Marshal(path)
	if err != nil {
		return nil, err
	}

	flowID, err := FlowIDForUUID(ctx, tx, org, fr.FlowReference().UUID)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load flow with uuid: %s", fr.FlowReference().UUID)
	}

	// create our run
	run := &FlowRun{}
	r := &run.r
	r.UUID = fr.UUID()
	r.Status = runStatusMap[fr.Status()]
	r.CreatedOn = fr.CreatedOn()
	r.ExitedOn = fr.ExitedOn()
	r.ExpiresOn = fr.ExpiresOn()
	r.ModifiedOn = fr.ModifiedOn()
	r.ContactID = fr.Contact().ID()
	r.FlowID = flowID
	r.SessionID = session.ID()
	r.StartID = NilStartID
	r.OrgID = org.OrgID()
	r.Path = string(pathJSON)
	if len(path) > 0 {
		r.CurrentNodeUUID = null.String(path[len(path)-1].NodeUUID)
	}
	run.run = fr

	// set our exit type if we exited
	// TODO: audit exit types
	if fr.Status() != flows.RunStatusActive && fr.Status() != flows.RunStatusWaiting {
		if fr.Status() == flows.RunStatusFailed {
			r.ExitType = ExitInterrupted
		} else {
			r.ExitType = ExitCompleted
		}
		r.IsActive = false
	} else {
		r.IsActive = true
	}

	// we filter which events we write to our events json right now
	filteredEvents := make([]flows.Event, 0)
	for _, e := range fr.Events() {
		if keptEvents[e.Type()] {
			filteredEvents = append(filteredEvents, e)
		}

		// mark ourselves as responded if we received a message
		if e.Type() == events.TypeMsgReceived {
			r.Responded = true
		}
	}
	eventJSON, err := json.Marshal(filteredEvents)
	if err != nil {
		return nil, errors.Wrapf(err, "error marshalling events for run: %s", run.UUID())
	}
	r.Events = string(eventJSON)

	// write our results out
	resultsJSON, err := json.Marshal(fr.Results())
	if err != nil {
		return nil, errors.Wrapf(err, "error marshalling results for run: %s", run.UUID())
	}
	r.Results = string(resultsJSON)

	// set our parent UUID if we have a parent
	if fr.Parent() != nil {
		uuid := fr.Parent().UUID()
		r.ParentUUID = &uuid
	}

	return run, nil
}

// FindFlowStartedOverlap returns the list of contact ids which overlap with those passed in and which
// have been in the flow passed in.
func FindFlowStartedOverlap(ctx context.Context, db *sqlx.DB, flowID FlowID, contacts []ContactID) ([]ContactID, error) {
	var overlap []ContactID
	err := db.SelectContext(ctx, &overlap, flowStartedOverlapSQL, pq.Array(contacts), flowID)
	return overlap, err
}

// TODO: no perfect index, will probably use contact index flows_flowrun_contact_id_985792a9
// could be slow in the cases of contacts having many distinct runs
const flowStartedOverlapSQL = `
SELECT
	DISTINCT(contact_id)
FROM
	flows_flowrun
WHERE
	contact_id = ANY($1) AND
	flow_id = $2
`

// RunExpiration looks up the run expiration for the passed in run, can return nil if the run is no longer active
func RunExpiration(ctx context.Context, db *sqlx.DB, runID FlowRunID) (*time.Time, error) {
	var expiration time.Time
	err := db.Get(&expiration, `SELECT expires_on FROM flows_flowrun WHERE id = $1 AND is_active = TRUE`, runID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "unable to select expiration for run: %d", runID)
	}
	return &expiration, nil
}

// InterruptContactRuns interrupts all runs and sesions that exist for the passed in list of contacts
func InterruptContactRuns(ctx context.Context, tx Queryer, sessionType FlowType, contactIDs []flows.ContactID, now time.Time) error {
	if len(contactIDs) == 0 {
		return nil
	}

	// first interrupt our runs
	err := Exec(ctx, "interrupting contact runs", tx, interruptContactRunsSQL, sessionType, pq.Array(contactIDs), now)
	if err != nil {
		return err
	}

	err = Exec(ctx, "interrupting contact sessions", tx, interruptContactSessionsSQL, sessionType, pq.Array(contactIDs), now)
	if err != nil {
		return err
	}

	return nil
}

const interruptContactRunsSQL = `
UPDATE
	flows_flowrun
SET
	is_active = FALSE,
	exited_on = $3,
	exit_type = 'I',
	status = 'I',
	modified_on = NOW()
WHERE
	id = ANY (
		SELECT 
		  fr.id 
		FROM 
		  flows_flowrun fr
		  JOIN flows_flow ff ON fr.flow_id = ff.id
		WHERE 
		  fr.contact_id = ANY($2) AND 
		  fr.is_active = TRUE AND
		  ff.flow_type = $1
		)
`

const interruptContactSessionsSQL = `
UPDATE
	flows_flowsession
SET
	status = 'I',
	ended_on = $3
WHERE
	id = ANY (SELECT id FROM flows_flowsession WHERE session_type = $1 AND contact_id = ANY($2) AND status = 'W')
`

// ExpireRunsAndSessions expires all the passed in runs and sessions. Note this should only be called
// for runs that have no parents or no way of continuing
func ExpireRunsAndSessions(ctx context.Context, db *sqlx.DB, runIDs []FlowRunID, sessionIDs []SessionID) error {
	if len(runIDs) == 0 {
		return nil
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "error starting transaction to expire sessions")
	}

	err = Exec(ctx, "expiring runs", tx, expireRunsSQL, pq.Array(runIDs))
	if err != nil {
		tx.Rollback()
		return errors.Wrapf(err, "error expiring runs")
	}

	if len(sessionIDs) > 0 {
		err = Exec(ctx, "expiring sessions", tx, expireSessionsSQL, pq.Array(sessionIDs))
		if err != nil {
			tx.Rollback()
			return errors.Wrapf(err, "error expiring sessions")
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrapf(err, "error committing expiration of runs and sessions")
	}
	return nil
}

const insertRunSQL = `
INSERT INTO
flows_flowrun(uuid, is_active, created_on, modified_on, exited_on, exit_type, status, expires_on, responded, results, path, 
	          events, current_node_uuid, contact_id, flow_id, org_id, session_id, start_id, parent_uuid, connection_id)
	   VALUES(:uuid, :is_active, :created_on, NOW(), :exited_on, :exit_type, :status, :expires_on, :responded, :results, :path,
	          :events, :current_node_uuid, :contact_id, :flow_id, :org_id, :session_id, :start_id, :parent_uuid, :connection_id)
RETURNING id
`

const expireSessionsSQL = `
	UPDATE
		flows_flowsession s
	SET
		timeout_on = NULL,
		ended_on = NOW(),
		status = 'X'
	WHERE
		id = ANY($1)
`

const expireRunsSQL = `
	UPDATE
		flows_flowrun fr
	SET
		is_active = FALSE,
		exited_on = NOW(),
		exit_type = 'E',
		status = 'E',
		modified_on = NOW()
	WHERE
		id = ANY($1)
`

func SelectRunUUIDsBySessionIDs(ctx context.Context, db *sqlx.DB, sessionIDs []SessionID) ([]string, error) {
	var flowrunUUIDs []string
	err := db.SelectContext(ctx, &flowrunUUIDs, selectRunsBySessionIDs, pq.Array(sessionIDs))
	if err != nil {
		return nil, errors.Wrapf(err, "error selecting flow run uuids by session ids")
	}
	return flowrunUUIDs, nil
}

func SelectRunUUIDsByContactsIDs(ctx context.Context, db *sqlx.DB, flowType FlowType, contactsIDs []ContactID) ([]string, error) {
	var flowrunUUIDs []string
	err := db.SelectContext(ctx, &flowrunUUIDs, selectRunsByContacts, flowType, pq.Array(contactsIDs))
	if err != nil {
		return nil, errors.Wrapf(err, "error selecting flow run uuids by contact ids")
	}
	return flowrunUUIDs, nil
}

func SelectRunUUIDsByIDs(ctx context.Context, db *sqlx.DB, runIDs []FlowRunID) ([]string, error) {
	var flowrunUUIDs []string
	err := db.SelectContext(ctx, &flowrunUUIDs, selectRunUUIDsByIDs, pq.Array(runIDs))
	if err != nil {
		return nil, errors.Wrapf(err, "error selecting flow run uuids by flow run ids")
	}
	return flowrunUUIDs, nil
}

const selectRunsBySessionIDs = `
SELECT
	fr.uuid
FROM 
	flows_flowrun fr
WHERE
	session_id = ANY($1) AND is_active = TRUE
`

const selectRunsByContacts = `
SELECT 
	fr.uuid 
FROM
	flows_flowrun fr
	JOIN flows_flow ff ON fr.flow_id = ff.id
WHERE 
	fr.contact_id = ANY($2) AND 
	fr.is_active = TRUE AND
	ff.flow_type = $1
`

const selectRunUUIDsByIDs = `
SELECT
	fr.uuid
FROM
	flows_flowrun fr
WHERE
	fr.id = ANY($1)
`

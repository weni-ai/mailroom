package models

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nyaruka/gocommon/storage"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SessionCommitHook func(context.Context, *sqlx.Tx, *redis.Pool, *OrgAssets, []*Session) error

type SessionID int64
type SessionStatus string

const (
	SessionStatusWaiting     = "W"
	SessionStatusCompleted   = "C"
	SessionStatusExpired     = "X"
	SessionStatusInterrupted = "I"
	SessionStatusFailed      = "F"
)

var sessionStatusMap = map[flows.SessionStatus]SessionStatus{
	flows.SessionStatusWaiting:   SessionStatusWaiting,
	flows.SessionStatusCompleted: SessionStatusCompleted,
	flows.SessionStatusFailed:    SessionStatusFailed,
}

// Session is the mailroom type for a FlowSession
type Session struct {
	s struct {
		ID            SessionID         `db:"id"`
		UUID          flows.SessionUUID `db:"uuid"`
		SessionType   FlowType          `db:"session_type"`
		Status        SessionStatus     `db:"status"`
		Responded     bool              `db:"responded"`
		Output        null.String       `db:"output"`
		OutputURL     null.String       `db:"output_url"`
		ContactID     ContactID         `db:"contact_id"`
		OrgID         OrgID             `db:"org_id"`
		CreatedOn     time.Time         `db:"created_on"`
		EndedOn       *time.Time        `db:"ended_on"`
		TimeoutOn     *time.Time        `db:"timeout_on"`
		WaitStartedOn *time.Time        `db:"wait_started_on"`
		CurrentFlowID FlowID            `db:"current_flow_id"`
		ConnectionID  *ConnectionID     `db:"connection_id"`
	}

	incomingMsgID      MsgID
	incomingExternalID null.String

	// any channel connection associated with this flow session
	channelConnection *ChannelConnection

	// time after our last message is sent that we should timeout
	timeout *time.Duration

	contact *flows.Contact
	runs    []*FlowRun

	seenRuns map[flows.RunUUID]time.Time

	// we keep around a reference to the sprint associated with this session
	sprint flows.Sprint

	// the scene for our event hooks
	scene *Scene

	// we also keep around a reference to the wait (if any)
	wait flows.ActivatedWait

	findStep func(flows.StepUUID) (flows.FlowRun, flows.Step)
}

func (s *Session) ID() SessionID                      { return s.s.ID }
func (s *Session) UUID() flows.SessionUUID            { return flows.SessionUUID(s.s.UUID) }
func (s *Session) SessionType() FlowType              { return s.s.SessionType }
func (s *Session) Status() SessionStatus              { return s.s.Status }
func (s *Session) Responded() bool                    { return s.s.Responded }
func (s *Session) Output() string                     { return string(s.s.Output) }
func (s *Session) OutputURL() string                  { return string(s.s.OutputURL) }
func (s *Session) ContactID() ContactID               { return s.s.ContactID }
func (s *Session) OrgID() OrgID                       { return s.s.OrgID }
func (s *Session) CreatedOn() time.Time               { return s.s.CreatedOn }
func (s *Session) EndedOn() *time.Time                { return s.s.EndedOn }
func (s *Session) TimeoutOn() *time.Time              { return s.s.TimeoutOn }
func (s *Session) ClearTimeoutOn()                    { s.s.TimeoutOn = nil }
func (s *Session) WaitStartedOn() *time.Time          { return s.s.WaitStartedOn }
func (s *Session) CurrentFlowID() FlowID              { return s.s.CurrentFlowID }
func (s *Session) ConnectionID() *ConnectionID        { return s.s.ConnectionID }
func (s *Session) IncomingMsgID() MsgID               { return s.incomingMsgID }
func (s *Session) IncomingMsgExternalID() null.String { return s.incomingExternalID }
func (s *Session) Scene() *Scene                      { return s.scene }

// WriteSessionsToStorage writes the outputs of the passed in sessions to our storage (S3), updating the
// output_url for each on success. Failure of any will cause all to fail.
func WriteSessionOutputsToStorage(ctx context.Context, rt *runtime.Runtime, sessions []*Session) error {
	start := time.Now()

	uploads := make([]*storage.Upload, len(sessions))
	for i, s := range sessions {
		uploads[i] = &storage.Upload{
			Path:        s.StoragePath(rt.Config),
			Body:        []byte(s.Output()),
			ContentType: "application/json",
			ACL:         s3.ObjectCannedACLPrivate,
		}
	}

	err := rt.SessionStorage.BatchPut(ctx, uploads)
	if err != nil {
		return errors.Wrapf(err, "error writing sessions to storage")
	}

	for i, s := range sessions {
		s.s.OutputURL = null.String(uploads[i].URL)
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("count", len(sessions)).Debug("wrote sessions to s3")

	return nil
}

const storageTSFormat = "20060102T150405.999Z"

// StoragePath returns the path for the session
func (s *Session) StoragePath(cfg *runtime.Config) string {
	ts := s.CreatedOn().UTC().Format(storageTSFormat)

	// example output: /orgs/1/c/20a5/20a5534c-b2ad-4f18-973a-f1aa3b4e6c74/session_20060102T150405.123Z_8a7fc501-177b-4567-a0aa-81c48e6de1c5_51df83ac21d3cf136d8341f0b11cb1a7.json"
	return path.Join(
		cfg.S3SessionPrefix,
		"orgs",
		fmt.Sprintf("%d", s.OrgID()),
		"c",
		string(s.ContactUUID()[:4]),
		string(s.ContactUUID()),
		fmt.Sprintf("%s_session_%s_%s.json", ts, s.UUID(), s.OutputMD5()),
	)
}

// ContactUUID returns the UUID of our contact
func (s *Session) ContactUUID() flows.ContactUUID {
	return s.contact.UUID()
}

// Contact returns the contact for this session
func (s *Session) Contact() *flows.Contact {
	return s.contact
}

// Runs returns our flow run
func (s *Session) Runs() []*FlowRun {
	return s.runs
}

// Sprint returns the sprint associated with this session
func (s *Session) Sprint() flows.Sprint {
	return s.sprint
}

// Wait returns the wait associated with this session (if any)
func (s *Session) Wait() flows.ActivatedWait {
	return s.wait
}

// FindStep finds the run and step with the given UUID
func (s *Session) FindStep(uuid flows.StepUUID) (flows.FlowRun, flows.Step) {
	return s.findStep(uuid)
}

// Timeout returns the amount of time after our last message sends that we should timeout
func (s *Session) Timeout() *time.Duration {
	return s.timeout
}

// OutputMD5 returns the md5 of the passed in session
func (s *Session) OutputMD5() string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s.s.Output)))
}

// SetIncomingMsg set the incoming message that this session should be associated with in this sprint
func (s *Session) SetIncomingMsg(id flows.MsgID, externalID null.String) {
	s.incomingMsgID = MsgID(id)
	s.incomingExternalID = externalID
}

// SetChannelConnection sets the channel connection associated with this sprint
func (s *Session) SetChannelConnection(cc *ChannelConnection) {
	connID := cc.ID()
	s.s.ConnectionID = &connID
	s.channelConnection = cc

	// also set it on all our runs
	for _, r := range s.runs {
		r.SetConnectionID(&connID)
	}
}

func (s *Session) ChannelConnection() *ChannelConnection {
	return s.channelConnection
}

// MarshalJSON is our custom marshaller so that our inner struct get output
func (s *Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.s)
}

// UnmarshalJSON is our custom marshaller so that our inner struct get output
func (s *Session) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &s.s)
}

// NewSession a session objects from the passed in flow session. It does NOT
// commit said session to the database.
func NewSession(ctx context.Context, tx *sqlx.Tx, org *OrgAssets, fs flows.Session, sprint flows.Sprint) (*Session, error) {
	output, err := json.Marshal(fs)
	if err != nil {
		return nil, errors.Wrapf(err, "error marshalling flow session")
	}

	// map our status over
	sessionStatus, found := sessionStatusMap[fs.Status()]
	if !found {
		return nil, errors.Errorf("unknown session status: %s", fs.Status())
	}

	// session must have at least one run
	if len(fs.Runs()) < 1 {
		return nil, errors.Errorf("cannot write session that has no runs")
	}

	// figure out our type
	sessionType, found := flowTypeMapping[fs.Type()]
	if !found {
		return nil, errors.Errorf("unknown flow type: %s", fs.Type())
	}

	uuid := fs.UUID()
	if uuid == "" {
		uuid = flows.SessionUUID(uuids.New())
	}

	// create our session object
	session := &Session{}
	s := &session.s
	s.UUID = uuid
	s.Status = sessionStatus
	s.SessionType = sessionType
	s.Responded = false
	s.Output = null.String(output)
	s.ContactID = ContactID(fs.Contact().ID())
	s.OrgID = org.OrgID()
	s.CreatedOn = fs.Runs()[0].CreatedOn()

	session.contact = fs.Contact()
	session.scene = NewSceneForSession(session)

	session.sprint = sprint
	session.wait = fs.Wait()
	session.findStep = fs.FindStep

	// now build up our runs
	for _, r := range fs.Runs() {
		run, err := newRun(ctx, tx, org, session, r)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating run: %s", r.UUID())
		}

		// save the run to our session
		session.runs = append(session.runs, run)

		// if this run is waiting, save it as the current flow
		if r.Status() == flows.RunStatusWaiting {
			flowID, err := FlowIDForUUID(ctx, tx, org, r.FlowReference().UUID)
			if err != nil {
				return nil, errors.Wrapf(err, "error loading current flow for UUID: %s", r.FlowReference().UUID)
			}
			s.CurrentFlowID = flowID
		}
	}

	// calculate our timeout if any
	session.calculateTimeout(fs, sprint)

	return session, nil
}

// ActiveSessionForContact returns the active session for the passed in contact, if any
func ActiveSessionForContact(ctx context.Context, db *sqlx.DB, st storage.Storage, org *OrgAssets, sessionType FlowType, contact *flows.Contact) (*Session, error) {
	rows, err := db.QueryxContext(ctx, selectLastSessionSQL, sessionType, contact.ID())
	if err != nil {
		return nil, errors.Wrapf(err, "error selecting active session")
	}
	defer rows.Close()

	// no rows? no sessions!
	if !rows.Next() {
		return nil, nil
	}

	// scan in our session
	session := &Session{
		contact: contact,
	}
	session.scene = NewSceneForSession(session)

	if err := rows.StructScan(&session.s); err != nil {
		return nil, errors.Wrapf(err, "error scanning session")
	}

	// load our output from storage if necessary
	if session.OutputURL() != "" {
		// strip just the path out of our output URL
		u, err := url.Parse(session.OutputURL())
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing output URL: %s", session.OutputURL())
		}

		start := time.Now()

		_, output, err := st.Get(ctx, u.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading session from storage: %s", session.OutputURL())
		}

		logrus.WithField("elapsed", time.Since(start)).WithField("output_url", session.OutputURL()).Debug("loaded session from storage")
		session.s.Output = null.String(output)
	}

	return session, nil
}

const selectLastSessionSQL = `
SELECT 
	id,
	uuid,
	session_type,
	status,
	responded,
	output,
	output_url,
	contact_id,
	org_id,
	created_on,
	ended_on,
	timeout_on,
	wait_started_on,
	current_flow_id,
	connection_id
FROM 
	flows_flowsession fs
WHERE
    session_type = $1 AND
	contact_id = $2 AND
	status = 'W'
ORDER BY
	created_on DESC
LIMIT 1
`

const insertCompleteSessionSQL = `
INSERT INTO
	flows_flowsession( uuid, session_type, status, responded, output, output_url, contact_id, org_id, created_on, ended_on, wait_started_on, connection_id)
               VALUES(:uuid,:session_type,:status,:responded,:output,:output_url,:contact_id,:org_id, NOW(),      NOW(),    NULL,           :connection_id)
RETURNING id
`

const insertIncompleteSessionSQL = `
INSERT INTO
	flows_flowsession( uuid, session_type, status, responded, output, output_url, contact_id, org_id, created_on, current_flow_id, timeout_on, wait_started_on, connection_id)
               VALUES(:uuid,:session_type,:status,:responded,:output,:output_url,:contact_id,:org_id, NOW(),     :current_flow_id,:timeout_on,:wait_started_on,:connection_id)
RETURNING id
`

const insertCompleteSessionSQLNoOutput = `
INSERT INTO
	flows_flowsession( uuid, session_type, status, responded, output_url, contact_id, org_id, created_on, ended_on, wait_started_on, connection_id)
               VALUES(:uuid,:session_type,:status,:responded, :output_url,:contact_id,:org_id, NOW(),      NOW(),    NULL,           :connection_id)
RETURNING id
`

const insertIncompleteSessionSQLNoOutput = `
INSERT INTO
	flows_flowsession( uuid, session_type, status, responded,  output_url, contact_id, org_id, created_on, ended_on, current_flow_id, timeout_on, wait_started_on, connection_id)
               VALUES(:uuid,:session_type,:status,:responded, :output_url,:contact_id,:org_id, NOW(), NOW(),    :current_flow_id,:timeout_on,:wait_started_on,:connection_id)
RETURNING id
`

// FlowSession creates a flow session for the passed in session object. It also populates the runs we know about
func (s *Session) FlowSession(cfg *runtime.Config, sa flows.SessionAssets, env envs.Environment) (flows.Session, error) {
	session, err := goflow.Engine(cfg).ReadSession(sa, json.RawMessage(s.s.Output), assets.IgnoreMissing)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to unmarshal session")
	}

	// walk through our session, populate seen runs
	s.seenRuns = make(map[flows.RunUUID]time.Time, len(session.Runs()))
	for _, r := range session.Runs() {
		s.seenRuns[r.UUID()] = r.ModifiedOn()
	}

	return session, nil
}

// calculates the timeout value for this session based on our waits
func (s *Session) calculateTimeout(fs flows.Session, sprint flows.Sprint) {
	// if we are on a wait and it has a timeout
	if fs.Wait() != nil && fs.Wait().TimeoutSeconds() != nil {
		now := time.Now()
		s.s.WaitStartedOn = &now

		seconds := time.Duration(*fs.Wait().TimeoutSeconds()) * time.Second
		s.timeout = &seconds

		timeoutOn := now.Add(seconds)
		s.s.TimeoutOn = &timeoutOn
	} else {
		s.s.WaitStartedOn = nil
		s.s.TimeoutOn = nil
		s.timeout = nil
	}
}

// WriteUpdatedSession updates the session based on the state passed in from our engine session, this also takes care of applying any event hooks
func (s *Session) WriteUpdatedSession(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, org *OrgAssets, fs flows.Session, sprint flows.Sprint, hook SessionCommitHook) error {
	// make sure we have our seen runs
	if s.seenRuns == nil {
		return errors.Errorf("missing seen runs, cannot update session")
	}

	output, err := json.Marshal(fs)
	if err != nil {
		return errors.Wrapf(err, "error marshalling flow session")
	}
	s.s.Output = null.String(output)

	// map our status over
	status, found := sessionStatusMap[fs.Status()]
	if !found {
		return errors.Errorf("unknown session status: %s", fs.Status())
	}
	s.s.Status = status

	// now build up our runs
	for _, r := range fs.Runs() {
		run, err := newRun(ctx, tx, org, s, r)
		if err != nil {
			return errors.Wrapf(err, "error creating run: %s", r.UUID())
		}

		// set the run on our session
		s.runs = append(s.runs, run)
	}

	// calculate our new timeout
	s.calculateTimeout(fs, sprint)

	// set our sprint, wait and step finder
	s.sprint = sprint
	s.wait = fs.Wait()
	s.findStep = fs.FindStep

	// run through our runs to figure out our current flow
	for _, r := range fs.Runs() {
		// if this run is waiting, save it as the current flow
		if r.Status() == flows.RunStatusWaiting {
			flowID, err := FlowIDForUUID(ctx, tx, org, r.FlowReference().UUID)
			if err != nil {
				return errors.Wrapf(err, "error loading flow: %s", r.FlowReference().UUID)
			}
			s.s.CurrentFlowID = flowID
		}

		// if we haven't already been marked as responded, walk our runs looking for an input
		if !s.s.Responded {
			// run through events, see if any are received events
			for _, e := range r.Events() {
				if e.Type() == events.TypeMsgReceived {
					s.s.Responded = true
					break
				}
			}
		}
	}

	// apply all our pre write events
	for _, e := range sprint.Events() {
		err := ApplyPreWriteEvent(ctx, rt, tx, org, s.scene, e)
		if err != nil {
			return errors.Wrapf(err, "error applying event: %v", e)
		}
	}

	// the SQL statement we'll use to update this session
	updateSQL := updateSessionSQL

	// if writing to S3, do so
	if rt.Config.SessionStorage == "s3" {
		err := WriteSessionOutputsToStorage(ctx, rt, []*Session{s})
		if err != nil {
			logrus.WithError(err).Error("error writing session to s3")
		}

		// don't write output in our SQL
		updateSQL = updateSessionSQLNoOutput
	}

	// write our new session state to the db
	_, err = tx.NamedExecContext(ctx, updateSQL, s.s)
	if err != nil {
		return errors.Wrapf(err, "error updating session")
	}

	// if this session is complete, so is any associated connection
	if s.channelConnection != nil {
		if s.Status() == SessionStatusCompleted || s.Status() == SessionStatusFailed {
			err := s.channelConnection.UpdateStatus(ctx, tx, ConnectionStatusCompleted, 0, time.Now())
			if err != nil {
				return errors.Wrapf(err, "error update channel connection")
			}
		}
	}

	// figure out which runs are new and which are updated
	updatedRuns := make([]interface{}, 0, 1)
	newRuns := make([]interface{}, 0)
	for _, r := range s.Runs() {
		modified, found := s.seenRuns[r.UUID()]
		if !found {
			newRuns = append(newRuns, &r.r)
			continue
		}

		if r.ModifiedOn().After(modified) {
			updatedRuns = append(updatedRuns, &r.r)
			continue
		}
	}

	// call our global pre commit hook if present
	if hook != nil {
		err := hook(ctx, tx, rt.RP, org, []*Session{s})
		if err != nil {
			return errors.Wrapf(err, "error calling commit hook: %v", hook)
		}
	}

	// update all modified runs at once
	err = BulkQuery(ctx, "update runs", tx, updateRunSQL, updatedRuns)
	if err != nil {
		logrus.WithError(err).WithField("session", string(output)).Error("error while updating runs for session")
		return errors.Wrapf(err, "error updating runs")
	}

	// insert all new runs at once
	err = BulkQuery(ctx, "insert runs", tx, insertRunSQL, newRuns)
	if err != nil {
		return errors.Wrapf(err, "error writing runs")
	}

	if err := RecordFlowStatistics(ctx, rt, tx, []flows.Session{fs}, []flows.Sprint{sprint}); err != nil {
		return errors.Wrapf(err, "error saving flow statistics")
	}

	// apply all our events
	if s.Status() != SessionStatusFailed {
		err = HandleEvents(ctx, rt, tx, org, s.scene, sprint.Events())
		if err != nil {
			return errors.Wrapf(err, "error applying events: %d", s.ID())
		}
	}

	// gather all our pre commit events, group them by hook and apply them
	err = ApplyEventPreCommitHooks(ctx, rt, tx, org, []*Scene{s.scene})
	if err != nil {
		return errors.Wrapf(err, "error applying pre commit hook: %T", hook)
	}

	return nil
}

const updateSessionSQL = `
UPDATE 
	flows_flowsession
SET 
	output = :output, 
	output_url = :output_url,
	status = :status, 
	ended_on = CASE WHEN :status = 'W' THEN NULL ELSE NOW() END,
	responded = :responded,
	current_flow_id = :current_flow_id,
	timeout_on = :timeout_on,
	wait_started_on = :wait_started_on
WHERE 
	id = :id
`

const updateSessionSQLNoOutput = `
UPDATE 
	flows_flowsession
SET 
	output_url = :output_url,
	status = :status, 
	ended_on = CASE WHEN :status = 'W' THEN NULL ELSE NOW() END,
	responded = :responded,
	current_flow_id = :current_flow_id,
	timeout_on = :timeout_on,
	wait_started_on = :wait_started_on
WHERE 
	id = :id
`

const updateRunSQL = `
UPDATE
	flows_flowrun fr
SET
	is_active = r.is_active::bool,
	exit_type = r.exit_type,
	status = r.status,
	exited_on = r.exited_on::timestamp with time zone,
	expires_on = r.expires_on::timestamp with time zone,
	responded = r.responded::bool,
	results = r.results,
	path = r.path::jsonb,
	events = r.events::jsonb,
	current_node_uuid = r.current_node_uuid::uuid,
	modified_on = NOW()
FROM (
	VALUES(:uuid, :is_active, :exit_type, :status, :exited_on, :expires_on, :responded, :results, :path, :events, :current_node_uuid)
) AS
	r(uuid, is_active, exit_type, status, exited_on, expires_on, responded, results, path, events, current_node_uuid)
WHERE
	fr.uuid = r.uuid::uuid
`

// WriteSessions writes the passed in session to our database, writes any runs that need to be created
// as well as appying any events created in the session
func WriteSessions(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, org *OrgAssets, ss []flows.Session, sprints []flows.Sprint, hook SessionCommitHook) ([]*Session, error) {
	if len(ss) == 0 {
		return nil, nil
	}

	// create all our session objects
	sessions := make([]*Session, 0, len(ss))
	completeSessionsI := make([]interface{}, 0, len(ss))
	incompleteSessionsI := make([]interface{}, 0, len(ss))
	completedConnectionIDs := make([]ConnectionID, 0, 1)
	for i, s := range ss {
		session, err := NewSession(ctx, tx, org, s, sprints[i])
		if err != nil {
			return nil, errors.Wrapf(err, "error creating session objects")
		}
		sessions = append(sessions, session)

		if session.Status() == SessionStatusCompleted {
			completeSessionsI = append(completeSessionsI, &session.s)
			if session.channelConnection != nil {
				completedConnectionIDs = append(completedConnectionIDs, session.channelConnection.ID())
			}
		} else {
			incompleteSessionsI = append(incompleteSessionsI, &session.s)
		}
	}

	// apply all our pre write events
	for i := range ss {
		for _, e := range sprints[i].Events() {
			err := ApplyPreWriteEvent(ctx, rt, tx, org, sessions[i].scene, e)
			if err != nil {
				return nil, errors.Wrapf(err, "error applying event: %v", e)
			}
		}
	}

	// call our global pre commit hook if present
	if hook != nil {
		err := hook(ctx, tx, rt.RP, org, sessions)
		if err != nil {
			return nil, errors.Wrapf(err, "error calling commit hook: %v", hook)
		}
	}

	// the SQL we'll use to do our insert of complete sessions
	insertCompleteSQL := insertCompleteSessionSQL
	insertIncompleteSQL := insertIncompleteSessionSQL

	// if writing our sessions to S3, do so
	if rt.Config.SessionStorage == "s3" {
		err := WriteSessionOutputsToStorage(ctx, rt, sessions)
		if err != nil {
			// for now, continue on for errors, we are still reading from the DB
			logrus.WithError(err).Error("error writing sessions to s3")
		}

		insertCompleteSQL = insertCompleteSessionSQLNoOutput
		insertIncompleteSQL = insertIncompleteSessionSQLNoOutput
	}

	// insert our complete sessions first
	err := BulkQuery(ctx, "insert completed sessions", tx, insertCompleteSQL, completeSessionsI)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting completed sessions")
	}

	// mark any connections that are done as complete as well
	err = UpdateChannelConnectionStatuses(ctx, tx, completedConnectionIDs, ConnectionStatusCompleted)
	if err != nil {
		return nil, errors.Wrapf(err, "error updating channel connections to complete")
	}

	// insert incomplete sessions
	err = BulkQuery(ctx, "insert incomplete sessions", tx, insertIncompleteSQL, incompleteSessionsI)
	if err != nil {
		return nil, errors.Wrapf(err, "error inserting incomplete sessions")
	}

	// for each session associate our run with each
	runs := make([]interface{}, 0, len(sessions))
	for _, s := range sessions {
		for _, r := range s.runs {
			runs = append(runs, &r.r)

			// set our session id now that it is written
			r.SetSessionID(s.ID())
		}
	}

	// insert all runs
	err = BulkQuery(ctx, "insert runs", tx, insertRunSQL, runs)
	if err != nil {
		return nil, errors.Wrapf(err, "error writing runs")
	}

	if err := RecordFlowStatistics(ctx, rt, tx, ss, sprints); err != nil {
		return nil, errors.Wrapf(err, "error saving flow statistics")
	}

	// apply our all events for the session
	scenes := make([]*Scene, 0, len(ss))
	for i := range sessions {
		if ss[i].Status() == SessionStatusFailed {
			continue
		}

		err = HandleEvents(ctx, rt, tx, org, sessions[i].Scene(), sprints[i].Events())
		if err != nil {
			return nil, errors.Wrapf(err, "error applying events for session: %d", sessions[i].ID())
		}

		scene := sessions[i].Scene()
		scenes = append(scenes, scene)
	}

	// gather all our pre commit events, group them by hook
	err = ApplyEventPreCommitHooks(ctx, rt, tx, org, scenes)
	if err != nil {
		return nil, errors.Wrapf(err, "error applying pre commit hook: %T", hook)
	}

	// return our session
	return sessions, nil
}

// FindActiveSessionOverlap returns the list of contact ids which overlap with those passed in which are active in any other flows
func FindActiveSessionOverlap(ctx context.Context, db *sqlx.DB, flowType FlowType, contacts []ContactID) ([]ContactID, error) {
	// background flows should look at messaging flows when determing overlap (background flows can't be active by definition)
	if flowType == FlowTypeBackground {
		flowType = FlowTypeMessaging
	}

	var overlap []ContactID
	err := db.SelectContext(ctx, &overlap, activeSessionOverlapSQL, flowType, pq.Array(contacts))
	return overlap, err
}

const activeSessionOverlapSQL = `
SELECT
	DISTINCT(contact_id)
FROM
	flows_flowsession fs JOIN
	flows_flow ff ON fs.current_flow_id = ff.id
WHERE
	fs.status = 'W' AND
	ff.is_active = TRUE AND
	ff.is_archived = FALSE AND
	ff.flow_type = $1 AND
	fs.contact_id = ANY($2)
`

// ExitSessions marks the passed in sessions as completed, also doing so for all associated runs
func ExitSessions(ctx context.Context, tx Queryer, sessionIDs []SessionID, exitType ExitType) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	// map exit type to statuses for sessions and runs
	sessionStatus := exitToSessionStatusMap[exitType]
	runStatus, found := exitToRunStatusMap[exitType]
	if !found {
		return errors.Errorf("unknown exit type: %s", exitType)
	}

	for _, idBatch := range chunkSessionIDs(sessionIDs, 1000) {
		// first interrupt our runs
		start := time.Now()

		res, err := tx.ExecContext(ctx, exitSessionRunsSQL, pq.Array(idBatch), exitType, time.Now(), runStatus)
		if err != nil {
			return errors.Wrapf(err, "error exiting session runs")
		}

		rows, _ := res.RowsAffected()
		logrus.WithField("count", rows).WithField("elapsed", time.Since(start)).Debug("exited session runs")

		// then our sessions
		start = time.Now()

		res, err = tx.ExecContext(ctx, exitSessionsSQL, pq.Array(idBatch), time.Now(), sessionStatus)
		if err != nil {
			return errors.Wrapf(err, "error exiting sessions")
		}

		rows, _ = res.RowsAffected()
		logrus.WithField("count", rows).WithField("elapsed", time.Since(start)).Debug("exited sessions")
	}

	return nil
}

const exitSessionRunsSQL = `
UPDATE
	flows_flowrun
SET
	is_active = FALSE,
	exit_type = $2,
	exited_on = $3,
	status = $4,
	modified_on = NOW()
WHERE
	id = ANY (SELECT id FROM flows_flowrun WHERE session_id = ANY($1) AND is_active = TRUE)
`

const exitSessionsSQL = `
UPDATE
	flows_flowsession
SET
	ended_on = $2,
	status = $3
WHERE
	id = ANY ($1) AND
	status = 'W'
`

package ticket

import (
	"context"
	"net/http"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/excellent/types"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
)

func init() {
	web.RegisterJSONRoute(http.MethodPost, "/mr/ticket/open", web.RequireAuthToken(web.WithHTTPLogs(handleOpen)))
}

// Open ticket with given params.
//
//		{
//		  "org_id": 123,
//		  "contact_id": 123,
//		  "ticketer_id": 123,
//		  "topic_id": 123,
//		  "assignee_id": 123,
//	    "extra": "{}",
//		}
func handleOpen(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	request := &openTicketRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return errors.Wrapf(err, "request failed validation"), http.StatusBadRequest, nil
	}

	oa, err := models.GetOrgAssets(ctx, rt, request.OrgID)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to load org assets")
	}

	ticketer := oa.TicketerByID(request.TicketerID)
	if ticketer == nil {
		return nil, 400, errors.New("unable to find department")
	}
	flowsTicketer := flows.NewTicketer(ticketer)

	var assignee *flows.User = nil
	if request.AssigneeID != 0 {
		assigneeModel := oa.UserByID(request.AssigneeID)
		assignee = flows.NewUser(&static.User{Email_: assigneeModel.Email(), Name_: assigneeModel.Name()})
	}

	topicModel := oa.TopicByID(models.TopicID(request.TopicID))
	topic := flows.NewTopic(topicModel)

	contactModel, err := models.LoadContact(ctx, rt.DB, oa, request.ContactID)
	if err != nil {
		return nil, 500, errors.Wrapf(err, "error on find contact")
	}
	contact, _ := contactModel.FlowContact(oa)

	currentSession := newEndpointSession(contact, oa.Env())

	svc, err := ticketer.AsService(rt.Config, flowsTicketer)
	if err != nil {
		return nil, 500, errors.Wrapf(err, "error creating ticketer service")
	}

	openedTicket, err := svc.Open(currentSession, topic, request.Extra, assignee, l.Ticketer(ticketer))
	if err != nil {
		return nil, 500, err
	}

	newTicket := models.NewTicket(
		openedTicket.UUID(),
		request.OrgID,
		request.ContactID,
		request.TicketerID,
		openedTicket.ExternalID(),
		request.TopicID,
		openedTicket.Body(),
		models.NilUserID,
		nil,
	)

	tx, err := rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 500, errors.Wrap(err, "error starting transaction")
	}
	err = models.InsertTickets(ctx, tx, []*models.Ticket{newTicket})
	if err != nil {
		tx.Rollback()
		return nil, 500, errors.Wrap(err, "error inserting tickets")
	}

	evt := models.NewTicketOpenedEvent(newTicket, models.NilUserID, models.NilUserID)
	err = models.InsertTicketEvents(ctx, tx, []*models.TicketEvent{evt})
	if err != nil {
		tx.Rollback()
		return nil, 500, errors.Wrap(err, "error inserting ticket events")
	}
	err = models.NotificationsFromTicketEvents(ctx, tx, oa, map[*models.Ticket]*models.TicketEvent{newTicket: evt})
	if err != nil {
		tx.Rollback()
		return nil, 500, errors.Wrap(err, "error inserting notifications")
	}
	err = tx.Commit()
	if err != nil {
		return nil, 500, errors.Wrap(err, "error committing transaction")
	}
	return openedTicket, 200, nil
}

type openTicketRequest struct {
	OrgID      models.OrgID      `json:"org_id"`
	ContactID  models.ContactID  `json:"contact_id"`
	TicketerID models.TicketerID `json:"ticketer_id"`
	TopicID    models.TopicID    `json:"topic_id"`
	AssigneeID models.UserID     `json:"assignee_id"`
	Extra      string            `json:"extra"`
}

type endpointSession struct {
	assets flows.SessionAssets

	uuid          flows.SessionUUID
	type_         flows.FlowType
	env           envs.Environment
	trigger       flows.Trigger
	currentResume flows.Resume
	contact       *flows.Contact
	runs          []flows.FlowRun
	status        flows.SessionStatus
	wait          flows.ActivatedWait
	input         flows.Input
}

func newEndpointSession(contact *flows.Contact, env envs.Environment) flows.Session {
	return &endpointSession{
		uuid:    flows.SessionUUID(uuids.New()),
		contact: contact,
		env:     env,
	}
}

func (s *endpointSession) Assets() flows.SessionAssets { return s.assets }

func (s *endpointSession) UUID() flows.SessionUUID  { return s.uuid }
func (s *endpointSession) Type() flows.FlowType     { return s.type_ }
func (s *endpointSession) SetType(t flows.FlowType) { s.type_ = t }

func (s *endpointSession) Environment() envs.Environment     { return s.env }
func (s *endpointSession) SetEnvironment(e envs.Environment) { s.env = e }

func (s *endpointSession) Contact() *flows.Contact     { return s.contact }
func (s *endpointSession) SetContact(c *flows.Contact) { s.contact = c }

func (s *endpointSession) Input() flows.Input         { return s.input }
func (s *endpointSession) SetInput(input flows.Input) { s.input = input }

func (s *endpointSession) Status() flows.SessionStatus                      { return s.status }
func (s *endpointSession) Trigger() flows.Trigger                           { return s.trigger }
func (s *endpointSession) CurrentResume() flows.Resume                      { return s.currentResume }
func (s *endpointSession) BatchStart() bool                                 { return false }
func (s *endpointSession) PushFlow(f flows.Flow, fr flows.FlowRun, ps bool) {}
func (s *endpointSession) Wait() flows.ActivatedWait                        { return s.wait }

func (s *endpointSession) Resume(r flows.Resume) (flows.Sprint, error)              { return nil, nil }
func (s *endpointSession) Runs() []flows.FlowRun                                    { return s.runs }
func (s *endpointSession) GetRun(uuid flows.RunUUID) (flows.FlowRun, error)         { return nil, nil }
func (s *endpointSession) FindStep(uuid flows.StepUUID) (flows.FlowRun, flows.Step) { return nil, nil }
func (s *endpointSession) GetCurrentChild(f flows.FlowRun) flows.FlowRun            { return nil }
func (s *endpointSession) ParentRun() flows.RunSummary                              { return nil }
func (s *endpointSession) CurrentContext() *types.XObject                           { return nil }
func (s *endpointSession) History() *flows.SessionHistory                           { return nil }

func (s *endpointSession) Engine() flows.Engine { return nil }

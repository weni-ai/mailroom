package goflow

import (
	"sync"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/goflow/services/brain"
	"github.com/nyaruka/goflow/services/webhooks"
	"github.com/nyaruka/goflow/services/wenigpt"
	"github.com/nyaruka/mailroom/runtime"

	"github.com/shopspring/decimal"
)

var eng, simulator flows.Engine
var engInit, simulatorInit sync.Once

var emailFactory func(*runtime.Config) engine.EmailServiceFactory
var classificationFactory func(*runtime.Config) engine.ClassificationServiceFactory
var ticketFactory func(*runtime.Config) engine.TicketServiceFactory
var airtimeFactory func(*runtime.Config) engine.AirtimeServiceFactory
var externalServiceFactory func(*runtime.Config) engine.ExternalServiceServiceFactory
var msgCatalogFactory func(*runtime.Config) engine.MsgCatalogServiceFactory
var orgContextFactory func(*runtime.Config) engine.OrgContextServiceFactory

// RegisterEmailServiceFactory can be used by outside callers to register a email factory
// for use by the engine
func RegisterEmailServiceFactory(f func(*runtime.Config) engine.EmailServiceFactory) {
	emailFactory = f
}

// RegisterClassificationServiceFactory can be used by outside callers to register a classification factory
// for use by the engine
func RegisterClassificationServiceFactory(f func(*runtime.Config) engine.ClassificationServiceFactory) {
	classificationFactory = f
}

// RegisterTicketServiceFactory can be used by outside callers to register a ticket service factory
// for use by the engine
func RegisterTicketServiceFactory(f func(*runtime.Config) engine.TicketServiceFactory) {
	ticketFactory = f
}

// RegisterAirtimeServiceFactory can be used by outside callers to register a airtime factory
// for use by the engine
func RegisterAirtimeServiceFactory(f func(*runtime.Config) engine.AirtimeServiceFactory) {
	airtimeFactory = f
}

func RegisterExternalServiceServiceFactory(f func(*runtime.Config) engine.ExternalServiceServiceFactory) {
	externalServiceFactory = f
}

func RegisterMsgCatalogServiceFactory(f func(*runtime.Config) engine.MsgCatalogServiceFactory) {
	msgCatalogFactory = f
}

func RegisterOrgContextServiceFactory(f func(*runtime.Config) engine.OrgContextServiceFactory) {
	orgContextFactory = f
}

// Engine returns the global engine instance for use with real sessions
func Engine(c *runtime.Config) flows.Engine {
	engInit.Do(func() {
		webhookHeaders := map[string]string{
			"User-Agent":      "RapidProMailroom/" + c.Version,
			"X-Mailroom-Mode": "normal",
		}

		httpClient, httpRetries, httpAccess := HTTP(c)

		eng = engine.NewBuilder().
			WithWebhookServiceFactory(webhooks.NewServiceFactory(httpClient, httpRetries, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes)).
			WithWeniGPTServiceFactory(wenigpt.NewServiceFactory(httpClient, httpRetries, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes, c.WenigptAuthToken, c.WenigptBaseURL)).
			WithBrainServiceFactory(brain.NewServiceFactory(httpClient, httpRetries, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes, c.RouterAuthToken, c.RouterBaseURL)).
			WithClassificationServiceFactory(classificationFactory(c)).
			WithEmailServiceFactory(emailFactory(c)).
			WithTicketServiceFactory(ticketFactory(c)).
			WithExternalServiceServiceFactory(externalServiceFactory((c))).
			WithMsgCatalogServiceFactory(msgCatalogFactory((c))). // msg catalog
			WithOrgContextServiceFactory(orgContextFactory((c))).
			WithAirtimeServiceFactory(airtimeFactory(c)).
			WithMaxStepsPerSprint(c.MaxStepsPerSprint).
			WithMaxResumesPerSession(c.MaxResumesPerSession).
			Build()
	})

	return eng
}

// Simulator returns the global engine instance for use with simulated sessions
func Simulator(c *runtime.Config) flows.Engine {
	simulatorInit.Do(func() {
		webhookHeaders := map[string]string{
			"User-Agent":      "RapidProMailroom/" + c.Version,
			"X-Mailroom-Mode": "simulation",
		}

		httpClient, _, httpAccess := HTTP(c) // don't do retries in simulator

		simulator = engine.NewBuilder().
			WithWebhookServiceFactory(webhooks.NewServiceFactory(httpClient, nil, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes)).
			WithWeniGPTServiceFactory(wenigpt.NewServiceFactory(httpClient, nil, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes, c.WenigptAuthToken, c.WenigptBaseURL)).
			WithBrainServiceFactory(brain.NewServiceFactory(httpClient, httpRetries, httpAccess, webhookHeaders, c.WebhooksMaxBodyBytes, c.RouterAuthToken, c.RouterBaseURL)).
			WithClassificationServiceFactory(classificationFactory(c)).     // simulated sessions do real classification
			WithExternalServiceServiceFactory(externalServiceFactory((c))). // and real external services
			WithMsgCatalogServiceFactory(msgCatalogFactory((c))).           // msg catalog
			WithEmailServiceFactory(simulatorEmailServiceFactory).          // but faked emails
			WithTicketServiceFactory(simulatorTicketServiceFactory).        // and faked tickets
			WithAirtimeServiceFactory(simulatorAirtimeServiceFactory).      // and faked airtime transfers
			WithMaxStepsPerSprint(c.MaxStepsPerSprint).
			WithMaxResumesPerSession(c.MaxResumesPerSession).
			Build()
	})

	return simulator
}

func simulatorEmailServiceFactory(session flows.Session) (flows.EmailService, error) {
	return &simulatorEmailService{}, nil
}

type simulatorEmailService struct{}

func (s *simulatorEmailService) Send(session flows.Session, addresses []string, subject, body string) error {
	return nil
}

func simulatorTicketServiceFactory(session flows.Session, ticketer *flows.Ticketer) (flows.TicketService, error) {
	return &simulatorTicketService{ticketer: ticketer}, nil
}

type simulatorTicketService struct {
	ticketer *flows.Ticketer
}

func (s *simulatorTicketService) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	return flows.OpenTicket(s.ticketer, topic, body, assignee), nil
}

func simulatorAirtimeServiceFactory(session flows.Session) (flows.AirtimeService, error) {
	return &simulatorAirtimeService{}, nil
}

type simulatorAirtimeService struct{}

func (s *simulatorAirtimeService) Transfer(session flows.Session, sender urns.URN, recipient urns.URN, amounts map[string]decimal.Decimal, logHTTP flows.HTTPLogCallback) (*flows.AirtimeTransfer, error) {
	transfer := &flows.AirtimeTransfer{
		Sender:        sender,
		Recipient:     recipient,
		DesiredAmount: decimal.Zero,
		ActualAmount:  decimal.Zero,
	}

	// pick arbitrary currency/amount pair in map
	for currency, amount := range amounts {
		transfer.Currency = currency
		transfer.DesiredAmount = amount
		transfer.ActualAmount = amount
		break
	}

	return transfer, nil
}

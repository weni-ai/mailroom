package chatgpt

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var baseURL = "https://api.openai.com"

var db *sqlx.DB
var mu = &sync.Mutex{}

func initDB(dbURL string) error {
	mu.Lock()
	defer mu.Unlock()
	if db == nil {
		newDB, err := sqlx.Open("postgres", dbURL)
		if err != nil {
			return errors.Wrap(err, "unable to open database connection")
		}
		SetDB(newDB)
	}
	return nil
}

func SetDB(newDB *sqlx.DB) {
	db = newDB
}

const (
	serviceType = "chatgpt"
)

func init() {
	models.RegisterExternalServiceService(serviceType, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	redactor   utils.Redactor
	config     map[string]string
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, externalService *flows.ExternalService, config map[string]string) (models.ExternalServiceService, error) {
	apiKey := config["api_key"]

	if apiKey != "" {
		if err := initDB(rtCfg.DB); err != nil {
			return nil, err
		}

		return &service{
			rtConfig:   rtCfg,
			restClient: NewClient(httpClient, httpRetries, baseURL, apiKey),
			redactor:   utils.NewRedactor(flows.RedactionMask, apiKey),
			config:     config,
		}, nil
	}

	return nil, errors.New("missing api_key in external service for chatgpt config")
}

func (s *service) Call(session flows.Session, callAction assets.ExternalServiceCallAction, params []assets.ExternalServiceParam, logHTTP flows.HTTPLogCallback) (*flows.ExternalServiceCall, error) {
	call := callAction.Name

	callResult := &flows.ExternalServiceCall{}
	sendHistory := false
	input := ""
	promptMessages := []ChatCompletionMessage{}
	// default could be "gpt-3.5-turbo"
	aiModel := s.config["ai_model"]

	switch call {
	case "ConsultarChatGPT":

		request := &ChatCompletionRequest{
			Model: aiModel,
		}

		for _, param := range params {
			dv := param.Data.Value
			switch param.Type {
			case "AditionalPrompts":

				pmdv, ok := param.Data.Value.([]interface{})
				if !ok {
					return nil, errors.New("error on convert 'Data.Value' to []interface{}")
				}
				for _, element := range pmdv {
					if elem, ok := element.(map[string]interface{}); ok {
						if text, ok := elem["text"].(string); ok {
							newMsgParam := ChatCompletionMessage{
								Role:    ChatMessageRoleSystem,
								Content: text,
							}
							promptMessages = append(promptMessages, newMsgParam)
						}
					}
				}

			case "SendCompleteHistory":
				var ok bool
				sendHistory, ok = dv.(bool)
				if !ok {
					sendHistory = false
				}
			case "UserInput":
				if dv == "" || dv == nil {
					return nil, errors.New("error on call chatgpt: input can't be empty")
				}
				input = dv.(string)
			}
		}

		if sendHistory {
			contact := session.Contact()
			after := session.Runs()[0].CreatedOn()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			msgs, err := models.SelectContactMessages(ctx, db, int(contact.ID()), after)
			if err != nil {
				logrus.Error(errors.Wrap(err, "failed to get history messages"))
			}

			sort.SliceStable(msgs, func(i, j int) bool {
				return msgs[i].CreatedOn().Before(msgs[j].CreatedOn())
			})

			for _, msg := range msgs {
				m := ChatCompletionMessage{
					Content: msg.Text(),
				}
				if msg.Direction() == "I" {
					m.Role = ChatMessageRoleUser
				} else {
					m.Role = ChatMessageRoleAssistant
				}
				request.Messages = append(request.Messages, m)
			}
		}

		rulesText := s.config["rules"]
		if rulesText != "" {
			rulesMsg := ChatCompletionMessage{
				Role:    ChatMessageRoleSystem,
				Content: rulesText,
			}
			request.Messages = append(request.Messages, rulesMsg)
		}

		knowledgeBaseText := s.config["knowledge_base"]
		if knowledgeBaseText != "" {
			knowledgeBaseMsg := ChatCompletionMessage{
				Role:    ChatMessageRoleSystem,
				Content: knowledgeBaseText,
			}
			request.Messages = append(request.Messages, knowledgeBaseMsg)
		}

		if len(promptMessages) > 0 {
			request.Messages = append(request.Messages, promptMessages...)
		}

		request.Messages = append(
			request.Messages,
			ChatCompletionMessage{
				Role:    ChatMessageRoleUser,
				Content: input,
			})

		r, t, err := s.restClient.CreateChatCompletion(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call openai create completion")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
		callResult.RequestMethod = t.Request.Method
		callResult.RequestURL = t.Request.URL.String()
	}
	return callResult, nil
}

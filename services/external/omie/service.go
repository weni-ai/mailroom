package omie

import (
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
)

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	redactor   utils.Redactor
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, externalService *flows.ExternalService, config map[string]string) (models.ExternalServiceService, error) {
	authToken := config["auth_token"]
	return &service{
		rtConfig:   rtCfg,
		restClient: NewClient(httpClient, httpRetries, "todo-base-url", authToken),
		redactor:   utils.NewRedactor(flows.RedactionMask),
	}, nil
}

func (s *service) Call(sesion flows.Session, body string, logHTTP flows.HTTPLogCallback) (*flows.ExternalServiceCall, error) {
	return nil, nil
}

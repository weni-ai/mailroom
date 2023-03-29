package omie

import (
	"encoding/json"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
)

var baseURL = "https://app.omie.com.br/api"

const (
	serviceType = "omie"
)

func init() {
	models.RegisterExternalServiceService(serviceType, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	redactor   utils.Redactor
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, externalService *flows.ExternalService, config map[string]string) (models.ExternalServiceService, error) {
	appKey := config["app_key"]
	appSecret := config["app_secret"]
	return &service{
		rtConfig:   rtCfg,
		restClient: NewClient(httpClient, httpRetries, baseURL, appKey, appSecret),
		redactor:   utils.NewRedactor(flows.RedactionMask),
	}, nil
}

func (s *service) Call(sesion flows.Session, callAction assets.ExternalServiceCallAction, params []assets.ExternalServiceParam, logHTTP flows.HTTPLogCallback) (*flows.ExternalServiceCall, error) {
	call := callAction.Name

	callResult := &flows.ExternalServiceCall{}

	switch call {
	case "IncluirContato":
		request, err := ParamsToIncluirContatoRequest(params)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal IncluirContatoRequest")
		}
		r, t, err := s.restClient.IncluirContato(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirContatoRequest")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
		callResult.RequestMethod = t.Request.Method
		callResult.RequestURL = t.Request.URL.String()
	case "IncluirOportunidade":
		request, err := ParamsToIncluirOportunidadeRequest(params)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal IncluirOportunidadeRequest")
		}
		r, t, err := s.restClient.IncluirOportunidade(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirOportunidade")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
		callResult.RequestMethod = t.Request.Method
		callResult.RequestURL = t.Request.URL.String()
	case "ListarClientes":
		request, err := ParamsToListarClientesRequest(params)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal ListarCLientesRequest")
		}
		r, t, err := s.restClient.ListarClientes(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie ListarClientes")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
		callResult.RequestMethod = t.Request.Method
		callResult.RequestURL = t.Request.URL.String()
	case "PesquisarLancamentos":
		request, err := ParamsToPesquisarLancamentosRequest(params)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal PesquisarLancamentosRequest")
		}
		r, t, err := s.restClient.PesquisarLancamentos(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie PesquisarLancamentos")
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

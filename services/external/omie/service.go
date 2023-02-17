package omie

import (
	"encoding/json"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/gocommon/httpx"
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

func (s *service) Call(sesion flows.Session, body string, logHTTP flows.HTTPLogCallback) (*flows.ExternalServiceCall, error) {
	call, err := jsonparser.GetString([]byte(body), "call")
	if err != nil {
		return nil, errors.Errorf("invalid jsonbody")
	}

	callResult := &flows.ExternalServiceCall{}

	switch call {
	case "IncluirContato":
		request := &IncluirContatoRequest{}
		err := json.Unmarshal([]byte(body), request)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal IncluirContatoRequest")
		}
		r, _, err := s.restClient.IncluirContato(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirContatoRequest")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
	case "IncluirOportunidade":
		request := &IncluirOportunidadeRequest{}
		err := json.Unmarshal([]byte(body), request)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal IncluirOportunidadeRequest")
		}
		r, _, err := s.restClient.IncluirOportunidade(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirOportunidade")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
	case "ListarClientes":
		request := &ListarClientesRequest{}
		err := json.Unmarshal([]byte(body), request)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal ListarCLientesRequest")
		}
		r, _, err := s.restClient.ListarClientes(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirOportunidade")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
	case "PesquisarLancamentosRequest":
		request := &PesquisarLancamentosRequest{}
		err := json.Unmarshal([]byte(body), request)
		if err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal PesquisarLancamentosRequest")
		}
		r, _, err := s.restClient.PesquisarLancamentos(request)
		if err != nil {
			return nil, errors.Wrap(err, "error on call omie IncluirOportunidade")
		}
		callResult.ResponseJSON, err = json.Marshal(r)
		if err != nil {
			return nil, errors.Wrap(err, "error to marshal result for ExternalServiceCall.ResponseJSON")
		}
	}

	return callResult, nil
}

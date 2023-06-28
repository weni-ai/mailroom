package omie_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	"github.com/nyaruka/mailroom/services/external/omie"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://app.omie.com.br/api/v1/crm/contatos/": {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCod": 5491077640,
				"cCodInt": "12344321",
				"cCodStatus": "0",
				"cDesStatus": "Contato cadastrado com sucesso!"
			}`),
		},
	}))

	omieService := flows.NewExternalService(static.NewExternalService(assets.ExternalServiceUUID(uuids.New()), "omie", "omie"))

	svc, err := omie.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		omieService,
		map[string]string{
			"app_key":    "omie-app-key",
			"app_secret": "omie-app-secret",
		},
	)
	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	callAction := assets.ExternalServiceCallAction{Name: "IncluirContato", Value: "IncluirContato"}
	params := []assets.ExternalServiceParam{}
	call, err := svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie IncluirContatoRequest: mocked error")
	assert.Nil(t, call)

	jsonParams := `[
		{"type": "identificacao", "filter": {"value": {"name":"cCodInt"}}, "data": {"value": "12344321"}},
		{"type": "identificacao", "filter": {"value": {"name":"cNome"}}, "data": {"value": "contact123"}}
	]`
	var ps []assets.ExternalServiceParam
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)
}

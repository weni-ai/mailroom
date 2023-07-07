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
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCod": 38511295,
				"cCodInt": "",
				"cCodStatus": "0",
				"cDesStatus": "Contato encontrado com sucesso!"
			}`),
		},
		"https://app.omie.com.br/api/v1/crm/oportunidades/": {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCodOp": 5491163010,
				"cCodIntOp": "123321",
				"cCodStatus": "0",
				"cDesStatus": "Oportunidade cadastrada com sucesso!"
			}`),
		},
		"https://app.omie.com.br/api/v1/geral/clientes/": {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"pagina": 1,
				"total_de_paginas": 1,
				"registros": 50,
				"total_de_registros": 1,
				"clientes_cadastro": [
					{
						"bairro": "SETOR 01",
						"bloquear_faturamento": "N",
						"cep": "76870175",
						"cidade": "ARIQUEMES (RO)",
						"cidade_ibge": "1100023",
						"cnpj_cpf": "02.923.710/0001-19",
						"codigo_cliente_integracao": "teste",
						"codigo_cliente_omie": 2370765,
						"codigo_pais": "1058",
						"complemento": "A/C LUCIANE",
						"dadosBancarios": {
							"agencia": "",
							"codigo_banco": "",
							"conta_corrente": "",
							"doc_titular": "",
							"nome_titular": "ARTHUR HENRIQUE DEMARCHI",
							"transf_padrao": "N"
						},
						"email": "primeiro@ccliente.com.br",
						"endereco": "AVENIDA JAMARI",
						"enderecoEntrega": {},
						"endereco_numero": "2007",
						"estado": "RO",
						"exterior": "N",
						"inativo": "N",
						"info": {
							"cImpAPI": "S",
							"dAlt": "27/06/2023",
							"dInc": "07/04/2014",
							"hAlt": "17:22:30",
							"hInc": "10:56:29",
							"uAlt": "WEBSERVICE",
							"uInc": "WEBSERVICE"
						},
						"inscricao_estadual": "ISENTO",
						"inscricao_municipal": "",
						"nome_fantasia": "Primeiro Cliente",
						"optante_simples_nacional": "S",
						"pessoa_fisica": "N",
						"produtor_rural": "N",
						"razao_social": "Primeiro Cliente  Ltda Meeeee",
						"recomendacoes": {
							"codigo_transportadora": 359656432,
							"gerar_boletos": "S"
						},
						"tags": [
							{
								"tag": "Cliente"
							},
							{
								"tag": "Fornecedor"
							},
							{
								"tag": "Whatsapp 11999999999"
							}
						],
						"telefone1_ddd": "69",
						"telefone1_numero": "35357030",
						"telefone2_ddd": "69",
						"telefone2_numero": "35355627",
						"valor_limite_credito": 1000
					}
				]
			}`),
		},
		"https://app.omie.com.br/api/v1/financas/pesquisartitulos/": {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nPagina": 1,
				"nTotPaginas": 1,
				"nRegistros": 1,
				"nTotRegistros": 1,
				"titulosEncontrados": [
					{
						"cabecTitulo": {
							"aCodCateg": [
								{
									"cCodCateg": "1.01.02",
									"nPerc": 100,
									"nValor": 500
								}
							],
							"cCPFCNPJCliente": "07.607.851/0001-46",
							"cCodCateg": "1.01.02",
							"cCodIntTitulo": "D19021401",
							"cCodVendedor": 1944242,
							"cNSU": "12345",
							"cNatureza": "R",
							"cNumBoleto": "343434",
							"cNumDocFiscal": "123456789",
							"cNumParcela": "001/001",
							"cNumTitulo": "Teste",
							"cOperacao": "12",
							"cOrigem": "APIR",
							"cRetCOFINS": "",
							"cRetCSLL": "",
							"cRetINSS": "",
							"cRetIR": "",
							"cRetISS": "",
							"cRetPIS": "",
							"cStatus": "RECEBIDO",
							"cTipo": "CHQ",
							"dDtEmissao": "19/02/2014",
							"dDtPagamento": "11/05/2022",
							"dDtPrevisao": "11/05/2022",
							"dDtRegistro": "19/02/2014",
							"dDtVenc": "13/05/2022",
							"nCodCC": 4243124,
							"nCodCliente": 4214850,
							"nCodTitRepet": 2037086,
							"nCodTitulo": 2037086,
							"nValorCOFINS": 0,
							"nValorCSLL": 0,
							"nValorINSS": 0,
							"nValorIR": 0,
							"nValorISS": 30,
							"nValorPIS": 0,
							"nValorTitulo": 500,
							"observacao": "teste 2"
						},
						"departamentos": [
							{
								"cCodDepartamento": "1208234",
								"nDistrPercentual": 100,
								"nDistrValor": 125,
								"nValorFixo": "N"
							}
						],
						"lancamentos": [
							{
								"cCodIntLanc": "",
								"cNatureza": "R",
								"cObsLanc": "",
								"dDtLanc": "11/05/2022",
								"nCodCC": 4243124,
								"nCodLanc": 5481192054,
								"nDesconto": 0,
								"nIdLancCC": 5481192038,
								"nJuros": 0,
								"nMulta": 0,
								"nValLanc": 500
							}
						],
						"resumo": {
							"cLiquidado": "S",
							"nDesconto": 0,
							"nJuros": 0,
							"nMulta": 0,
							"nValAberto": 0,
							"nValLiquido": 500,
							"nValPago": 500
						}
					}
				]
			}`),
		},
		"https://app.omie.com.br/api/v1/financas/contareceberboleto/": {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "mocked error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"cLinkBoleto": "https://mockedlink.mock",
				"cCodStatus": "0",
				"cDesStatus": "mock desc status",
				"dDtEmBol": "01/01/2023",
				"cNumBoleto": "123",
				"cCodBarras": "123456789",
				"nPerJuros": 15,
				"nPerMulta": 15,
				"cNumBancario": "1445",
				"dDescontoCond1": "01/02/2023",
				"vDescontoCond1": 3500,
				"dDescontoCond2": "01/03/2023",
				"vDescontoCond2": 3900,
				"dDescontoCond3": "01/04/2023",
				"vDescontoCond3": 4500
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

	// test IncluirContato
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

	// Test IncluirOportunidade
	callAction = assets.ExternalServiceCallAction{Name: "IncluirOportunidade", Value: "IncluirOportunidade"}

	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie IncluirOportunidade: mocked error")
	assert.Nil(t, call)

	jsonParams = `[
		{"type": "identificacao", "filter": {"value": {"name":"cCodIntOp"}}, "data": {"value": "123321"}},
		{"type": "identificacao", "filter": {"value": {"name":"cDesOp"}}, "data": {"value": "mock descr"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodConta"}}, "data": {"value": "1"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodContato"}}, "data": {"value": "38511295"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodOrigem"}}, "data": {"value": "1208177"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodSolucao"}}, "data": {"value": "1208182"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodVendedor"}}, "data": {"value": "1"}}
	]`

	ps = []assets.ExternalServiceParam{}
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)

	// Test ListarClientes
	callAction = assets.ExternalServiceCallAction{Name: "ListarClientes", Value: "ListarClientes"}

	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie ListarClientes: mocked error")
	assert.Nil(t, call)

	jsonParams = `[
		{"type": "pagina", "data": {"value": "1"}},
		{"type": "registros_por_pagina", "data": {"value": "50"}},
		{"type": "apenas_importado_api", "data": {"value": "N"}}
	]`

	ps = []assets.ExternalServiceParam{}
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)

	// Test PesquisarLancamentos
	callAction = assets.ExternalServiceCallAction{Name: "PesquisarLancamentos", Value: "PesquisarLancamentos"}

	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie PesquisarLancamentos: mocked error")
	assert.Nil(t, call)

	jsonParams = `[
		{"type": "nPagina", "data": {"value": "1"}},
		{"type": "nRegPorPagina", "data": {"value": "20"}}
	]`

	ps = []assets.ExternalServiceParam{}
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)

	// Test ObterBoleto
	callAction = assets.ExternalServiceCallAction{Name: "ObterBoleto", Value: "ObterBoleto"}

	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie ObterBoleto: mocked error")
	assert.Nil(t, call)

	jsonParams = `[
		{"type": "nCodTitulo", "data": {"value": "1"}},
		{"type": "cCodIntTitulo", "data": {"value": "123"}}
	]`

	ps = []assets.ExternalServiceParam{}
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)

	// Test VerificarContato
	callAction = assets.ExternalServiceCallAction{Name: "VerificarContato", Value: "VerificarContato"}

	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call omie VerificarContato: mocked error")
	assert.Nil(t, call)

	jsonParams = `[{"type": "cEmail", "data": {"value": "cont1@email.com"}}]`

	ps = []assets.ExternalServiceParam{}
	err = json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	_, err = svc.Call(session, callAction, ps, logger.Log)
	assert.NoError(t, err)
}

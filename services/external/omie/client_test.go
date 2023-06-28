package omie_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/external/omie"
	"github.com/stretchr/testify/assert"
)

const (
	baseURL   = "https://omie.com.br"
	appKey    = "APP_KEY"
	appSecret = "APP_SECRET"
)

func TestRequest(t *testing.T) {
	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)

	response := new(interface{})

	// should return error because body is a anon function
	_, err := client.Request("GET", "", nil, func() {}, response)
	assert.Error(t, err)

	// should return error because method is invalid
	_, err = client.Request("{[:INVALID:]}", "", nil, nil, response)
	assert.Error(t, err)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/crm/contatos/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCod": 5491077640,
				"cCodInt": "12344321",
				"cCodStatus": "0",
				"cDesStatus": "Contato cadastrado com sucesso!"
			}`),
			httpx.NewMockResponse(200, nil, `{`),
		},
		fmt.Sprintf("%s/v1/crm/contatos/?param1=vparam1", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"nCod": 5491077640,
				"cCodInt": "12344321",
				"cCodStatus": "0",
				"cDesStatus": "Contato cadastrado com sucesso!"
			}`),
		},
	}))

	// as mock response returns error should error
	_, err = client.Request("GET", fmt.Sprintf("%s/v1/crm/contatos/", baseURL), nil, nil, nil)
	assert.Error(t, err)

	// should return error nil because expected response is nil and doesn't need to try unmarshal response body
	_, err = client.Request("GET", fmt.Sprintf("%s/v1/crm/contatos/", baseURL), nil, nil, nil)
	assert.Nil(t, err)

	// as mock response returns invalid json then error
	_, err = client.Request("GET", fmt.Sprintf("%s/v1/crm/contatos/", baseURL), nil, nil, response)
	assert.Error(t, err)
	assert.EqualError(t, err, "couldn't parse response body: unexpected end of JSON input")

	params := &url.Values{"param1": {"vparam1"}}
	_, err = client.Request("GET", fmt.Sprintf("%s/v1/crm/contatos/", baseURL), params, nil, nil)
	assert.Nil(t, err)
}

func TestIncluirContato(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/crm/contatos/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
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

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.IncluirContatoRequest{}

	paramJSON := `[{"identificacao":{"cCodInt":"12344321","cNome":"nome12134","cSobrenome":"","cCargo":"","dDtNasc":"","nCodVend":0,"nCodConta":0},"endereco":{"cEndereco":"","cCompl":"","cCEP":"","cBairro":"","cCidade":"","cUF":"","cPais":""},"telefone_email":{"cDDDCel1":"","cNumCel1":"","cEmail":"","cWebsite":""}}]`
	var param []omie.IncluirContatoRequestParam

	err := json.Unmarshal([]byte(paramJSON), &param)
	assert.NoError(t, err)
	data.Param = param

	_, _, err = client.IncluirContato(data)
	assert.EqualError(t, err, "error")

	op, trace, err := client.IncluirContato(data)
	assert.NoError(t, err)
	assert.Equal(t, int64(5491077640), op.NCod)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 135\r\n\r\n", string(trace.ResponseTrace))
}

func TestIncluirOportunidade(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/crm/oportunidades/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCodOp": 5460847117,
				"cCodIntOp": "e8f8b762-24c1-4adf-7",
				"cCodStatus": "0",
				"cDesStatus": "Oportunidade cadastrada com sucesso!"
			}`),
		},
	}))

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.IncluirOportunidadeRequest{Param: []omie.IncluirOportunidadeRequestParam{}}

	param := omie.IncluirOportunidadeRequestParam{
		Identificacao: omie.OpIdentificacao{
			CCodIntOp: "e8f8b762-24c1-4adf-7",
		},
	}

	data.Param = append(data.Param, param)

	_, _, err := client.IncluirOportunidade(data)
	assert.EqualError(t, err, "error")

	op, trace, err := client.IncluirOportunidade(data)
	assert.NoError(t, err)
	assert.Equal(t, "e8f8b762-24c1-4adf-7", op.CCodIntOp)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 153\r\n\r\n", string(trace.ResponseTrace))
}

func TestListarClientes(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/geral/clientes/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"pagina": 1,
				"total_de_paginas": 1,
				"registros": 1,
				"total_de_registros": 1,
				"clientes_cadastro": [
					{
						"bairro": "SETOR 01",
						"bloquear_faturamento": "N",
						"cep": "76870175",
						"cidade": "ARIQUEMES (RO)",
						"cidade_ibge": "1100023",
						"cnpj_cpf": "02.923.710/0001-19",
						"codigo_cliente_integracao": "934690",
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
						"email": "bertieberti@hotmail.com",
						"endereco": "AVENIDA JAMARI",
						"enderecoEntrega": {},
						"endereco_numero": "2007",
						"estado": "RO",
						"exterior": "N",
						"inativo": "N",
						"info": {
							"cImpAPI": "S",
							"dAlt": "16/12/2022",
							"dInc": "07/04/2014",
							"hAlt": "08:11:04",
							"hInc": "10:56:29",
							"uAlt": "WEBSERVICE",
							"uInc": "WEBSERVICE"
						},
						"inscricao_estadual": "ISENTO",
						"inscricao_municipal": "",
						"nome_fantasia": "BERTI BERTI LTDA",
						"optante_simples_nacional": "S",
						"pessoa_fisica": "N",
						"produtor_rural": "N",
						"razao_social": "BERTI BERTI LTDA",
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
								"tag": "Transportadora"
							},
							{
								"tag": "Aluga"
							},
							{
								"tag": "Tag 08092022"
							},
							{
								"tag": "Tag 08092022 2"
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
	}))

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.ListarClientesRequest{}

	_, _, err := client.ListarClientes(data)
	assert.EqualError(t, err, "error")

	cls, trace, err := client.ListarClientes(data)
	assert.NoError(t, err)
	assert.Equal(t, 1, cls.TotalDeRegistros)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 1964\r\n\r\n", string(trace.ResponseTrace))
}

func TestPesquisarLancamentos(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/financas/pesquisartitulos/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
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
							"cTipo": "DIN",
							"dDtEmissao": "19/02/2014",
							"dDtPagamento": "07/02/2023",
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
							"nValorISS": 0,
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
								"cCodIntLanc": "574786335674895",
								"cNatureza": "R",
								"cObsLanc": "cardoso",
								"dDtLanc": "07/02/2023",
								"nCodCC": 4243124,
								"nCodLanc": 5460509034,
								"nDesconto": 0,
								"nIdLancCC": 5460509033,
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
	}))

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.PesquisarLancamentosRequest{}

	_, _, err := client.PesquisarLancamentos(data)
	assert.EqualError(t, err, "error")

	l, trace, err := client.PesquisarLancamentos(data)
	assert.NoError(t, err)
	assert.Equal(t, 1, l.NTotRegistros)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 2098\r\n\r\n", string(trace.ResponseTrace))
}

func TestVerificarContato(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/crm/contatos/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"nCod": 1,
				"cCodInt": "whatsapp:100000",
				"cCodStatus": "0",
				"nTotRegistros": 1,
				"cDesStatus": "Contato encontrado com sucesso!"
			}`),
		},
	}))

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.VerificarContatoRequest{}

	_, _, err := client.VerificarContato(data)
	assert.EqualError(t, err, "error")

	c, trace, err := client.VerificarContato(data)
	assert.NoError(t, err)
	assert.Equal(t, "Contato encontrado com sucesso!", c.CDesStatus)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 154\r\n\r\n", string(trace.ResponseTrace))
}

func TestObterBoleto(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v1/financas/contareceberboleto/", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"faultstring": "error",
				"faultcode": "ERROR-CODE 123"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"cLinkBoleto": "https://foo.boleto",
				"cCodStatus": "0",
				"cDesStatus": "status",
				"dDtEmBol": "07/02/2023",
				"cNumBoleto": "12345",
				"cCodBarras": "123456789",
				"nPerJuros": 0.5,
				"nPerMulta": 0.2,
				"cNumBancario": "0000",
				"dDescontoCond1": "02/03/2023",
				"vDescontoCond1": 0.3,
				"dDescontoCond2": "01/04/2023",
				"vDescontoCond2": 0.6,
				"dDescontoCond3": "03/05/2023",
				"vDescontoCond3": 0.8
			}`),
		},
	}))

	client := omie.NewClient(http.DefaultClient, nil, baseURL, appKey, appSecret)
	data := &omie.ObterBoletoRequest{}

	_, _, err := client.ObterBoleto(data)
	assert.EqualError(t, err, "error")

	b, trace, err := client.ObterBoleto(data)
	assert.NoError(t, err)
	assert.Equal(t, "https://foo.boleto", b.CLinkBoleto)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 446\r\n\r\n", string(trace.ResponseTrace))
}

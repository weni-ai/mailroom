package omie

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/goflow/assets"
	"github.com/stretchr/testify/assert"
)

func TestParamsToIncluirContatoRequest(t *testing.T) {

	jsonParams := `[
		{"type": "identificacao", "filter": {"value": {"name":"nCod"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"cCodInt"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"cNome"}}, "data": {"value": "contact123"}},
		{"type": "identificacao", "filter": {"value": {"name":"cSobrenome"}}, "data": {"value": "contact123"}},
		{"type": "identificacao", "filter": {"value": {"name":"cCargo"}}, "data": {"value": "none"}},
		{"type": "identificacao", "filter": {"value": {"name":"dDtNasc"}}, "data": {"value": "01/01/1981"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodVend"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodConta"}}, "data": {"value": "123"}},

		{"type": "endereco", "filter": {"value": {"name":"cEndereco"}}, "data": {"value": "mock end"}},
		{"type": "endereco", "filter": {"value": {"name":"cCompl"}}, "data": {"value": "mock comp"}},
		{"type": "endereco", "filter": {"value": {"name":"cCEP"}}, "data": {"value": "mock cep"}},
		{"type": "endereco", "filter": {"value": {"name":"cBairro"}}, "data": {"value": "mock bairro"}},
		{"type": "endereco", "filter": {"value": {"name":"cCidade"}}, "data": {"value": "mock cid"}},
		{"type": "endereco", "filter": {"value": {"name":"cUF"}}, "data": {"value": "mock uf"}},
		{"type": "endereco", "filter": {"value": {"name":"cPais"}}, "data": {"value": "mock pais"}},

		{"type": "telefone_email", "filter": {"value": {"name":"cDDDCel1"}}, "data": {"value": "mock dddcel1"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cNumCel1"}}, "data": {"value": "mock cel1"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cDDDCel2"}}, "data": {"value": "mock dddcel2"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cNumCel2"}}, "data": {"value": "mock cel2"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cDDDTel"}}, "data": {"value": "mock dddtel"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cNumTel"}}, "data": {"value": "mock tel"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cDDDFax"}}, "data": {"value": "mock dddfax"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cNumFax"}}, "data": {"value": "mock numfax"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cEmail"}}, "data": {"value": "mock email"}},
		{"type": "telefone_email", "filter": {"value": {"name":"cWebsite"}}, "data": {"value": "mock wewbsite"}},

		{"type": "cObs", "data": {"value": "123"}}
	]`
	var ps []assets.ExternalServiceParam
	err := json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	r, err := ParamsToIncluirContatoRequest(ps)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].Identificacao.NCod)
	assert.Equal(t, "123", r.Param[0].Identificacao.CCodInt)
	assert.Equal(t, "contact123", r.Param[0].Identificacao.CNome)
	assert.Equal(t, "contact123", r.Param[0].Identificacao.CSobrenome)
	assert.Equal(t, "none", r.Param[0].Identificacao.CCargo)
	assert.Equal(t, "01/01/1981", r.Param[0].Identificacao.DDtNasc)
	assert.Equal(t, 123, r.Param[0].Identificacao.NCodVend)
	assert.Equal(t, 123, r.Param[0].Identificacao.NCodConta)

	assert.Equal(t, "mock end", r.Param[0].Endereco.CEndereco)
	assert.Equal(t, "mock comp", r.Param[0].Endereco.CCompl)
	assert.Equal(t, "mock cep", r.Param[0].Endereco.CCEP)
	assert.Equal(t, "mock bairro", r.Param[0].Endereco.CBairro)
	assert.Equal(t, "mock cid", r.Param[0].Endereco.CCidade)
	assert.Equal(t, "mock uf", r.Param[0].Endereco.CUF)
	assert.Equal(t, "mock pais", r.Param[0].Endereco.CPais)
}

func TestParamsToIncluirOportunidadeRequest(t *testing.T) {

	jsonParams := `[
		{"type": "identificacao", "filter": {"value": {"name":"cCodIntOp"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"cDesOp"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodConta"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodContato"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodOp"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodOrigem"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodSolucao"}}, "data": {"value": "123"}},
		{"type": "identificacao", "filter": {"value": {"name":"nCodVendedor"}}, "data": {"value": "123"}},

		{"type": "fasesStatus", "filter": {"value": {"name":"dConclusao"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"dNovoLead"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"dProjeto"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"dQualificacao"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"dShowRoom"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"dTreinamento"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"nCodFase"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"nCodMotivo"}}, "data": {"value": "123"}},
		{"type": "fasesStatus", "filter": {"value": {"name":"nCodStatus"}}, "data": {"value": "123"}},

		{"type": "ticket", "filter": {"value": {"name":"nMeses"}}, "data": {"value": "1"}},
		{"type": "ticket", "filter": {"value": {"name":"nProdutos"}}, "data": {"value": "1"}},
		{"type": "ticket", "filter": {"value": {"name":"nRecorrencia"}}, "data": {"value": "1"}},
		{"type": "ticket", "filter": {"value": {"name":"nServicos"}}, "data": {"value": "1"}},
		{"type": "ticket", "filter": {"value": {"name":"nTicket"}}, "data": {"value": "1"}},

		{"type": "previsaoTemp", "filter": {"value": {"name":"nAnoPrev"}}, "data": {"value": "1"}},
		{"type": "previsaoTemp", "filter": {"value": {"name":"nMesPrev"}}, "data": {"value": "1"}},
		{"type": "previsaoTemp", "filter": {"value": {"name":"nTemperatura"}}, "data": {"value": "1"}},

		{"type": "observacoes", "data": {"value": "mock text"}},

		{"type": "outrasInf", "filter": {"value": {"name":"cEmailOp"}}, "data": {"value": "mock@email.com"}},
		{"type": "outrasInf", "filter": {"value": {"name":"dAlteracao"}}, "data": {"value": "01/02/2023"}},
		{"type": "outrasInf", "filter": {"value": {"name":"dInclusao"}}, "data": {"value": "01/02/2023"}},
		{"type": "outrasInf", "filter": {"value": {"name":"hAlteracao"}}, "data": {"value": "00:00"}},
		{"type": "outrasInf", "filter": {"value": {"name":"hInclusao"}}, "data": {"value": "00:00"}},
		{"type": "outrasInf", "filter": {"value": {"name":"nCodTipo"}}, "data": {"value": "1"}},

		{"type": "envolvidos", "filter": {"value": {"name":"nCodFinder"}}, "data": {"value": "12"}},
		{"type": "envolvidos", "filter": {"value": {"name":"nCodParceiro"}}, "data": {"value": "12"}},
		{"type": "envolvidos", "filter": {"value": {"name":"nCodPrevenda"}}, "data": {"value": "12"}}
	]
	`
	var ps []assets.ExternalServiceParam
	err := json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	r, err := ParamsToIncluirOportunidadeRequest(ps)
	assert.NoError(t, err)
	assert.Equal(t, "123", r.Param[0].Identificacao.CCodIntOp)
}

func TestParamsToListarClientesRequest(t *testing.T) {

	jsonParams := `[
		{"type": "pagina", "data": {"value": "1"}},
		{"type": "registros_por_pagina", "data": {"value": "50"}},
		{"type": "apenas_importado_api", "data": {"value": "50"}},
		{"type": "ordenar_por", "data": {"value": "mockOrder"}},
		{"type": "ordem_decrescente", "data": {"value": "false"}},
		{"type": "filtrar_por_data_de", "data": {"value": "01/01/2023"}},
		{"type": "filtrar_por_data_ate", "data": {"value": "30/12/2023"}},
		{"type": "filtrar_por_hora_de", "data": {"value": "00:00:00"}},
		{"type": "filtrar_por_hora_ate", "data": {"value": "23:59:59"}},
		{"type": "filtrar_apenas_inclusao", "data": {"value": "false"}},
		{"type": "filtrar_apenas_alteracao", "data": {"value": "false"}},

		{"type": "clientesFiltro", "filter": {"value": {"name":"codigo_cliente_omie"}}, "data": {"value": "123"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"codigo_cliente_integracao"}}, "data": {"value": "123"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"cnpj_cpf"}}, "data": {"value": "12345678910"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"razao_social"}}, "data": {"value": "mock razao"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"nome_fantasia"}}, "data": {"value": "mock nome_fantasia"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"endereco"}}, "data": {"value": "mock endereco"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"bairro"}}, "data": {"value": "mock bairro"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"cidade"}}, "data": {"value": "mock cidade"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"estado"}}, "data": {"value": "mock estado"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"cep"}}, "data": {"value": "mock cep"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"contato"}}, "data": {"value": "mock contato"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"email"}}, "data": {"value": "mock@email.com"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"homepage"}}, "data": {"value": "mockhomepage.mock"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"inscricao_municipal"}}, "data": {"value": "mock inscricao_municipal"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"inscricao_estadual"}}, "data": {"value": "mock inscricao_estadual"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"inscricao_suframa"}}, "data": {"value": "mock inscricao_suframa"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"pessoa_fisica"}}, "data": {"value": "mock pessoa_fisica"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"optante_simples_nacional"}}, "data": {"value": "mock optante"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"inativo"}}, "data": {"value": "false"}},
		{"type": "clientesFiltro", "filter": {"value": {"name":"tags"}}, "data": {"value": "mocktag"}},

		{"type": "clientesPorCodigo", "filter": {"value": {"name":"codigo_cliente_omie"}}, "data": {"value": "123"}},
		{"type": "clientesPorCodigo", "filter": {"value": {"name":"codigo_cliente_integracao"}}, "data": {"value": "mock-cod-int"}},

		{"type": "exibir_caracteristicas", "data": {"value": "false"}}
	]`
	var ps []assets.ExternalServiceParam
	err := json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	r, err := ParamsToListarClientesRequest(ps)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].ClientesPorCodigo.CodigoClienteOmie)
}

func TestParamsToPesquisarLancamentosRequest(t *testing.T) {

	jsonParams := `[
		{"type": "nPagina", "data": {"value": "1"}},
		{"type": "nRegPorPagina", "data": {"value": "50"}},
		{"type": "cOrdenarPor", "data": {"value": "mockOrder"}},
		{"type": "cOrdemDecrescente", "data": {"value": "true"}},
		{"type": "nCodTitulo", "data": {"value": "123"}},
		{"type": "cCodIntTitulo", "data": {"value": "123"}},
		{"type": "cNumTitulo", "data": {"value": "123"}},
		{"type": "dDtEmisDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtEmisAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtVencDe", "data": {"value": "30/01/2023"}},
		{"type": "dDtVencAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtPagtoDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtPagtoAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtPrevDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtPrevAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtRegDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtRegAte", "data": {"value": "30/12/2023"}},
		{"type": "nCodCliente", "data": {"value": "123"}},
		{"type": "cCPFCNPJCliente", "data": {"value": "123456789"}},
		{"type": "nCodCtr", "data": {"value": "123"}},
		{"type": "cNumCtr", "data": {"value": "123"}},
		{"type": "nCodOS", "data": {"value": "123"}},
		{"type": "cNumOS", "data": {"value": "123"}},
		{"type": "nCodCC", "data": {"value": "123"}},
		{"type": "cStatus", "data": {"value": "123"}},
		{"type": "cNatureza", "data": {"value": "123"}},
		{"type": "cTipo", "data": {"value": "123"}},
		{"type": "cOperacao", "data": {"value": "123"}},
		{"type": "cNumDocFiscal", "data": {"value": "123"}},
		{"type": "cCodigoBarras", "data": {"value": "123"}},
		{"type": "nCodProjeto", "data": {"value": "123"}},
		{"type": "nCodVendedor", "data": {"value": "123"}},
		{"type": "nCodComprador", "data": {"value": "123"}},
		{"type": "cCodCateg", "data": {"value": "123"}},
		{"type": "dDtIncDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtIncAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtAltDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtAltAte", "data": {"value": "30/12/2023"}},
		{"type": "dDtCancDe", "data": {"value": "01/01/2023"}},
		{"type": "dDtCancAte", "data": {"value": "30/12/2023"}},
		{"type": "cChaveNFe", "data": {"value": "123456789"}}
	]`

	var ps []assets.ExternalServiceParam
	err := json.Unmarshal([]byte(jsonParams), &ps)
	assert.NoError(t, err)

	r, err := ParamsToPesquisarLancamentosRequest(ps)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].NCodTitulo)
}

func TestParamsToVerificarContatoRequest(t *testing.T) {
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"foo",
		"",
		"string",
		"",
		"cNome",
		"cNome",
	)

	pm2 := assets.NewExternalServiceParam(
		"foo@bar.com",
		"",
		"string",
		"",
		"cEmail",
		"cEmail",
	)

	params = append(params, *pm)
	params = append(params, *pm2)

	r, err := ParamsToVerificarContatoRequest(params)
	assert.NoError(t, err)
	assert.Equal(t, "foo", r.Param[0].CNome)
	assert.Equal(t, "foo@bar.com", r.Param[0].CEmail)
}

func TestParamsToObterBoletoRequest(t *testing.T) {
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"123",
		"",
		"string",
		"",
		"nCodTitulo",
		"nCodTitulo",
	)

	pm2 := assets.NewExternalServiceParam(
		"123",
		"",
		"string",
		"",
		"cCodIntTitulo",
		"cCodIntTitulo",
	)

	params = append(params, *pm)
	params = append(params, *pm2)

	r, err := ParamsToObterBoletoRequest(params)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].NCodTitulo)
	assert.Equal(t, "123", r.Param[0].CCodIntTitulo)
}

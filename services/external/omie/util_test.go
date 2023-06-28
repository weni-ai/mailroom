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
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"123",
		"cCodIntOp",
		"string",
		"cCodIntOp",
		"identificacao",
		"identificacao",
	)

	params = append(params, *pm)

	r, err := ParamsToIncluirOportunidadeRequest(params)
	assert.NoError(t, err)
	assert.Equal(t, "123", r.Param[0].Identificacao.CCodIntOp)
}

func TestParamsToListarClientesRequest(t *testing.T) {
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"123",
		"codigo_cliente_omie",
		"string",
		"codigo_cliente_omie",
		"clientesPorCodigo",
		"clientesPorCodigo",
	)

	params = append(params, *pm)

	r, err := ParamsToListarClientesRequest(params)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].ClientesPorCodigo.CodigoClienteOmie)
}

func TestParamsToPesquisarLancamentosRequest(t *testing.T) {
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"123",
		"",
		"string",
		"",
		"nCodTitulo",
		"nCodTitulo",
	)

	params = append(params, *pm)

	r, err := ParamsToPesquisarLancamentosRequest(params)
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

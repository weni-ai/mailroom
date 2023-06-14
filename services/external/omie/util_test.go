package omie

import (
	"testing"

	"github.com/nyaruka/goflow/assets"
	"github.com/stretchr/testify/assert"
)

func TestParamsToIncluirContatoRequest(t *testing.T) {
	params := []assets.ExternalServiceParam{}

	pm := assets.NewExternalServiceParam(
		"123",
		"nCod",
		"string",
		"nCod",
		"identificacao",
		"identificacao",
	)

	params = append(params, *pm)

	r, err := ParamsToIncluirContatoRequest(params)
	assert.NoError(t, err)
	assert.Equal(t, 123, r.Param[0].Identificacao.NCod)
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

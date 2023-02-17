package omie

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

type baseClient struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
	appKey      string
	appSecret   string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, appKey, appSecret string) baseClient {
	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		appKey:      appKey,
		appSecret:   appSecret,
		baseURL:     baseURL,
	}
}

type Client struct {
	baseClient
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, appKey, appSecret string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, baseURL, appKey, appSecret),
	}
}

type errorResponse struct {
	Faultstring string `json:"faultstring"`
	Faultcode   string `json:"faultcode"`
}

func (c *baseClient) request(method, url string, params *url.Values, body, response interface{}) (*httpx.Trace, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	data := strings.NewReader(string(b))
	req, err := httpx.NewRequest(method, url, data, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	if params != nil {
		req.URL.RawQuery = params.Encode()
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return trace, errors.New(response.Faultstring)
	}

	if response != nil {
		err = json.Unmarshal(trace.ResponseBody, response)
		return trace, errors.Wrap(err, "couldn't parse response body")
	}

	return trace, nil
}

func (c *baseClient) post(url string, params *url.Values, payload, response interface{}) (*httpx.Trace, error) {
	return c.request("POST", url, params, payload, response)
}

func (c *Client) IncluirContato(data *IncluirContatoRequest) (*IncluirContatoResponse, *httpx.Trace, error) {
	requestUrl := c.baseURL + "/v1/crm/contatos/"
	response := &IncluirContatoResponse{}

	data.Call = "IncluirContato"
	data.AppKey = c.appKey
	data.AppSecret = c.appSecret

	trace, err := c.post(requestUrl, nil, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) IncluirOportunidade(data *IncluirOportunidadeRequest) (*OpIncluirResponse, *httpx.Trace, error) {
	requestUrl := c.baseURL + "/v1/crm/oportunidades/"
	response := &OpIncluirResponse{}

	data.Call = "IncluirOportunidade"
	data.AppKey = c.appKey
	data.AppSecret = c.appSecret

	trace, err := c.post(requestUrl, nil, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) ListarClientes(data *ListarClientesRequest) (*ListarClientesResponse, *httpx.Trace, error) {
	requestUrl := c.baseURL + "/v1/geral/clientes/"
	response := &ListarClientesResponse{}

	data.Call = "ListarClientes"
	data.AppKey = c.appKey
	data.AppSecret = c.appSecret

	trace, err := c.post(requestUrl, nil, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) PesquisarLancamentos(data *PesquisarLancamentosRequest) (*PesquisarLancamentosResponse, *httpx.Trace, error) {
	requestUrl := c.baseURL + "/v1/financas/pesquisartitulos/"
	response := &PesquisarLancamentosResponse{}

	data.Call = "PesquisarLancamentosRequest"
	data.AppKey = c.appKey
	data.AppSecret = c.appSecret

	trace, err := c.post(requestUrl, nil, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

type OmieCall struct {
	Call      string `json:"call"`
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
}

type ListResponse struct {
	Pagina           int `json:"pagina"`
	TotalDePaginas   int `json:"total_de_paginas"`
	Registros        int `json:"registros"`
	TotalDeRegistros int `json:"total_de_registros"`
}

type IncluirContatoResponse struct {
	NCod       int64  `json:"nCod"`
	CCodInt    string `json:"cCodInt"`
	CCodStatus string `json:"cCodStatus"`
	CDesStatus string `json:"cDesStatus"`
}

type IncluirContatoRequest struct {
	OmieCall
	Param []struct {
		Identificacao struct {
			NCod       int    `json:"nCod"`
			CCodInt    string `json:"cCodInt"`
			CNome      string `json:"cNome"`
			CSobrenome string `json:"cSobrenome"`
			CCargo     string `json:"cCargo"`
			DDtNasc    string `json:"dDtNasc"`
			NCodVend   int    `json:"nCodVend"`
			NCodConta  int    `json:"nCodConta"`
		} `json:"identificacao"`
		Endereco struct {
			CEndereco string `json:"cEndereco"`
			CCompl    string `json:"cCompl"`
			CCEP      string `json:"cCEP"`
			CBairro   string `json:"cBairro"`
			CCidade   string `json:"cCidade"`
			CUF       string `json:"cUF"`
			CPais     string `json:"cPais"`
		} `json:"endereco"`
		TelefoneEmail struct {
			CDDDCel1 string `json:"cDDDCel1"`
			CNumCel1 string `json:"cNumCel1"`
			CDDDCel2 string `json:"cDDDCel2"`
			CNumCel2 string `json:"cNumCel2"`
			CDDDTel  string `json:"cDDDTel"`
			CNumTel  string `json:"cNumTel"`
			CDDDFax  string `json:"cDDDFax"`
			CNumFax  string `json:"cNumFax"`
			CEmail   string `json:"cEmail"`
			CWebsite string `json:"cWebsite"`
		} `json:"telefone_email"`
		CObs string `json:"cObs"`
	} `json:"param"`
}

type OpIncluirResponse struct {
	NCodOp     int64  `json:"nCodOp"`
	CCodIntOp  string `json:"cCodIntOp"`
	CCodStatus string `json:"cCodStatus"`
	CDesStatus string `json:"cDesStatus"`
}

type IncluirOportunidadeRequest struct {
	OmieCall
	Param []OpIncluirRequest
}

type OpIncluirRequest struct {
	Identificacao OpIdentificacao `json:"identificacao"`
	FasesStatus   OpFasesStatus   `json:"fasesStatus"`
	Ticket        OpTicket        `json:"ticket"`
	PrevisaoTemp  OpPrevisaoTemp  `json:"previsaoTemp"`
	Observacoes   OpObservacoes   `json:"observacoes"`
	OutraInf      OpOutrasInf     `json:"outrasInf"`
	Envolvidos    OpEnvolvidos    `json:"envolvidos"`
	Concorrentes  []interface{}   `json:"concorrentes"`
}

type OpIdentificacao struct {
	CCodIntOp    string `json:"cCodIntOp"`
	CDesOp       string `json:"cDesOp"`
	NCodConta    int    `json:"nCodConta"`
	NCodContato  int    `json:"nCodContato"`
	NCodOp       int64  `json:"nCodOp"`
	NCodOrigem   int    `json:"nCodOrigem"`
	NCodSolucao  int    `json:"nCodSolucao"`
	NCodVendedor int    `json:"nCodVendedor"`
}

type OpFasesStatus struct {
	DConclusao    string `json:"dConclusao"`
	DNovoLead     string `json:"dNovoLead"`
	DProjeto      string `json:"dProjeto"`
	DQualificacao string `json:"dQualificacao"`
	DShowRoom     string `json:"dShowRoom"`
	DTreinamento  string `json:"dTreinamento"`
	NCodFase      int    `json:"nCodFase"`
	NCodMotivo    int    `json:"nCodMotivo"`
	NCodStatus    int    `json:"nCodStatus"`
}
type OpTicket struct {
	NMeses       int `json:"nMeses"`
	NProdutos    int `json:"nProdutos"`
	NRecorrencia int `json:"nRecorrencia"`
	NServicos    int `json:"nServicos"`
	NTicket      int `json:"nTicket"`
}

type OpPrevisaoTemp struct {
	NAnoPrev     int `json:"nAnoPrev"`
	NMesPrev     int `json:"nMesPrev"`
	NTemperatura int `json:"nTemperatura"`
}

type OpObservacoes struct {
	CObs string `json:"cObs"`
}

type OpOutrasInf struct {
	CEmailOp   string `json:"cEmailOp"`
	DAlteracao string `json:"dAlteracao"`
	DInclusao  string `json:"dInclusao"`
	HAlteracao string `json:"hAlteracao"`
	HInclusao  string `json:"hInclusao"`
	NCodTipo   int    `json:"nCodTipo"`
}

type OpEnvolvidos struct {
	NCodFinder   int `json:"nCodFinder"`
	NCodParceiro int `json:"nCodParceiro"`
	NCodPrevenda int `json:"nCodPrevenda"`
}

type ListarClientesResponse struct {
	Pagina           int               `json:"pagina"`
	TotalDePaginas   int               `json:"total_de_paginas"`
	Registros        int               `json:"registros"`
	TotalDeRegistros int               `json:"total_de_registros"`
	ClientesCadastro []ClienteCadastro `json:"clientes_cadastro"`
}

type ClienteCadastro struct {
	Bairro                  string `json:"bairro"`
	BloquearExclusao        string `json:"bloquear_exclusao"`
	BloquearFaturamento     string `json:"bloquear_faturamento"`
	Cep                     string `json:"cep"`
	Cidade                  string `json:"cidade"`
	CidadeIbge              string `json:"cidade_ibge"`
	CnpjCpf                 string `json:"cnpj_cpf"`
	CodigoClienteIntegracao string `json:"codigo_cliente_integracao"`
	CodigoClienteOmie       int64  `json:"codigo_cliente_omie"`
	CodigoPais              string `json:"codigo_pais"`
	Complemento             string `json:"complemento"`
	DadosBancarios          struct {
		Agencia       string `json:"agencia"`
		CodigoBanco   string `json:"codigo_banco"`
		ContaCorrente string `json:"conta_corrente"`
		DocTitular    string `json:"doc_titular"`
		NomeTitular   string `json:"nome_titular"`
		TransfPadrao  string `json:"transf_padrao"`
	} `json:"dadosBancarios"`
	Email           string `json:"email"`
	Endereco        string `json:"endereco"`
	EnderecoEntrega struct {
	} `json:"enderecoEntrega"`
	EnderecoNumero string `json:"endereco_numero"`
	Estado         string `json:"estado"`
	Exterior       string `json:"exterior"`
	Inativo        string `json:"inativo"`
	Info           struct {
		CImpAPI string `json:"cImpAPI"`
		DAlt    string `json:"dAlt"`
		DInc    string `json:"dInc"`
		HAlt    string `json:"hAlt"`
		HInc    string `json:"hInc"`
		UAlt    string `json:"uAlt"`
		UInc    string `json:"uInc"`
	} `json:"info"`
	InscricaoEstadual  string `json:"inscricao_estadual"`
	InscricaoMunicipal string `json:"inscricao_municipal"`
	NomeFantasia       string `json:"nome_fantasia"`
	PessoaFisica       string `json:"pessoa_fisica"`
	RazaoSocial        string `json:"razao_social"`
	Recomendacoes      struct {
		GerarBoletos string `json:"gerar_boletos"`
	} `json:"recomendacoes"`
	Tags            []interface{} `json:"tags"`
	Telefone1Ddd    string        `json:"telefone1_ddd"`
	Telefone1Numero string        `json:"telefone1_numero"`
}

type ListarClientesRequest struct {
	OmieCall
	Param []ListarClientesRequestParam `json:"param"`
}

type ListarClientesRequestParam struct {
	Pagina                 int    `json:"pagina"`
	RegistrosPorPagina     int    `json:"registros_por_pagina"`
	ApenasImportadoAPI     string `json:"apenas_importado_api"`
	OrdenarPor             string `json:"ordenar_por"`
	OrdemDecrescente       string `json:"ordem_decrescente"`
	FiltrarPorDataDe       string `json:"filtrar_por_data_de"`
	FiltrarPorDataAte      string `json:"filtrar_por_data_ate"`
	FiltrarPorHoraDe       string `json:"filtrar_por_hora_de"`
	FiltrarPorHoraAte      string `json:"filtrar_por_hora_ate"`
	FiltrarApenasInclusao  string `json:"filtrar_apenas_inclusao"`
	FiltrarApenasAlteracao string `json:"filtrar_apenas_alteracao"`
	ClientesFiltro         struct {
		CodigoClienteOmie       int    `json:"codigo_cliente_omie"`
		CodigoClienteIntegracao string `json:"codigo_cliente_integracao"`
		CnpjCpf                 string `json:"cnpj_cpf"`
		RazaoSocial             string `json:"razao_social"`
		NomeFantasia            string `json:"nome_fantasia"`
		Endereco                string `json:"endereco"`
		Bairro                  string `json:"bairro"`
		Cidade                  string `json:"cidade"`
		Estado                  string `json:"estado"`
		Cep                     string `json:"cep"`
		Contato                 string `json:"contato"`
		Email                   string `json:"email"`
		Homepage                string `json:"homepage"`
		InscricaoMunicipal      string `json:"inscricao_municipal"`
		InscricaoEstadual       string `json:"inscricao_estadual"`
		InscricaoSuframa        string `json:"inscricao_suframa"`
		PessoaFisica            string `json:"pessoa_fisica"`
		OptanteSimplesNacional  string `json:"optante_simples_nacional"`
		Inativo                 string `json:"inativo"`
		Tags                    string `json:"tags"`
	} `json:"clientesFiltro"`
	ClientesPorCodigo struct {
		CodigoClienteOmie       int    `json:"codigo_cliente_omie"`
		CodigoClienteIntegracao string `json:"codigo_cliente_integracao"`
	} `json:"clientesPorCodigo"`
	ExibirCaracteristicas string `json:"exibir_caracteristicas"`
}

type PesquisarLancamentosRequest struct {
	OmieCall
	Param []PesquisarLancamentosParam `json:"param"`
}

type PesquisarLancamentosParam struct {
	NPagina           int    `json:"nPagina"`
	NRegPorPagina     int    `json:"nRegPorPagina"`
	COrdenarPor       string `json:"cOrdenarPor"`
	COrdemDecrescente string `json:"cOrdemDecrescente"`
	NCodTitulo        int    `json:"nCodTitulo"`
	CCodIntTitulo     string `json:"cCodIntTitulo"`
	CNumTitulo        string `json:"cNumTitulo"`
	DDtEmisDe         string `json:"dDtEmisDe"`
	DDtEmisAte        string `json:"dDtEmisAte"`
	DDtVencDe         string `json:"dDtVencDe"`
	DDtVencAte        string `json:"dDtVencAte"`
	DDtPagtoDe        string `json:"dDtPagtoDe"`
	DDtPagtoAte       string `json:"dDtPagtoAte"`
	DDtPrevDe         string `json:"dDtPrevDe"`
	DDtPrevAte        string `json:"dDtPrevAte"`
	DDtRegDe          string `json:"dDtRegDe"`
	DDtRegAte         string `json:"dDtRegAte"`
	NCodCliente       int    `json:"nCodCliente"`
	CCPFCNPJCliente   string `json:"cCPFCNPJCliente"`
	NCodCtr           int    `json:"nCodCtr"`
	CNumCtr           string `json:"cNumCtr"`
	NCodOS            int    `json:"nCodOS"`
	CNumOS            string `json:"cNumOS"`
	NCodCC            int    `json:"nCodCC"`
	CStatus           string `json:"cStatus"`
	CNatureza         string `json:"cNatureza"`
	CTipo             string `json:"cTipo"`
	COperacao         string `json:"cOperacao"`
	CNumDocFiscal     string `json:"cNumDocFiscal"`
	CCodigoBarras     string `json:"cCodigoBarras"`
	NCodProjeto       int    `json:"nCodProjeto"`
	NCodVendedor      int    `json:"nCodVendedor"`
	NCodComprador     int    `json:"nCodComprador"`
	CCodCateg         string `json:"cCodCateg"`
	DDtIncDe          string `json:"dDtIncDe"`
	DDtIncAte         string `json:"dDtIncAte"`
	DDtAltDe          string `json:"dDtAltDe"`
	DDtAltAte         string `json:"dDtAltAte"`
	DDtCancDe         string `json:"dDtCancDe"`
	DDtCancAte        string `json:"dDtCancAte"`
	CChaveNFe         string `json:"cChaveNFe"`
}

type PesquisarLancamentosResponse struct {
	NPagina            int `json:"nPagina"`
	NTotPaginas        int `json:"nTotPaginas"`
	NRegistros         int `json:"nRegistros"`
	NTotRegistros      int `json:"nTotRegistros"`
	TitulosEncontrados []struct {
		CabecTitulo struct {
			ACodCateg []struct {
				CCodCateg string `json:"cCodCateg"`
				NPerc     int    `json:"nPerc"`
				NValor    int    `json:"nValor"`
			} `json:"aCodCateg"`
			CCPFCNPJCliente string `json:"cCPFCNPJCliente"`
			CCodCateg       string `json:"cCodCateg"`
			CCodIntTitulo   string `json:"cCodIntTitulo"`
			CCodVendedor    int    `json:"cCodVendedor"`
			CNSU            string `json:"cNSU"`
			CNatureza       string `json:"cNatureza"`
			CNumBoleto      string `json:"cNumBoleto"`
			CNumDocFiscal   string `json:"cNumDocFiscal"`
			CNumParcela     string `json:"cNumParcela"`
			CNumTitulo      string `json:"cNumTitulo"`
			COperacao       string `json:"cOperacao"`
			COrigem         string `json:"cOrigem"`
			CRetCOFINS      string `json:"cRetCOFINS"`
			CRetCSLL        string `json:"cRetCSLL"`
			CRetINSS        string `json:"cRetINSS"`
			CRetIR          string `json:"cRetIR"`
			CRetISS         string `json:"cRetISS"`
			CRetPIS         string `json:"cRetPIS"`
			CStatus         string `json:"cStatus"`
			CTipo           string `json:"cTipo"`
			DDtEmissao      string `json:"dDtEmissao"`
			DDtPagamento    string `json:"dDtPagamento"`
			DDtPrevisao     string `json:"dDtPrevisao"`
			DDtRegistro     string `json:"dDtRegistro"`
			DDtVenc         string `json:"dDtVenc"`
			NCodCC          int    `json:"nCodCC"`
			NCodCliente     int    `json:"nCodCliente"`
			NCodTitRepet    int    `json:"nCodTitRepet"`
			NCodTitulo      int    `json:"nCodTitulo"`
			NValorCOFINS    int    `json:"nValorCOFINS"`
			NValorCSLL      int    `json:"nValorCSLL"`
			NValorINSS      int    `json:"nValorINSS"`
			NValorIR        int    `json:"nValorIR"`
			NValorISS       int    `json:"nValorISS"`
			NValorPIS       int    `json:"nValorPIS"`
			NValorTitulo    int    `json:"nValorTitulo"`
			Observacao      string `json:"observacao"`
		} `json:"cabecTitulo"`
		Departamentos []struct {
			CCodDepartamento string `json:"cCodDepartamento"`
			NDistrPercentual int    `json:"nDistrPercentual"`
			NDistrValor      int    `json:"nDistrValor"`
			NValorFixo       string `json:"nValorFixo"`
		} `json:"departamentos,omitempty"`
		Lancamentos []struct {
			CCodIntLanc string `json:"cCodIntLanc"`
			CNatureza   string `json:"cNatureza"`
			CObsLanc    string `json:"cObsLanc"`
			DDtLanc     string `json:"dDtLanc"`
			NCodCC      int    `json:"nCodCC"`
			NCodLanc    int64  `json:"nCodLanc"`
			NDesconto   int    `json:"nDesconto"`
			NIDLancCC   int64  `json:"nIdLancCC"`
			NJuros      int    `json:"nJuros"`
			NMulta      int    `json:"nMulta"`
			NValLanc    int    `json:"nValLanc"`
		} `json:"lancamentos"`
		Resumo struct {
			CLiquidado  string `json:"cLiquidado"`
			NDesconto   int    `json:"nDesconto"`
			NJuros      int    `json:"nJuros"`
			NMulta      int    `json:"nMulta"`
			NValAberto  int    `json:"nValAberto"`
			NValLiquido int    `json:"nValLiquido"`
			NValPago    int    `json:"nValPago"`
		} `json:"resumo"`
	} `json:"titulosEncontrados"`
}

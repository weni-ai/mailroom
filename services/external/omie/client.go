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

	data.Call = "PesquisarLancamentos"
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
	Param []IncluirContatoRequestParam `json:"param"`
}

type IncluirContatoRequestParam struct {
	Identificacao struct {
		NCod       int    `json:"nCod,omitempty"`
		CCodInt    string `json:"cCodInt,omitempty"`
		CNome      string `json:"cNome,omitempty"`
		CSobrenome string `json:"cSobrenome,omitempty"`
		CCargo     string `json:"cCargo,omitempty"`
		DDtNasc    string `json:"dDtNasc,omitempty"`
		NCodVend   int    `json:"nCodVend,omitempty"`
		NCodConta  int    `json:"nCodConta,omitempty"`
	} `json:"identificacao,omitempty"`
	Endereco struct {
		CEndereco string `json:"cEndereco,omitempty"`
		CCompl    string `json:"cCompl,omitempty"`
		CCEP      string `json:"cCEP,omitempty"`
		CBairro   string `json:"cBairro,omitempty"`
		CCidade   string `json:"cCidade,omitempty"`
		CUF       string `json:"cUF,omitempty"`
		CPais     string `json:"cPais,omitempty"`
	} `json:"endereco,omitempty"`
	TelefoneEmail struct {
		CDDDCel1 string `json:"cDDDCel1,omitempty"`
		CNumCel1 string `json:"cNumCel1,omitempty"`
		CDDDCel2 string `json:"cDDDCel2,omitempty"`
		CNumCel2 string `json:"cNumCel2,omitempty"`
		CDDDTel  string `json:"cDDDTel,omitempty"`
		CNumTel  string `json:"cNumTel,omitempty"`
		CDDDFax  string `json:"cDDDFax,omitempty"`
		CNumFax  string `json:"cNumFax,omitempty"`
		CEmail   string `json:"cEmail,omitempty"`
		CWebsite string `json:"cWebsite,omitempty"`
	} `json:"telefone_email,omitempty"`
	CObs string `json:"cObs,omitempty"`
}

type OpIncluirResponse struct {
	NCodOp     int64  `json:"nCodOp"`
	CCodIntOp  string `json:"cCodIntOp"`
	CCodStatus string `json:"cCodStatus"`
	CDesStatus string `json:"cDesStatus"`
}

type IncluirOportunidadeRequest struct {
	OmieCall
	Param []IncluirOportunidadeRequestParam `json:"param"`
}

type IncluirOportunidadeRequestParam struct {
	Identificacao OpIdentificacao `json:"identificacao,omitempty"`
	FasesStatus   OpFasesStatus   `json:"fasesStatus,omitempty"`
	Ticket        OpTicket        `json:"ticket,omitempty"`
	PrevisaoTemp  OpPrevisaoTemp  `json:"previsaoTemp,omitempty"`
	Observacoes   OpObservacoes   `json:"observacoes,omitempty"`
	OutraInf      OpOutrasInf     `json:"outrasInf,omitempty"`
	Envolvidos    OpEnvolvidos    `json:"envolvidos,omitempty"`
	Concorrentes  []interface{}   `json:"concorrentes,omitempty"`
}

type OpIdentificacao struct {
	CCodIntOp    string `json:"cCodIntOp,omitempty"`
	CDesOp       string `json:"cDesOp,omitempty"`
	NCodConta    int    `json:"nCodConta,omitempty"`
	NCodContato  int    `json:"nCodContato,omitempty"`
	NCodOp       int64  `json:"nCodOp,omitempty"`
	NCodOrigem   int    `json:"nCodOrigem,omitempty"`
	NCodSolucao  int    `json:"nCodSolucao,omitempty"`
	NCodVendedor int    `json:"nCodVendedor,omitempty"`
}

type OpFasesStatus struct {
	DConclusao    string `json:"dConclusao,omitempty"`
	DNovoLead     string `json:"dNovoLead,omitempty"`
	DProjeto      string `json:"dProjeto,omitempty"`
	DQualificacao string `json:"dQualificacao,omitempty"`
	DShowRoom     string `json:"dShowRoom,omitempty"`
	DTreinamento  string `json:"dTreinamento,omitempty"`
	NCodFase      int    `json:"nCodFase,omitempty"`
	NCodMotivo    int    `json:"nCodMotivo,omitempty"`
	NCodStatus    int    `json:"nCodStatus,omitempty"`
}
type OpTicket struct {
	NMeses       int `json:"nMeses,omitempty"`
	NProdutos    int `json:"nProdutos,omitempty"`
	NRecorrencia int `json:"nRecorrencia,omitempty"`
	NServicos    int `json:"nServicos,omitempty"`
	NTicket      int `json:"nTicket,omitempty"`
}

type OpPrevisaoTemp struct {
	NAnoPrev     int `json:"nAnoPrev,omitempty"`
	NMesPrev     int `json:"nMesPrev,omitempty"`
	NTemperatura int `json:"nTemperatura,omitempty"`
}

type OpObservacoes struct {
	CObs string `json:"cObs,omitempty"`
}

type OpOutrasInf struct {
	CEmailOp   string `json:"cEmailOp,omitempty"`
	DAlteracao string `json:"dAlteracao,omitempty"`
	DInclusao  string `json:"dInclusao,omitempty"`
	HAlteracao string `json:"hAlteracao,omitempty"`
	HInclusao  string `json:"hInclusao,omitempty"`
	NCodTipo   int    `json:"nCodTipo,omitempty"`
}

type OpEnvolvidos struct {
	NCodFinder   int `json:"nCodFinder,omitempty"`
	NCodParceiro int `json:"nCodParceiro,omitempty"`
	NCodPrevenda int `json:"nCodPrevenda,omitempty"`
}

type OpConcorrentes struct {
	NCodConc    int    `json:"nCodConc,omitempty"`
	CCodIntConc string `json:"cCodIntConc,omitempty"`
	CObservacao string `json:"cObservacao,omitempty"`
}

type ListarClientesResponse struct {
	Pagina           int               `json:"pagina,omitempty"`
	TotalDePaginas   int               `json:"total_de_paginas,omitempty"`
	Registros        int               `json:"registros,omitempty"`
	TotalDeRegistros int               `json:"total_de_registros,omitempty"`
	ClientesCadastro []ClienteCadastro `json:"clientes_cadastro,omitempty"`
}

type ClienteCadastro struct {
	Bairro                  string `json:"bairro,omitempty"`
	BloquearExclusao        string `json:"bloquear_exclusao,omitempty"`
	BloquearFaturamento     string `json:"bloquear_faturamento,omitempty"`
	Cep                     string `json:"cep,omitempty"`
	Cidade                  string `json:"cidade,omitempty"`
	CidadeIbge              string `json:"cidade_ibge,omitempty"`
	CnpjCpf                 string `json:"cnpj_cpf,omitempty"`
	CodigoClienteIntegracao string `json:"codigo_cliente_integracao,omitempty"`
	CodigoClienteOmie       int64  `json:"codigo_cliente_omie,omitempty"`
	CodigoPais              string `json:"codigo_pais,omitempty"`
	Complemento             string `json:"complemento,omitempty"`
	DadosBancarios          struct {
		Agencia       string `json:"agencia,omitempty"`
		CodigoBanco   string `json:"codigo_banco,omitempty"`
		ContaCorrente string `json:"conta_corrente,omitempty"`
		DocTitular    string `json:"doc_titular,omitempty"`
		NomeTitular   string `json:"nome_titular,omitempty"`
		TransfPadrao  string `json:"transf_padrao,omitempty"`
	} `json:"dadosBancarios,omitempty"`
	Email           string `json:"email,omitempty"`
	Endereco        string `json:"endereco,omitempty"`
	EnderecoEntrega struct {
	} `json:"enderecoEntrega,omitempty"`
	EnderecoNumero string `json:"endereco_numero,omitempty"`
	Estado         string `json:"estado,omitempty"`
	Exterior       string `json:"exterior,omitempty"`
	Inativo        string `json:"inativo,omitempty"`
	Info           struct {
		CImpAPI string `json:"cImpAPI,omitempty"`
		DAlt    string `json:"dAlt,omitempty"`
		DInc    string `json:"dInc,omitempty"`
		HAlt    string `json:"hAlt,omitempty"`
		HInc    string `json:"hInc,omitempty"`
		UAlt    string `json:"uAlt,omitempty"`
		UInc    string `json:"uInc,omitempty"`
	} `json:"info,omitempty"`
	InscricaoEstadual  string `json:"inscricao_estadual,omitempty"`
	InscricaoMunicipal string `json:"inscricao_municipal,omitempty"`
	NomeFantasia       string `json:"nome_fantasia,omitempty"`
	PessoaFisica       string `json:"pessoa_fisica,omitempty"`
	RazaoSocial        string `json:"razao_social,omitempty"`
	Recomendacoes      struct {
		GerarBoletos string `json:"gerar_boletos,omitempty"`
	} `json:"recomendacoes,omitempty"`
	Tags            []interface{} `json:"tags,omitempty"`
	Telefone1Ddd    string        `json:"telefone1_ddd,omitempty"`
	Telefone1Numero string        `json:"telefone1_numero,omitempty"`
}

type ListarClientesRequest struct {
	OmieCall
	Param []ListarClientesRequestParam `json:"param,omitempty"`
}

type ListarClientesRequestParam struct {
	Pagina                 int    `json:"pagina,omitempty"`
	RegistrosPorPagina     int    `json:"registros_por_pagina,omitempty"`
	ApenasImportadoAPI     string `json:"apenas_importado_api,omitempty"`
	OrdenarPor             string `json:"ordenar_por,omitempty"`
	OrdemDecrescente       string `json:"ordem_decrescente,omitempty"`
	FiltrarPorDataDe       string `json:"filtrar_por_data_de,omitempty"`
	FiltrarPorDataAte      string `json:"filtrar_por_data_ate,omitempty"`
	FiltrarPorHoraDe       string `json:"filtrar_por_hora_de,omitempty"`
	FiltrarPorHoraAte      string `json:"filtrar_por_hora_ate,omitempty"`
	FiltrarApenasInclusao  string `json:"filtrar_apenas_inclusao,omitempty"`
	FiltrarApenasAlteracao string `json:"filtrar_apenas_alteracao,omitempty"`
	ClientesFiltro         struct {
		CodigoClienteOmie       int    `json:"codigo_cliente_omie,omitempty"`
		CodigoClienteIntegracao string `json:"codigo_cliente_integracao,omitempty"`
		CnpjCpf                 string `json:"cnpj_cpf,omitempty"`
		RazaoSocial             string `json:"razao_social,omitempty"`
		NomeFantasia            string `json:"nome_fantasia,omitempty"`
		Endereco                string `json:"endereco,omitempty"`
		Bairro                  string `json:"bairro,omitempty"`
		Cidade                  string `json:"cidade,omitempty"`
		Estado                  string `json:"estado,omitempty"`
		Cep                     string `json:"cep,omitempty"`
		Contato                 string `json:"contato,omitempty"`
		Email                   string `json:"email,omitempty"`
		Homepage                string `json:"homepage,omitempty"`
		InscricaoMunicipal      string `json:"inscricao_municipal,omitempty"`
		InscricaoEstadual       string `json:"inscricao_estadual,omitempty"`
		InscricaoSuframa        string `json:"inscricao_suframa,omitempty"`
		PessoaFisica            string `json:"pessoa_fisica,omitempty"`
		OptanteSimplesNacional  string `json:"optante_simples_nacional,omitempty"`
		Inativo                 string `json:"inativo,omitempty"`
		Tags                    string `json:"tags,omitempty"`
	} `json:"clientesFiltro,omitempty"`
	ClientesPorCodigo struct {
		CodigoClienteOmie       int    `json:"codigo_cliente_omie,omitempty"`
		CodigoClienteIntegracao string `json:"codigo_cliente_integracao,omitempty"`
	} `json:"clientesPorCodigo,omitempty"`
	ExibirCaracteristicas string `json:"exibir_caracteristicas,omitempty"`
}

type PesquisarLancamentosRequest struct {
	OmieCall
	Param []PesquisarLancamentosParam `json:"param,omitempty"`
}

type PesquisarLancamentosParam struct {
	NPagina           int    `json:"nPagina,omitempty"`
	NRegPorPagina     int    `json:"nRegPorPagina,omitempty"`
	COrdenarPor       string `json:"cOrdenarPor,omitempty"`
	COrdemDecrescente string `json:"cOrdemDecrescente,omitempty"`
	NCodTitulo        int    `json:"nCodTitulo,omitempty"`
	CCodIntTitulo     string `json:"cCodIntTitulo,omitempty"`
	CNumTitulo        string `json:"cNumTitulo,omitempty"`
	DDtEmisDe         string `json:"dDtEmisDe,omitempty"`
	DDtEmisAte        string `json:"dDtEmisAte,omitempty"`
	DDtVencDe         string `json:"dDtVencDe,omitempty"`
	DDtVencAte        string `json:"dDtVencAte,omitempty"`
	DDtPagtoDe        string `json:"dDtPagtoDe,omitempty"`
	DDtPagtoAte       string `json:"dDtPagtoAte,omitempty"`
	DDtPrevDe         string `json:"dDtPrevDe,omitempty"`
	DDtPrevAte        string `json:"dDtPrevAte,omitempty"`
	DDtRegDe          string `json:"dDtRegDe,omitempty"`
	DDtRegAte         string `json:"dDtRegAte,omitempty"`
	NCodCliente       int    `json:"nCodCliente,omitempty"`
	CCPFCNPJCliente   string `json:"cCPFCNPJCliente,omitempty"`
	NCodCtr           int    `json:"nCodCtr,omitempty"`
	CNumCtr           string `json:"cNumCtr,omitempty"`
	NCodOS            int    `json:"nCodOS,omitempty"`
	CNumOS            string `json:"cNumOS,omitempty"`
	NCodCC            int    `json:"nCodCC,omitempty"`
	CStatus           string `json:"cStatus,omitempty"`
	CNatureza         string `json:"cNatureza,omitempty"`
	CTipo             string `json:"cTipo,omitempty"`
	COperacao         string `json:"cOperacao,omitempty"`
	CNumDocFiscal     string `json:"cNumDocFiscal,omitempty"`
	CCodigoBarras     string `json:"cCodigoBarras,omitempty"`
	NCodProjeto       int    `json:"nCodProjeto,omitempty"`
	NCodVendedor      int    `json:"nCodVendedor,omitempty"`
	NCodComprador     int    `json:"nCodComprador,omitempty"`
	CCodCateg         string `json:"cCodCateg,omitempty"`
	DDtIncDe          string `json:"dDtIncDe,omitempty"`
	DDtIncAte         string `json:"dDtIncAte,omitempty"`
	DDtAltDe          string `json:"dDtAltDe,omitempty"`
	DDtAltAte         string `json:"dDtAltAte,omitempty"`
	DDtCancDe         string `json:"dDtCancDe,omitempty"`
	DDtCancAte        string `json:"dDtCancAte,omitempty"`
	CChaveNFe         string `json:"cChaveNFe,omitempty"`
}

type PesquisarLancamentosResponse struct {
	NPagina            int `json:"nPagina,omitempty"`
	NTotPaginas        int `json:"nTotPaginas,omitempty"`
	NRegistros         int `json:"nRegistros,omitempty"`
	NTotRegistros      int `json:"nTotRegistros,omitempty"`
	TitulosEncontrados []struct {
		CabecTitulo struct {
			ACodCateg []struct {
				CCodCateg string  `json:"cCodCateg,omitempty"`
				NPerc     float64 `json:"nPerc,omitempty"`
				NValor    float64 `json:"nValor,omitempty"`
			} `json:"aCodCateg,omitempty"`
			CCPFCNPJCliente string  `json:"cCPFCNPJCliente,omitempty"`
			CCodCateg       string  `json:"cCodCateg,omitempty"`
			CCodIntTitulo   string  `json:"cCodIntTitulo,omitempty"`
			CCodVendedor    int     `json:"cCodVendedor,omitempty"`
			CNSU            string  `json:"cNSU,omitempty"`
			CNatureza       string  `json:"cNatureza,omitempty"`
			CNumBoleto      string  `json:"cNumBoleto,omitempty"`
			CNumDocFiscal   string  `json:"cNumDocFiscal,omitempty"`
			CNumParcela     string  `json:"cNumParcela,omitempty"`
			CNumTitulo      string  `json:"cNumTitulo,omitempty"`
			COperacao       string  `json:"cOperacao,omitempty"`
			COrigem         string  `json:"cOrigem,omitempty"`
			CRetCOFINS      string  `json:"cRetCOFINS,omitempty"`
			CRetCSLL        string  `json:"cRetCSLL,omitempty"`
			CRetINSS        string  `json:"cRetINSS,omitempty"`
			CRetIR          string  `json:"cRetIR,omitempty"`
			CRetISS         string  `json:"cRetISS,omitempty"`
			CRetPIS         string  `json:"cRetPIS,omitempty"`
			CStatus         string  `json:"cStatus,omitempty"`
			CTipo           string  `json:"cTipo,omitempty"`
			DDtEmissao      string  `json:"dDtEmissao,omitempty"`
			DDtPagamento    string  `json:"dDtPagamento,omitempty"`
			DDtPrevisao     string  `json:"dDtPrevisao,omitempty"`
			DDtRegistro     string  `json:"dDtRegistro,omitempty"`
			DDtVenc         string  `json:"dDtVenc,omitempty"`
			NCodCC          int     `json:"nCodCC,omitempty"`
			NCodCliente     int     `json:"nCodCliente,omitempty"`
			NCodTitRepet    int     `json:"nCodTitRepet,omitempty"`
			NCodTitulo      int     `json:"nCodTitulo,omitempty"`
			NValorCOFINS    float64 `json:"nValorCOFINS,omitempty"`
			NValorCSLL      float64 `json:"nValorCSLL,omitempty"`
			NValorINSS      float64 `json:"nValorINSS,omitempty"`
			NValorIR        float64 `json:"nValorIR,omitempty"`
			NValorISS       float64 `json:"nValorISS,omitempty"`
			NValorPIS       float64 `json:"nValorPIS,omitempty"`
			NValorTitulo    float64 `json:"nValorTitulo,omitempty"`
			Observacao      string  `json:"observacao,omitempty"`
			Departamentos   []struct {
				CCodDepartamento string  `json:"cCodDepartamento,omitempty"`
				NDistrPercentual float64 `json:"nDistrPercentual,omitempty"`
				NDistrValor      float64 `json:"nDistrValor,omitempty"`
				NValorFixo       string  `json:"nValorFixo,omitempty"`
			} `json:"departamentos,omitempty"`
		} `json:"cabecTitulo,omitempty"`
		Lancamentos []struct {
			CCodIntLanc string  `json:"cCodIntLanc,omitempty"`
			CNatureza   string  `json:"cNatureza,omitempty"`
			CObsLanc    string  `json:"cObsLanc,omitempty"`
			DDtLanc     string  `json:"dDtLanc,omitempty"`
			NCodCC      int     `json:"nCodCC,omitempty"`
			NCodLanc    int64   `json:"nCodLanc,omitempty"`
			NDesconto   float64 `json:"nDesconto,omitempty"`
			NIDLancCC   int64   `json:"nIdLancCC,omitempty"`
			NJuros      float64 `json:"nJuros,omitempty"`
			NMulta      float64 `json:"nMulta,omitempty"`
			NValLanc    float64 `json:"nValLanc,omitempty"`
		} `json:"lancamentos,omitempty"`
		Resumo struct {
			CLiquidado  string  `json:"cLiquidado,omitempty"`
			NDesconto   float64 `json:"nDesconto,omitempty"`
			NJuros      float64 `json:"nJuros,omitempty"`
			NMulta      float64 `json:"nMulta,omitempty"`
			NValAberto  float64 `json:"nValAberto,omitempty"`
			NValLiquido float64 `json:"nValLiquido,omitempty"`
			NValPago    float64 `json:"nValPago,omitempty"`
		} `json:"resumo,omitempty"`
	} `json:"titulosEncontrados,omitempty"`
}

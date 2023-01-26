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

func (c *Client) IncluirContato(data *IncluirContatoPayload) (*OmieResponse, *httpx.Trace, error) {
	requestUrl := "https://app.omie.com.br/api/v1/crm/contatos/"
	response := &OmieResponse{}

	data.Call = "IncluirContato"
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

type OmieResponse struct {
	NCod       int64  `json:"nCod"`
	CCodInt    string `json:"cCodInt"`
	CCodStatus string `json:"cCodStatus"`
	CDesStatus string `json:"cDesStatus"`
}

type IncluirContatoPayload struct {
	OmieCall
	Param []struct {
		Identificacao struct {
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
			CEmail   string `json:"cEmail"`
			CWebsite string `json:"cWebsite"`
		} `json:"telefone_email"`
	} `json:"param"`
}

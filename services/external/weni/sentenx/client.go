package sentenx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

const BaseURL = "https://sentenx.weni.ai"

type SearchRequest struct {
	Search string `json:"search,omitempty"`
	Filter struct {
		CatalogID string `json:"catalog_id,omitempty"`
	} `json:"filter,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

type SearchResponse struct {
	Products []Product `json:"products,omitempty"`
}

type Product struct {
	ProductRetailerID string `json:"product_retailer_id,omitempty"`
}

type ErrorResponse struct {
	Detail []struct {
		Msg string `json:"msg,omitempty"`
	} `json:"detail,omitempty"`
}

type Client struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL string) *Client {
	return &Client{httpClient, httpRetries, baseURL}
}

func (c *Client) Request(method, url string, body, response interface{}) (*httpx.Trace, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	data := strings.NewReader(string(b))
	req, err := httpx.NewRequest(method, url, data, nil)
	if err != nil {
		return nil, err
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		var errorResponse []ErrorResponse
		err = jsonx.Unmarshal(trace.ResponseBody, &errorResponse)
		if err != nil {
			return trace, err
		}
		return trace, errors.New(fmt.Sprint(errorResponse))
	}

	if response != nil {
		err := json.Unmarshal(trace.ResponseBody, response)
		return trace, errors.Wrap(err, "couldn't unmarshal response body")
	}
	return trace, nil
}

func (c *Client) Search(data *SearchRequest) (*SearchResponse, *httpx.Trace, error) {
	requestURL := c.baseURL + "/search"
	response := &SearchResponse{}

	trace, err := c.Request("GET", requestURL, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

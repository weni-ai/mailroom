package wenigpt

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

const BaseURL = "https://api.runpod.ai/v2/y4dkssg660i2vp/runsync"

var (
	defaultMaxNewTokens  = int64(1000)
	defaultTopP          = float64(0.1)
	defaultTemperature   = float64(0.1)
	DefaultStopSequences = []string{"Request", "Response"}
)

type Input struct {
	Prompt         string         `json:"prompt"`
	SamplingParams SamplingParams `json:"sampling_params"`
}

type Output struct {
	Text []string `json:"text"`
}

type SamplingParams struct {
	MaxNewTokens  int64    `json:"max_new_tokens"`
	TopP          float64  `json:"top_p"`
	Temperature   float64  `json:"temperature"`
	DoSample      bool     `json:"do_sample"`
	StopSequences []string `json:"stop_sequences,omitempty"`
}

type WeniGPTRequest struct {
	Input Input `json:"input"`
}

type WeniGPTResponse struct {
	DelayTime     int64  `json:"delayTime"`
	ExecutionTime int64  `json:"executionTime"`
	ID            string `json:"id"`
	Output        Output `json:"output"`
	Status        string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type WeniGPTStatus string

const (
	STATUS_COMPLETED   = WeniGPTStatus("COMPLETED")
	STATUS_IN_PROGRESS = WeniGPTStatus("IN_PROGRESS")
)

func NewWenigptRequest(prompt string, maxNewTokens int64, topP float64, temperature float64, doSample bool, stopSequences []string) *WeniGPTRequest {
	if maxNewTokens <= 0 {
		maxNewTokens = defaultMaxNewTokens
	}
	if topP <= 0.0 {
		topP = defaultTopP
	}
	if temperature <= 0.0 {
		temperature = defaultTemperature
	}

	return &WeniGPTRequest{
		Input: Input{
			Prompt: prompt,
			SamplingParams: SamplingParams{
				MaxNewTokens:  maxNewTokens,
				TopP:          topP,
				Temperature:   temperature,
				DoSample:      doSample,
				StopSequences: stopSequences,
			},
		},
	}
}

type Client struct {
	httpClient    *http.Client
	httpRetries   *httpx.RetryConfig
	baseURL       string
	authorization string
	cookie        string
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, authorization, cookie string) *Client {
	return &Client{httpClient, httpRetries, baseURL, authorization, cookie}
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
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.authorization)
	req.Header.Add("Cookie", c.cookie)

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &ErrorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return trace, errors.New(response.Error)
	}

	if response != nil {
		err := json.Unmarshal(trace.ResponseBody, response)
		return trace, errors.Wrap(err, "couldn't parse response body")
	}

	return trace, nil
}

func (c *Client) WeniGPTRequest(data *WeniGPTRequest) (*WeniGPTResponse, *httpx.Trace, error) {
	requestURL := c.baseURL
	response := &WeniGPTResponse{}

	trace, err := c.Request("POST", requestURL, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

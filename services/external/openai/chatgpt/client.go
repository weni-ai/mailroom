package chatgpt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/pkg/errors"
)

const (
	ChatMessageRoleSystem    = "system"
	ChatMessageRoleUser      = "user"
	ChatMessageRoleAssistant = "assistant"
)

var (
	ErrChatCompletionInvalidModel = errors.New("this model is not supported with this method, please use CreateCompletion client method instead")
)

type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type ChatCompletionRequest struct {
	Model            string                  `json:"model,omitempty"`
	Messages         []ChatCompletionMessage `json:"messages,omitempty"`
	MaxToken         int                     `json:"max_token,omitempty"`
	Temperature      float32                 `json:"temperature,omitempty"`
	TopP             float32                 `json:"top_p,omitempty"`
	N                int                     `json:"n,omitempty"`
	Stream           bool                    `json:"stream,omitempty"`
	Stop             []string                `json:"stop,omitempty"`
	PresencePenalty  float32                 `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32                 `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int          `json:"logit_bias,omitempty"`
	User             string                  `json:"user,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index,omitempty"`
	Message      ChatCompletionMessage `json:"message,omitempty"`
	FinishReason string                `json:"finish_reason,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id,omitempty"`
	Object  string                 `json:"object,omitempty"`
	Created int64                  `json:"created,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Choices []ChatCompletionChoice `json:"choices,omitempty"`
	Usage   Usage                  `json:"usage,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Error struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

type ErrorResponse struct {
	Error Error `json:"error,omitempty"`
}

type baseClient struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
	apiKey      string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, apiKey string) baseClient {
	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		baseURL:     baseURL,
		apiKey:      apiKey,
	}
}

type Client struct {
	baseClient
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, apiKey string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, baseURL, apiKey),
	}
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
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	if params != nil {
		req.URL.RawQuery = params.Encode()
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &ErrorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return trace, errors.New(fmt.Sprintf("message:%s. type:%s", response.Error.Message, response.Error.Type))
	}

	if response != nil {
		err = json.Unmarshal(trace.ResponseBody, response)
		return trace, errors.Wrap(err, "couldn't parse response body")
	}

	return trace, nil
}

func (c *baseClient) CreateChatCompletion(data *ChatCompletionRequest) (*ChatCompletionResponse, *httpx.Trace, error) {
	requestURL := c.baseURL + "/v1/chat/completions"
	response := &ChatCompletionResponse{}

	trace, err := c.request("POST", requestURL, nil, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

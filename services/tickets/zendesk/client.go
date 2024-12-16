package zendesk

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

type baseClient struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	subdomain   string
	token       string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, subdomain, token string) baseClient {
	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		subdomain:   subdomain,
		token:       token,
	}
}

type errorResponse struct {
	Error       string `json:"error"`
	Description string `json:"description"`
}

func (c *baseClient) get(endpoint string, payload interface{}, response interface{}) (*httpx.Trace, error) {
	return c.request("GET", endpoint, payload, response)
}

func (c *baseClient) post(endpoint string, payload interface{}, response interface{}) (*httpx.Trace, error) {
	return c.request("POST", endpoint, payload, response)
}

func (c *baseClient) put(endpoint string, payload interface{}, response interface{}) (*httpx.Trace, error) {
	return c.request("PUT", endpoint, payload, response)
}

func (c *baseClient) delete(endpoint string) (*httpx.Trace, error) {
	return c.request("DELETE", endpoint, nil, nil)
}

func (c *baseClient) request(method, endpoint string, payload interface{}, response interface{}) (*httpx.Trace, error) {
	url := fmt.Sprintf("https://%s.zendesk.com/api/v2/%s", c.subdomain, endpoint)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", c.token),
	}
	var body io.Reader

	if payload != nil {
		data, err := jsonx.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
		headers["Content-Type"] = "application/json"
	}

	req, err := httpx.NewRequest(method, url, body, headers)
	if err != nil {
		return nil, err
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return trace, errors.New(response.Description)
	}

	if response != nil {
		return trace, jsonx.Unmarshal(trace.ResponseBody, response)
	}
	return trace, nil
}

// RESTClient is a client for the Zendesk REST API
type RESTClient struct {
	baseClient
}

// NewRESTClient creates a new REST client
func NewRESTClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, subdomain, token string) *RESTClient {
	return &RESTClient{baseClient: newBaseClient(httpClient, httpRetries, subdomain, token)}
}

type Webhook struct {
	ID             string `json:"id"`
	Authentication struct {
		AddPosition string `json:"add_position"`
		Data        struct {
			Password string `json:"password"`
			Username string `json:"username"`
		} `json:"data"`
		Type string `json:"type"`
	} `json:"authentication"`
	Endpoint      string   `json:"endpoint"`
	HttpMethod    string   `json:"http_method"`
	Name          string   `json:"name"`
	RequestFormat string   `json:"request_format"`
	Status        string   `json:"status"`
	Subscriptions []string `json:"subscriptions"`
}

// CreateWebhook see https://developer.zendesk.com/api-reference/event-connectors/webhooks/webhooks/#create-or-clone-webhook
func (c *RESTClient) CreateWebhook(webhook *Webhook) (*Webhook, *httpx.Trace, error) {
	payload := struct {
		Webhook *Webhook `json:"webhook"`
	}{Webhook: webhook}

	response := &struct {
		Webhook *Webhook `json:"webhook"`
	}{}

	trace, err := c.post("webhooks", payload, response)
	if err != nil {
		return nil, trace, err
	}

	return response.Webhook, trace, nil
}

// DeleteWebhook see https://developer.zendesk.com/api-reference/event-connectors/webhooks/webhooks/#delete-webhook
func (c *RESTClient) DeleteWebhook(id int64) (*httpx.Trace, error) {
	return c.delete(fmt.Sprintf("webhooks/%d", id))
}

// Condition see https://developer.zendesk.com/rest_api/docs/support/triggers#conditions
type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// Conditions see https://developer.zendesk.com/rest_api/docs/support/triggers#conditions
type Conditions struct {
	All []Condition `json:"all"`
	Any []Condition `json:"any"`
}

// Action see https://developer.zendesk.com/rest_api/docs/support/triggers#actions
type Action struct {
	Field string   `json:"field"`
	Value []string `json:"value"`
}

// Trigger see https://developer.zendesk.com/rest_api/docs/support/triggers
type Trigger struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Conditions Conditions `json:"conditions"`
	Actions    []Action   `json:"actions"`
}

// CreateTrigger see https://developer.zendesk.com/rest_api/docs/support/triggers#create-trigger
func (c *RESTClient) CreateTrigger(trigger *Trigger) (*Trigger, *httpx.Trace, error) {
	payload := struct {
		Trigger *Trigger `json:"trigger"`
	}{Trigger: trigger}

	response := &struct {
		Trigger *Trigger `json:"trigger"`
	}{}

	trace, err := c.post("triggers.json", payload, response)
	if err != nil {
		return nil, trace, err
	}

	return response.Trigger, trace, nil
}

// DeleteTrigger see https://developer.zendesk.com/rest_api/docs/support/triggers#delete-trigger
func (c *RESTClient) DeleteTrigger(id int64) (*httpx.Trace, error) {
	return c.delete(fmt.Sprintf("triggers/%d.json", id))
}

// Ticket see https://developer.zendesk.com/rest_api/docs/support/tickets#json-format
type Ticket struct {
	ID         int64  `json:"id,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	Status     string `json:"status,omitempty"`
}

// JobStatus see https://developer.zendesk.com/rest_api/docs/support/job_statuses#job-statuses
type JobStatus struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

// UpdateManyTickets see https://developer.zendesk.com/rest_api/docs/support/tickets#update-many-tickets
func (c *RESTClient) UpdateManyTickets(ids []int64, status string) (*JobStatus, *httpx.Trace, error) {
	payload := struct {
		Ticket *Ticket `json:"ticket"`
	}{
		Ticket: &Ticket{Status: status},
	}

	response := &struct {
		JobStatus *JobStatus `json:"job_status"`
	}{}

	trace, err := c.put("tickets/update_many.json?ids="+encodeIds(ids), payload, response)
	if err != nil {
		return nil, trace, err
	}

	return response.JobStatus, trace, nil
}

// PushClient is a client for the Zendesk channel push API and requires a special push token
type PushClient struct {
	baseClient
}

// NewPushClient creates a new push client
func NewPushClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, subdomain, token string) *PushClient {
	return &PushClient{baseClient: newBaseClient(httpClient, httpRetries, subdomain, token)}
}

// FieldValue is a value for the named field
type FieldValue struct {
	ID    string      `json:"id"`
	Value interface{} `json:"value"`
}

// Author see https://developer.zendesk.com/rest_api/docs/support/channel_framework#author-object
type Author struct {
	ExternalID string       `json:"external_id"`
	Name       string       `json:"name,omitempty"`
	ImageURL   string       `json:"image_url,omitempty"`
	Locale     string       `json:"locale,omitempty"`
	Fields     []FieldValue `json:"fields,omitempty"`
}

// DisplayInfo see https://developer.zendesk.com/rest_api/docs/support/channel_framework#display_info-object
type DisplayInfo struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

// ExternalResource see https://developer.zendesk.com/rest_api/docs/support/channel_framework#external_resource-object
type ExternalResource struct {
	ExternalID       string        `json:"external_id"`
	Message          string        `json:"message"`
	HTMLMessage      string        `json:"html_message,omitempty"`
	ParentID         string        `json:"parent_id,omitempty"`
	ThreadID         string        `json:"thread_id,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	Author           Author        `json:"author"`
	DisplayInfo      []DisplayInfo `json:"display_info,omitempty"`
	AllowChannelback bool          `json:"allow_channelback"`
	Fields           []FieldValue  `json:"fields,omitempty"`
	FileURLs         []string      `json:"file_urls,omitempty"`
}

// Status see https://developer.zendesk.com/rest_api/docs/support/channel_framework#status-object
type Status struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Result see https://developer.zendesk.com/rest_api/docs/support/channel_framework#result-object
type Result struct {
	ExternalResourceID string `json:"external_resource_id"`
	Status             Status `json:"status"`
}

// Push pushes the given external resources
func (c *PushClient) Push(instanceID, requestID string, externalResources []*ExternalResource) ([]*Result, *httpx.Trace, error) {
	payload := struct {
		InstancePushID    string              `json:"instance_push_id"`
		RequestID         string              `json:"request_id,omitempty"`
		ExternalResources []*ExternalResource `json:"external_resources"`
	}{InstancePushID: instanceID, RequestID: requestID, ExternalResources: externalResources}

	response := &struct {
		Results []*Result `json:"results"`
	}{}

	trace, err := c.post("any_channel/push.json", payload, response)
	if err != nil {
		return nil, trace, err
	}

	return response.Results, trace, nil
}

func encodeIds(ids []int64) string {
	idStrs := make([]string, len(ids))
	for i := range ids {
		idStrs[i] = fmt.Sprintf("%d", ids[i])
	}
	return strings.Join(idStrs, ",")
}

// User see https://developer.zendesk.com/api-reference/ticketing/users/users/#json-format
type User struct {
	ID           int64  `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Organization struct {
		Name string `json:"name,omitempty"`
	} `json:"organization,omitempty"`
	ExternalID string     `json:"external_id,omitempty"`
	Identities []Identity `json:"identities,omitempty"`
	Verified   bool       `json:"verified,omitempty"`
	Role       string     `json:"role,omitempty"`
}

type Identity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// CreateUser creates a new user in zendesk
func (c *RESTClient) CreateUser(user *User) (*User, *httpx.Trace, error) {
	payload := struct {
		User *User `json:"user"`
	}{User: user}

	response := &struct {
		User *User `json:"user"`
	}{}

	trace, err := c.post("users.json", payload, response)
	if err != nil {
		return nil, trace, err
	}

	return response.User, trace, nil
}

type SearchUserResponse struct {
	Users []User `json:"users"`
}

// SearchUser returns the user or null if it does not exist, with retry logic for consistency delays.
func (c *RESTClient) SearchUser(query string) (*User, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("users/search.json?query=%s", query)
	maxRetries := 3
	delay := 2 * time.Second
	var (
		response SearchUserResponse
		trace    *httpx.Trace
		err      error
	)

	for i := 0; i < maxRetries; i++ {
		trace, err = c.get(endpoint, nil, &response)
		if err != nil {
			return nil, trace, err
		}

		if len(response.Users) > 0 {
			return &response.Users[0], trace, nil
		}

		time.Sleep(delay)
	}

	return nil, trace, nil
}

// MergeUser merge two users
func (c *RESTClient) MergeUser(userID int64, unmergedUserID int64) (*User, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("users/%s/merge", strconv.FormatInt(unmergedUserID, 10))

	payload := struct {
		User *User `json:"user"`
	}{User: &User{ID: userID}}

	response := &struct {
		User *User `json:"user"`
	}{}

	trace, err := c.put(endpoint, payload, &response)
	if err != nil {
		return nil, trace, err
	}
	return response.User, trace, nil
}

package twilioflex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

type baseClient struct {
	httpClient   *http.Client
	httpRetries  *httpx.RetryConfig
	authToken    string
	accountSid   string
	serviceSid   string
	workspaceSid string
	flexFlowSid  string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, authToken, accountSid, serviceSid, workspaceSid, flexFlowSid string) baseClient {
	return baseClient{
		httpClient:   httpClient,
		httpRetries:  httpRetries,
		authToken:    authToken,
		accountSid:   accountSid,
		serviceSid:   serviceSid,
		workspaceSid: workspaceSid,
		flexFlowSid:  flexFlowSid,
	}
}

type errorResponse struct {
	Code     int32  `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	MoreInfo string `json:"more_info,omitempty"`
	Status   int32  `json:"status,omitempty"`
}

func (c *baseClient) request(method, url string, payload url.Values, response interface{}) (*httpx.Trace, error) {
	data := strings.NewReader(payload.Encode())
	req, err := httpx.NewRequest(method, url, data, map[string]string{})
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.accountSid, c.authToken)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(payload.Encode())))

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return trace, errors.New(response.Message)
	}

	if response != nil {
		return trace, jsonx.Unmarshal(trace.ResponseBody, response)
	}
	return trace, nil
}

func (c *baseClient) post(url string, payload url.Values, response interface{}) (*httpx.Trace, error) {
	return c.request("POST", url, payload, response)
}

func (c *baseClient) get(url string, payload url.Values, response interface{}) (*httpx.Trace, error) {
	return c.request("GET", url, payload, response)
}

type Client struct {
	baseClient
}

// NewClient returns a new twilio api client.
func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, authToken, accountSid, serviceSid, workspaceSid, flexFlowSid string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, authToken, accountSid, serviceSid, workspaceSid, flexFlowSid),
	}
}

// CreateUser creates a new twilio chat User.
func (c *Client) CreateUser(user *CreateUserParams) (*User, *httpx.Trace, error) {
	requestUrl := fmt.Sprintf("https://conversations.twilio.com/v1/Services/%s/Users", c.serviceSid)
	response := &User{}
	data, err := query.Values(user)
	if err != nil {
		return nil, nil, err
	}
	trace, err := c.post(requestUrl, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// FetchUser fetch a twilio chat User by sid.
func (c *Client) FetchUser(userSid string) (*User, *httpx.Trace, error) {
	requestUrl := fmt.Sprintf("https://conversations.twilio.com/v1/Services/%s/Users/%s", c.serviceSid, userSid)
	response := &User{}
	trace, err := c.post(requestUrl, url.Values{}, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// CreateFlexChannel creates a new twilio flex Channel.
func (c *Client) CreateFlexChannel(channel *CreateFlexChannelParams) (*FlexChannel, *httpx.Trace, error) {
	url := "https://flex-api.twilio.com/v1/Channels"
	response := &FlexChannel{}
	data, err := query.Values(channel)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, err
}

// FetchFlexChannel fetch a twilio flex Channel by sid.
func (c *Client) FetchFlexChannel(channelSid string) (*FlexChannel, *httpx.Trace, error) {
	fetchUrl := fmt.Sprintf("https://flex-api.twilio.com/v1/Channels/%s", channelSid)
	response := &FlexChannel{}
	data := url.Values{}
	trace, err := c.get(fetchUrl, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, err
}

// CreateFlexConversationWebhook create a webhook target that is specific to a Channel.
func (c *Client) CreateFlexConversationWebhook(conversationWebhook *CreateConversationWebhookParams, channelSid string) (*ConversationWebhook, *httpx.Trace, error) {
	requestUrl := fmt.Sprintf("https://conversations.twilio.com/v1/Services/%s/Conversations/%s/Webhooks", c.serviceSid, channelSid)
	response := &ConversationWebhook{}
	data := url.Values{
		"Configuration.Url":     []string{conversationWebhook.ConfigurationUrl},
		"Configuration.Filters": conversationWebhook.ConfigurationFilters,
		"Configuration.Method":  []string{conversationWebhook.ConfigurationMethod},
		"Target":                []string{conversationWebhook.Target},
	}
	trace, err := c.post(requestUrl, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, err
}

// CreateMessage create a message in conversation.
func (c *Client) CreateMessage(message *CreateMessageParams) (*Message, *httpx.Trace, error) {
	url := fmt.Sprintf("https://conversations.twilio.com/v1/Services/%s/Conversations/%s/Messages", c.serviceSid, message.ConversationSid)
	response := &Message{}
	data, err := query.Values(message)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// CompleteTask updates a twilio taskrouter Task as completed.
func (c *Client) CompleteTask(taskSid string) (*TaskrouterTask, *httpx.Trace, error) {
	url := fmt.Sprintf("https://taskrouter.twilio.com/v1/Workspaces/%s/Tasks/%s", c.workspaceSid, taskSid)
	response := &TaskrouterTask{}
	task := &TaskrouterTask{
		AssignmentStatus: "completed",
		Reason:           "resolved",
	}
	data, err := query.Values(task)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) CreateMedia(media *CreateMediaParams) (*Media, *httpx.Trace, error) {
	url := fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media", c.serviceSid)
	response := &Media{}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	mediaPart, err := writer.CreateFormFile("Media", media.FileName)
	if err != nil {
		return nil, nil, err
	}
	mediaReader := bytes.NewReader(media.Media)
	io.Copy(mediaPart, mediaReader)

	filenamePart, err := writer.CreateFormField("FileName")
	if err != nil {
		return nil, nil, err
	}
	filenameReader := bytes.NewReader([]byte(media.FileName))
	io.Copy(filenamePart, filenameReader)

	writer.Close()

	req, err := httpx.NewRequest("POST", url, body, map[string]string{})
	if err != nil {
		return nil, nil, err
	}
	req.SetBasicAuth(c.accountSid, c.authToken)
	req.Header.Add("Content-Type", writer.FormDataContentType())

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return nil, trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		jsonx.Unmarshal(trace.ResponseBody, response)
		return nil, trace, errors.New(response.Message)
	}

	err = jsonx.Unmarshal(trace.ResponseBody, response)
	if err != nil {
		return nil, trace, err
	}

	return response, trace, nil
}

// FetchMedia fetch a twilio flex Media by this sid.
func (c *Client) FetchMedia(mediaSid string) (*Media, *httpx.Trace, error) {
	fetchUrl := fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media/%s", c.serviceSid, mediaSid)
	response := &Media{}
	data := url.Values{}
	trace, err := c.get(fetchUrl, data, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, err
}

// https://www.twilio.com/docs/conversations/api/user-resource#user-properties
type User struct {
	AccountSid   string                 `json:"account_sid,omitempty"`
	Attributes   string                 `json:"attributes,omitempty"`
	DateCreated  *time.Time             `json:"date_created,omitempty"`
	DateUpdated  *time.Time             `json:"date_updated,omitempty"`
	FriendlyName string                 `json:"friendly_name,omitempty"`
	Identity     string                 `json:"identity,omitempty"`
	Links        map[string]interface{} `json:"links,omitempty"`
	RoleSid      string                 `json:"role_sid,omitempty"`
	ServiceSid   string                 `json:"service_sid,omitempty"`
	Sid          string                 `json:"sid,omitempty"`
	Url          string                 `json:"url,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/user-resource#create-a-conversations-user
type CreateUserParams struct {
	XTwilioWebhookEnabled string `json:"X-Twilio-Webhook-Enabled,omitempty"`
	Attributes            string `json:"Attributes,omitempty"`
	FriendlyName          string `json:"FriendlyName,omitempty"`
	Identity              string `json:"Identity,omitempty"`
	RoleSid               string `json:"RoleSid,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/conversation-resource#conversation-properties
type Conversation struct {
	AccountSid          string     `json:"account_sid,omitempty"`
	Attributes          string     `json:"attributes,omitempty"`
	ChatServiceSid      string     `json:"chat_service_sid,omitempty"`
	DateCreated         *time.Time `json:"date_created,omitempty"`
	DateUpdated         *time.Time `json:"date_updated,omitempty"`
	FriendlyName        string     `json:"friendly_name,omitempty"`
	MessagingServiceSid string     `json:"messaging_service_sid"`
	Sid                 string     `json:"sid,omitempty"`
	UniqueName          string     `json:"unique_name,omitempty"`
}

// https://www.twilio.com/docs/flex/developer/messaging/api/chat-channel#channel-properties
type FlexChannel struct {
	AccountSid  string     `json:"account_sid,omitempty"`
	DateCreated *time.Time `json:"date_created,omitempty"`
	DateUpdated *time.Time `json:"date_updated,omitempty"`
	FlexFlowSid string     `json:"flex_flow_sid,omitempty"`
	Sid         string     `json:"sid,omitempty"`
	TaskSid     string     `json:"task_sid,omitempty"`
	Url         string     `json:"url,omitempty"`
	UserSid     string     `json:"user_sid,omitempty"`
}

// https://www.twilio.com/docs/flex/developer/messaging/api/chat-channel#create-a-channel-resource
type CreateFlexChannelParams struct {
	ChatFriendlyName     string `json:"ChatFriendlyName,omitempty"`
	ChatUniqueName       string `json:"ChatUniqueName,omitempty"`
	ChatUserFriendlyName string `json:"ChatUserFriendlyName,omitempty"`
	FlexFlowSid          string `json:"FlexFlowSid,omitempty"`
	Identity             string `json:"Identity,omitempty"`
	LongLived            bool   `json:"LongLived,omitempty"`
	PreEngagementData    string `json:"PreEngagementData,omitempty"`
	Target               string `json:"Target,omitempty"`
	TaskAttributes       string `json:"TaskAttributes,omitempty"`
	TaskSid              string `json:"TaskSid,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/conversation-message-resource#create-a-conversationmessage-resource
type Message struct {
	AccountSid      string                 `json:"account_sid,omitempty"`
	Attributes      string                 `json:"attributes,omitempty"`
	Body            string                 `json:"body,omitempty"`
	ConversationSid string                 `json:"conversation_sid,omitempty"`
	DateCreated     *time.Time             `json:"date_created,omitempty"`
	DateUpdated     *time.Time             `json:"date_updated,omitempty"`
	Author          string                 `json:"author,omitempty"`
	Index           int                    `json:"index,omitempty"`
	Media           map[string]interface{} `json:"media,omitempty"`
	ChatServiceSid  string                 `json:"chat_service_sid,omitempty"`
	Sid             string                 `json:"sid,omitempty"`
	To              string                 `json:"to,omitempty"`
	Type            string                 `json:"type,omitempty"`
	Url             string                 `json:"url,omitempty"`
	WasEdited       bool                   `json:"was_edited,omitempty"`
}

type CreateMessageParams struct {
	Body            string `json:"Body,omitempty"`
	Author          string `json:"Author,omitempty"`
	Attributes      string `json:"Attributes,omitempty"`
	MediaSid        string `json:"MediaSid,omitempty"`
	ConversationSid string `json:"ConversationSid,omitempty"`
	DateCreated     string `json:"DateCreated,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/conversation-scoped-webhook-resource#conversationscopedwebhook-properties
type ConversationWebhook struct {
	AccountSid      string                 `json:"account_sid,omitempty"`
	ConversationSid string                 `json:"conversation_sid,omitempty"`
	Configuration   map[string]interface{} `json:"configuration,omitempty"`
	DateCreated     *time.Time             `json:"date_created,omitempty"`
	DateUpdated     *time.Time             `json:"date_updated,omitempty"`
	Sid             string                 `json:"sid,omitempty"`
	Target          string                 `json:"target,omitempty"`
	Url             string                 `json:"url,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/conversation-scoped-webhook-resource#create-a-conversationscopedwebhook-resource
type CreateConversationWebhookParams struct {
	ConfigurationFilters  []string `json:"Configuration.Filters,omitempty"`
	ConfigurationFlowSid  string   `json:"Configuration.FlowSid,omitempty"`
	ConfigurationMethod   string   `json:"Configuration.Method,omitempty"`
	ConfigurationTriggers []string `json:"Configuration.Triggers,omitempty"`
	ConfigurationUrl      string   `json:"Configuration.Url,omitempty"`
	Target                string   `json:"Target,omitempty"`
}

// https://www.twilio.com/docs/taskrouter/api/task#task-properties
type TaskrouterTask struct {
	AccountSid            string                 `json:"account_sid,omitempty"`
	Addons                string                 `json:"addons,omitempty"`
	Age                   int                    `json:"age,omitempty"`
	AssignmentStatus      string                 `json:"assignment_status,omitempty"`
	Attributes            string                 `json:"attributes,omitempty"`
	DateCreated           *time.Time             `json:"date_created,omitempty"`
	DateUpdated           *time.Time             `json:"date_updated,omitempty"`
	Links                 map[string]interface{} `json:"links,omitempty"`
	Priority              int                    `json:"priority,omitempty"`
	Reason                string                 `json:"reason,omitempty"`
	Sid                   string                 `json:"sid,omitempty"`
	TaskChannel           string                 `json:"task_channel,omitempty"`
	TaskChannelUniqueName string                 `json:"task_channel_unique_name,omitempty"`
	TaskQueueEnteredDate  *time.Time             `json:"task_queue_entered_date,omitempty"`
	TaskQueueFriendlyName string                 `json:"task_queue_friendly_name,omitempty"`
	TaskQueueSid          string                 `json:"task_queue_sid,omitempty"`
	Timeout               int                    `json:"timeout,omitempty"`
	Url                   string                 `json:"url,omitempty"`
	WorkflowFriendlyName  string                 `json:"workflow_friendly_name,omitempty"`
	WorkflowSid           string                 `json:"workflow_sid,omitempty"`
	WorkspaceSid          string                 `json:"workspace_sid,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/media-resource#properties
type CreateMediaParams struct {
	FileName string `json:"FileName,omitempty"`
	Media    []byte `json:"Media,omitempty"`
	Author   string `json:"Author,omitempty"`
}

// https://www.twilio.com/docs/conversations/api/media-resource#properties
type Media struct {
	Sid               string `json:"sid"`
	ServiceSid        string `json:"service_sid"`
	DateCreated       string `json:"date_created"`
	DateUploadUpdated string `json:"date_upload_updated"`
	DateUpdated       string `json:"date_updated"`
	Links             struct {
		Content                string `json:"content"`
		ContentDirectTemporary string `json:"content_direct_temporary"`
	} `json:"links"`
	Size                int         `json:"size"`
	ContentType         string      `json:"content_type"`
	Filename            string      `json:"filename"`
	Author              string      `json:"author"`
	Category            string      `json:"category"`
	MessageSid          interface{} `json:"message_sid"`
	ChannelSid          interface{} `json:"channel_sid"`
	URL                 string      `json:"url"`
	IsMultipartUpstream bool        `json:"is_multipart_upstream"`
}

// removeEmpties remove empty values from url.Values
func removeEmpties(uv url.Values) url.Values {
	for k, v := range uv {
		if len(v) == 0 || len(v[0]) == 0 {
			delete(uv, k)
		}
	}
	return uv
}

package twilioflex2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

type baseClient struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	authToken   string
	accountSid  string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, authToken, accountSid string) baseClient {
	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		authToken:   authToken,
		accountSid:  accountSid,
	}
}

type errorResponse struct {
	Code     int32  `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	MoreInfo string `json:"more_info,omitempty"`
	Status   int32  `json:"status,omitempty"`
}

func (c *baseClient) request(method, url string, payload url.Values, response any, extraHeaders http.Header) (*httpx.Trace, error) {
	data := strings.NewReader(payload.Encode())
	req, err := httpx.NewRequest(method, url, data, map[string]string{})
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.accountSid, c.authToken)

	for k, vv := range extraHeaders {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

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

func (c *baseClient) post(url string, payload url.Values, response any, extraHeaders http.Header) (*httpx.Trace, error) {
	return c.request("POST", url, payload, response, extraHeaders)
}

func (c *baseClient) get(url string, payload url.Values, response any, extraHeaders http.Header) (*httpx.Trace, error) {
	return c.request("GET", url, payload, response, extraHeaders)
}

type Client struct {
	baseClient
}

// NewClient returns a new twilio flex interactions api client.
func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, authToken, accountSid string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, authToken, accountSid),
	}
}

// CreateInteractionScopedWebhook creates a webhook scoped to a specific interaction.
// https://www.twilio.com/docs/flex/developer/conversations/register-interactions-webhooks#register-an-interaction-webhook-for-a-specific-interaction
func (c *Client) CreateInteractionScopedWebhook(instanceSid string, webhook *CreateInteractionWebhookRequest) (*CreateInteractionWebhookResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("https://flex-api.twilio.com/v1/Instances/%s/InteractionWebhooks", instanceSid)
	response := &CreateInteractionWebhookResponse{}
	data, err := query.Values(webhook)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// CreateInteraction creates a new interaction.
func (c *Client) CreateInteraction(interaction *CreateInteractionRequest) (*CreateInteractionResponse, *httpx.Trace, error) {
	endpoint := "https://flex-api.twilio.com/v1/Interactions"
	response := &CreateInteractionResponse{}
	channelPayload := map[string]any{
		"type":         interaction.Channel.Type,
		"initiated_by": interaction.Channel.InitiatedBy,
		"participants": interaction.Channel.Participants,
	}

	if len(interaction.Channel.Properties) > 0 {
		channelPayload["properties"] = interaction.Channel.Properties
	}

	routingPayload := map[string]any{
		"type":       interaction.Routing.Type,
		"properties": interaction.Routing.Properties,
	}

	chJSON, _ := json.Marshal(channelPayload)
	routingJSON, _ := json.Marshal(routingPayload)

	data := url.Values{}
	data.Set("Channel", string(chJSON))
	data.Set("Routing", string(routingJSON))
	if strings.TrimSpace(interaction.WebhookTtid) != "" {
		data.Set("WebhookTtid", interaction.WebhookTtid)
	}

	trace, err := c.post(endpoint, data, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// CreateConversationScopedWebhook creates a webhook for a specific conversation.
func (c *Client) CreateConversationScopedWebhook(conversationSid string, webhook *CreateConversationWebhookRequest) (*CreateConversationWebhookResponse, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("https://conversations.twilio.com/v1/Conversations/%s/Webhooks", conversationSid)
	response := &CreateConversationWebhookResponse{}
	form := url.Values{}
	form.Set("Target", webhook.Target)
	form.Set("Configuration.Url", webhook.ConfigurationUrl)
	form.Set("Configuration.Method", webhook.ConfigurationMethod)
	if len(webhook.ConfigurationFilters) > 0 {
		for _, filter := range webhook.ConfigurationFilters {
			form.Add("Configuration.Filters", filter)
		}
	}
	trace, err := c.post(endpoint, form, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// SendCustomerMessage sends a message from a customer to a conversation.
func (c *Client) SendCustomerMessage(conversationSid string, message *CreateConversationMessageRequest) (*CreateConversationMessageResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("https://conversations.twilio.com/v1/Conversations/%s/Messages", conversationSid)
	response := &CreateConversationMessageResponse{}
	data, err := query.Values(message)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// UpdateInteractionChannel updates a channel within an interaction.
func (c *Client) UpdateInteractionChannel(interactionSid, channelSid string, channel *UpdateInteractionChannelRequest) (*UpdateInteractionChannelResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("https://flex-api.twilio.com/v1/Interactions/%s/Channels/%s", interactionSid, channelSid)
	response := &UpdateInteractionChannelResponse{}
	data, err := query.Values(channel)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(url, data, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// CreateMedia creates a new media resource for conversations.
func (c *Client) CreateMedia(serviceSid string, media *CreateMediaParams) (*Media, *httpx.Trace, error) {
	url := fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media", serviceSid)
	response := &Media{}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set(
		"Content-Disposition",
		fmt.Sprintf(
			`form-data; name="%s"; filename="%s"`,
			"Media", media.FileName,
		),
	)
	h.Set("Content-Type", media.ContentType)
	mediaPart, err := writer.CreatePart(h)
	if err != nil {
		return nil, nil, err
	}
	mediaReader := bytes.NewReader(media.Media)
	io.Copy(mediaPart, mediaReader)

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

// FetchMedia fetches a media resource by its SID.
func (c *Client) FetchMedia(serviceSid, mediaSid string) (*Media, *httpx.Trace, error) {
	fetchUrl := fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media/%s", serviceSid, mediaSid)
	response := &Media{}
	data := url.Values{}
	trace, err := c.get(fetchUrl, data, response, nil)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

// FindConversationUserByIdentity fetches a participant using identity in the Participants/{Sid} slot.
// Twilio accepts identity in place of the participant SID for lookup and returns participant details including Sid.
func (c *Client) FindConversationUserByIdentity(conversationSid, identity string) (*CreateConversationParticipantResponse, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("https://conversations.twilio.com/v1/Conversations/%s/Participants/%s", conversationSid, identity)
	resp := &CreateConversationParticipantResponse{}
	data := url.Values{}
	trace, err := c.get(endpoint, data, resp, nil)
	if err != nil {
		return nil, trace, err
	}
	return resp, trace, nil
}

// UpdateChatUser updates a Chat/Conversations Service user by SID (US...).
// API ref: https://chat.twilio.com/v2/Services/{ServiceSid}/Users/{Sid}
func (c *Client) UpdateChatUser(serviceSid, userSid string, user *UpdateChatUserRequest) (*ChatUser, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("https://chat.twilio.com/v2/Services/%s/Users/%s", serviceSid, userSid)
	resp := &ChatUser{}
	data, err := query.Values(user)
	if err != nil {
		return nil, nil, err
	}
	data = removeEmpties(data)
	trace, err := c.post(endpoint, data, resp, nil)
	if err != nil {
		return nil, trace, err
	}
	return resp, trace, nil
}

// CreateInteractionWebhookRequest parameters for creating an interaction webhook
// https://www.twilio.com/docs/flex/developer/conversations/register-interactions-webhooks
type CreateInteractionWebhookRequest struct {
	Type          string   `json:"Type,omitempty"`
	WebhookUrl    string   `json:"WebhookUrl,omitempty"`
	WebhookMethod string   `json:"WebhookMethod,omitempty"`
	WebhookEvents []string `json:"WebhookEvents,omitempty"`
}

// https://www.twilio.com/docs/flex/developer/conversations/register-interactions-webhooks#request-parameters
type CreateInteractionWebhookResponse struct {
	Ttid          string   `json:"ttid,omitempty"`
	AccountSid    string   `json:"account_sid,omitempty"`
	InstanceSid   string   `json:"instance_sid,omitempty"`
	WebhookUrl    string   `json:"webhook_url,omitempty"`
	WebhookEvents []string `json:"webhook_events,omitempty"`
	WebhookMethod string   `json:"webhook_method,omitempty"`
	Type          string   `json:"type,omitempty"`
	Url           string   `json:"url,omitempty"`
}

// CreateInteractionRequest parameters for creating an interaction
// https://www.twilio.com/docs/flex/developer/conversations/interactions-api/interactions#create-an-interaction-resource
type CreateInteractionRequest struct {
	Channel     InteractionChannelParam `json:"Channel,omitempty"`
	Routing     InteractionRoutingParam `json:"Routing,omitempty"`
	WebhookTtid string                  `json:"WebhookTtid,omitempty"`
}

type InteractionChannelParam struct {
	Type         string                          `json:"type,omitempty"`
	InitiatedBy  string                          `json:"initiated_by,omitempty"`
	Properties   map[string]any                  `json:"properties,omitempty"`
	Participants []InteractionChannelParticipant `json:"participants,omitempty"`
}

type InteractionChannelParticipant struct {
	Identity string `json:"identity,omitempty"`
	Name     string `json:"name,omitempty"`
}

type InteractionRoutingParam struct {
	Type       string                       `json:"type,omitempty"`
	Properties InteractionRoutingProperties `json:"properties"`
}

type InteractionRoutingProperties struct {
	WorkspaceSid          string         `json:"workspace_sid,omitempty"`
	WorkflowSid           string         `json:"workflow_sid,omitempty"`
	TaskChannelUniqueName string         `json:"task_channel_unique_name,omitempty"`
	Attributes            map[string]any `json:"attributes,omitempty"`
}

type InteractionRoutingPropertiesResponse struct {
	WorkspaceSid          string `json:"workspace_sid,omitempty"`
	WorkflowSid           string `json:"workflow_sid,omitempty"`
	TaskChannelUniqueName string `json:"task_channel_unique_name,omitempty"`
	Attributes            string `json:"attributes,omitempty"`
}

// // https://www.twilio.com/docs/flex/developer/conversations/interactions-api/interactions#interaction-properties
type CreateInteractionResponse struct {
	Sid                   string                     `json:"sid,omitempty"`
	Channel               map[string]any             `json:"channel,omitempty"`
	Routing               InteractionRoutingResponse `json:"routing,omitempty"`
	InteractionContextSid string                     `json:"interaction_context_sid,omitempty"`
	WebhookTtid           string                     `json:"webhook_ttid,omitempty"`
	URL                   string                     `json:"url,omitempty"`
}

type InteractionRouting struct {
	Properties InteractionRoutingProperties `json:"properties,omitempty"`
}

type InteractionRoutingResponse struct {
	Properties InteractionRoutingPropertiesResponse `json:"properties,omitempty"`
}

// CreateConversationWebhookRequest parameters for creating a conversation webhook
// https://www.twilio.com/docs/conversations/api/conversation-scoped-webhook-resource#create-a-conversationscopedwebhook-resource
type CreateConversationWebhookRequest struct {
	Target               string   `json:"Target,omitempty"`
	ConfigurationUrl     string   `json:"Configuration.Url,omitempty"`
	ConfigurationMethod  string   `json:"Configuration.Method,omitempty"`
	ConfigurationFilters []string `json:"Configuration.Filters,omitempty"`
}

// ConversationWebhook represents a webhook for conversations
// https://www.twilio.com/docs/conversations/api/conversation-scoped-webhook-resource#create-a-conversationscopedwebhook-resource
type CreateConversationWebhookResponse struct {
	Sid             string         `json:"sid,omitempty"`
	AccountSid      string         `json:"account_sid,omitempty"`
	ConversationSid string         `json:"conversation_sid,omitempty"`
	Target          string         `json:"target,omitempty"`
	Configuration   map[string]any `json:"configuration,omitempty"`
	DateCreated     *time.Time     `json:"date_created,omitempty"`
	DateUpdated     *time.Time     `json:"date_updated,omitempty"`
	Url             string         `json:"url,omitempty"`
}

// CreateConversationMessageRequest parameters for creating a conversation message
// https://www.twilio.com/docs/conversations/api/conversation-message-resource#message-properties
type CreateConversationMessageRequest struct {
	Author                string `json:"Author,omitempty"`
	Body                  string `json:"Body,omitempty"`
	DateCreated           string `json:"DateCreated,omitempty"`
	MediaSid              string `json:"MediaSid,omitempty"`
	XTwilioWebhookEnabled string `json:"X-Twilio-Webhook-Enabled,omitempty"`
}

// ConversationMessage represents a message in a conversation
// https://www.twilio.com/docs/conversations/api/conversation-message-resource#message-properties
type CreateConversationMessageResponse struct {
	Sid             string           `json:"sid,omitempty"`
	AccountSid      string           `json:"account_sid,omitempty"`
	ConversationSid string           `json:"conversation_sid,omitempty"`
	Body            string           `json:"body,omitempty"`
	Author          string           `json:"author,omitempty"`
	Media           []map[string]any `json:"media,omitempty"`
	ParticipantSid  string           `json:"participant_sid,omitempty"`
	Index           int              `json:"index,omitempty"`
}

// CreateConversationParticipantResponse represents a participant
// https://www.twilio.com/docs/conversations/api/conversation-participant-resource
type CreateConversationParticipantResponse struct {
	Sid             string `json:"sid,omitempty"`
	AccountSid      string `json:"account_sid,omitempty"`
	ConversationSid string `json:"conversation_sid,omitempty"`
	Identity        string `json:"identity,omitempty"`
	FriendlyName    string `json:"friendly_name,omitempty"`
	Attributes      string `json:"attributes,omitempty"`
	DateCreated     string `json:"date_created,omitempty"`
	DateUpdated     string `json:"date_updated,omitempty"`
	URL             string `json:"url,omitempty"`
}

// UpdateChatUserRequest payload for chat user update (FriendlyName/Attributes)
type UpdateChatUserRequest struct {
	FriendlyName string `url:"FriendlyName,omitempty" json:"FriendlyName,omitempty"`
	Attributes   string `url:"Attributes,omitempty" json:"Attributes,omitempty"`
}

// ChatUser represents a chat user response (minimal fields we care about)
type ChatUser struct {
	Sid          string `json:"sid,omitempty"`
	Identity     string `json:"identity,omitempty"`
	FriendlyName string `json:"friendly_name,omitempty"`
	Attributes   string `json:"attributes,omitempty"`
}

// UpdateInteractionChannelRequest parameters for updating an interaction channel
type UpdateInteractionChannelRequest struct {
	Status         string `json:"Status,omitempty"`
	Routing        string `json:"Routing,omitempty"`
	JanitorEnabled string `json:"JanitorEnabled,omitempty"`
}

// UpdateInteractionChannelResponse represents a channel within an interaction
type UpdateInteractionChannelResponse struct {
	Sid            string           `json:"sid,omitempty"`
	AccountSid     string           `json:"account_sid,omitempty"`
	InteractionSid string           `json:"interaction_sid,omitempty"`
	Type           string           `json:"type,omitempty"`
	Status         string           `json:"status,omitempty"`
	Participants   []map[string]any `json:"participants,omitempty"`
	DateCreated    *time.Time       `json:"date_created,omitempty"`
	DateUpdated    *time.Time       `json:"date_updated,omitempty"`
	Url            string           `json:"url,omitempty"`
}

// CreateMediaParams parameters for creating media
// https://www.twilio.com/docs/chat/rest/media
type CreateMediaParams struct {
	FileName    string `json:"FileName,omitempty"`
	Media       []byte `json:"Media,omitempty"`
	Author      string `json:"Author,omitempty"`
	ContentType string `json:"ContentType"`
}

// Media represents a media resource
// https://www.twilio.com/docs/chat/rest/media
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

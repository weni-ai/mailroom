package freshchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
)

type baseClient struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
	authToken   string
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, authToken string) baseClient {
	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		baseURL:     baseURL,
		authToken:   authToken,
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func (c *baseClient) request(method, endpoint string, payload interface{}, response interface{}) (*httpx.Trace, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, endpoint)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", c.authToken),
		"Content-Type":  "application/json",
	}
	var body io.Reader

	if payload != nil {
		data, err := jsonx.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
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
		err := jsonx.Unmarshal(trace.ResponseBody, response)
		if err != nil {
			return trace, err
		}
		return trace, errors.New(response.Error)
	}

	if response != nil {
		return trace, jsonx.Unmarshal(trace.ResponseBody, response)
	}
	return trace, nil
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

type Client struct {
	baseClient
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, apiKey string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, baseURL, apiKey),
	}
}

func (c *Client) CreateConversation(conversation *Conversation) (*Conversation, *httpx.Trace, error) {
	endpoint := "/v2/conversations"
	response := &Conversation{}
	trace, err := c.post(endpoint, conversation, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) CreateUser(user *User) (*User, *httpx.Trace, error) {
	endpoint := "/v2/users"
	response := &User{}
	trace, err := c.post(endpoint, user, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) GetChannels() ([]*Channel, *httpx.Trace, error) {
	endpoint := "/v2/channels"
	response := []*Channel{}
	trace, err := c.get(endpoint, nil, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) CreateMessage(message *Message) (*Message, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("/v2/%s/messages", message.ConversationID)
	response := &Message{}
	trace, err := c.post(endpoint, message, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) UpdateConversation(conversation *Conversation) (*httpx.Trace, error) {
	endpoint := fmt.Sprintf("/v2/conversations/%s", string(conversation.ConversationID))
	response := &Conversation{}
	trace, err := c.put(endpoint, conversation, response)
	if err != nil {
		return trace, err
	}
	return trace, err
}

func (c *Client) UploadImage(imageURL string) (string, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	fileName := strings.Split(imageURL, ".")[len(strings.Split(imageURL, "."))-1]
	if u, err := url.Parse(imageURL); err == nil {
		parts := strings.Split(u.Path, "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			fileName = parts[len(parts)-1]
		}
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("image", fileName)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, resp.Body)
	if err != nil {
		return "", err
	}
	writer.Close()

	endpoint := c.baseURL + "/v2/images/upload"
	req, err := http.NewRequest("POST", endpoint, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("accept", "application/json")

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return "", err
	}
	defer trace.Response.Body.Close()

	var imgResp Image
	if err := json.NewDecoder(trace.Response.Body).Decode(&imgResp); err != nil {
		return "", err
	}
	return imgResp.URL, nil
}

func (c *Client) UploadFile(fileURL string) (*File, error) {
	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	fileName := strings.Split(fileURL, ".")[len(strings.Split(fileURL, "."))-1]
	if u, err := url.Parse(fileURL); err == nil {
		parts := strings.Split(u.Path, "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			fileName = parts[len(parts)-1]
		}
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, resp.Body)
	if err != nil {
		return nil, err
	}
	writer.Close()

	endpoint := c.baseURL + "/v2/files/upload"
	req, err := http.NewRequest("POST", endpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("accept", "application/json")

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return nil, err
	}
	defer trace.Response.Body.Close()

	// Parse the JSON response into a struct for the raw response.
	var fr struct {
		FileName         string `json:"file_name"`
		FileSize         int64  `json:"file_size"`
		FileContentType  string `json:"file_content_type"`
		FileExtension    string `json:"file_extension_type"`
		FileHash         string `json:"file_hash"`
		FileSecurityStat string `json:"file_security_status"`
	}
	if err := json.NewDecoder(trace.Response.Body).Decode(&fr); err != nil {
		return nil, err
	}
	fileResp := File{
		Name:            fr.FileName,
		FileSizeInBytes: int(fr.FileSize),
		ContentType:     fr.FileContentType,
		FileExtension:   fr.FileExtension,
		FileHash:        fr.FileHash,
	}
	return &fileResp, nil
}

type Conversation struct {
	ConversationID     string                   `json:"conversation_id"`
	Status             string                   `json:"status"`
	ChannelID          string                   `json:"channel_id"`
	AssignedAgentID    string                   `json:"assigned_agent_id"`
	AssignedOrgAgentID string                   `json:"assigned_org_agent_id"`
	AssignedGroupID    string                   `json:"assigned_group_id"`
	AssignedOrgGroupID string                   `json:"assigned_org_group_id"`
	Messages           []Message                `json:"messages"`
	Properties         []map[string]interface{} `json:"properties"` //Array of custom properties
	Users              []User                   `json:"users"`
	UserID             string                   `json:"user_id"`
}

type Message struct {
	ID               string                 `json:"id"`
	CreatedTime      string                 `json:"created_time"`
	MessagesPart     []MessagesPart         `json:"messages_part"`
	ReplyParts       []MessagesPart         `json:"reply_parts"`
	AppID            string                 `json:"app_id"`
	ActorType        string                 `json:"actor_type"`
	ActorID          string                 `json:"actor_id"`
	OrgActorID       string                 `json:"org_actor_id"`
	ChannelID        string                 `json:"channel_id"`
	ConversationID   string                 `json:"conversation_id"`
	InterationID     string                 `json:"interation_id"`
	MessageType      string                 `json:"message_type"`
	UserID           string                 `json:"user_id"`
	MetaData         map[string]interface{} `json:"meta_data"`
	InReplyTo        map[string]interface{} `json:"in_reply_to"`
	ErrorCode        int                    `json:"error_code"`
	Status           string                 `json:"status"`
	ErrorMessage     string                 `json:"error_message"`
	ErrorCategory    string                 `json:"error_category"`
	RestrictResponse bool                   `json:"restrict_response"`
	BotsPrivateNote  bool                   `json:"bots_private_note"`
}

type MessagesPart struct {
	File             *File             `json:"file"`
	Text             *Text             `json:"text"`
	Image            *Image            `json:"image"`
	Video            *Video            `json:"video"`
	Collection       *Collection       `json:"collection"`
	URLButton        *URLButton        `json:"url_button"`
	QuickReplyButton *QuickReplyButton `json:"quick_reply_button"`
	TemplateContent  *TemplateContent  `json:"template_content"`
	Callback         *Callback         `json:"callback"`
	// AttachmentInput *AttachmentInput `json:"attachment_input"`
	// Reference *Reference `json:"reference"`
	// TextInput *TextInput `json:"text_input"`
	// HelpText *HelpText `json:"help_text"`
}

type File struct {
	FileHash        string `json:"fileHash"`
	FileSource      string `json:"fileSource"` //Possible values: FRESHCHAT, FRESHBOTS
	Name            string `json:"name"`
	URL             string `json:"url"`
	FileSizeInBytes int    `json:"file_size_in_bytes"`
	ContentType     string `json:"content_type"`
	FileExtension   string `json:"file_extension"`
}

type Text struct {
	Content string `json:"content"`
}

type Image struct {
	URL string `json:"url"`
}

type Video struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
}

type Collection struct {
	SubParts []MessagesPart `json:"sub_parts"`
}

type URLButton struct {
	URL    string `json:"url"`
	Label  string `json:"label"`
	Target string `json:"target"` //Possible values:_self, _blank | By default, _blank is the value of url_button.target.
}

type QuickReplyButton struct {
	CustomReplyText string `json:"custom_reply_text"`
	Label           string `json:"label"`
	Payload         string `json:"payload"`
	DisplayOrder    string `json:"display_order"`
	Type            string `json:"type"` //Possible value: RESEND_OTP
}

type TemplateContent struct {
	Type     string    `json:"type"` //Possible value: carousel, carousel_card_default, quick_reply_dropdown
	Sections []Section `json:"sections"`
}

type Section struct {
	Name  string         `json:"name"`
	Parts []MessagesPart `json:"parts"`
}

type Callback struct {
	Payload string `json:"payload"`
	Label   string `json:"label"`
}

type Choice struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type User struct {
	ID          string         `json:"id"`
	Email       string         `json:"email"`
	FirstName   string         `json:"first_name"`
	LastName    string         `json:"last_name"`
	Phone       string         `json:"phone"`
	CreatedTime string         `json:"created_time"`
	UpdatedTime string         `json:"updated_time"`
	ReferenceID string         `json:"reference_id"`
	Properties  []UserProperty `json:"properties"`
}

type UserProperty struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UpdatedTime string `json:"updated_time"`
	Icon        struct {
		URL string `json:"url"`
	}
	Enabled        bool     `json:"enabled"`
	Public         bool     `json:"public"`
	Tags           []string `json:"tags"`
	WelcomeMessage struct {
		MessageParts []MessagesPart `json:"message_parts"`
		MessageType  string         `json:"message_type"`
	} `json:"welcome_message"`
	Locale string `json:"locale"`
}

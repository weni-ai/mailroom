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
	var bodyData []byte

	if payload != nil {
		data, err := jsonx.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyData = data
		body = bytes.NewReader(bodyData)
	}

	req, err := httpx.NewRequest(method, url, body, headers)
	if err != nil {
		return nil, err
	}

	// Debug: print request and response for tracing
	fmt.Printf("[Freshchat Debug] Request: %s %s\n", req.Method, req.URL.String())
	if len(bodyData) > 0 {
		fmt.Printf("[Freshchat Debug] Request Body: %s\n", string(bodyData))
	}
	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		fmt.Printf("[Freshchat Debug] Error: %v\n", err)
		fmt.Printf("[Freshchat Debug] Response Body: %s\n", string(trace.ResponseBody))
		return trace, err
	}

	fmt.Printf("[Freshchat Debug] Response Status: %d\n", trace.Response.StatusCode)
	fmt.Printf("[Freshchat Debug] Response Body: %s\n", string(trace.ResponseBody))

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		err := jsonx.Unmarshal(trace.ResponseBody, response)
		if err != nil {
			fmt.Printf("[Freshchat Debug] Unmarshal Error: %v\n", err)
			return trace, err
		}
		fmt.Printf("[Freshchat Debug] API Error: %v\n", response)
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
	endpoint := "v2/conversations"
	response := &Conversation{}
	trace, err := c.post(endpoint, conversation, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) CreateUser(user *User) (*User, *httpx.Trace, error) {
	endpoint := "v2/users"
	response := &User{}
	trace, err := c.post(endpoint, user, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) GetUser(userID string) (*User, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("v2/users?reference_id=%s", userID)
	response := &struct {
		Users []User `json:"users"`
	}{}
	trace, err := c.get(endpoint, nil, response)
	if err != nil {
		return nil, trace, err
	}
	if len(response.Users) == 0 || response.Users[0].ReferenceID != userID {
		return nil, trace, fmt.Errorf("user not found")
	}
	return &response.Users[0], trace, nil
}

func (c *Client) GetChannels() ([]Channel, *httpx.Trace, error) {
	endpoint := "v2/channels"
	var response struct {
		Channels []Channel `json:"channels"`
	}
	trace, err := c.get(endpoint, nil, &response)
	if err != nil {
		return nil, trace, err
	}
	return response.Channels, trace, nil
}

func (c *Client) CreateMessage(message *Message) (*Message, *httpx.Trace, error) {
	endpoint := fmt.Sprintf("v2/conversations/%s/messages", message.ConversationID)
	response := &Message{}
	trace, err := c.post(endpoint, message, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) UpdateConversation(conversation *Conversation) (*httpx.Trace, error) {
	endpoint := fmt.Sprintf("v2/conversations/%s", string(conversation.ConversationID))
	response := &Conversation{}
	trace, err := c.put(endpoint, conversation, response)
	if err != nil {
		return trace, err
	}
	return trace, err
}

func (c *Client) UploadImage(imageURL string) (string, error) {
	getReq, err := httpx.NewRequest("GET", imageURL, nil, nil)
	if err != nil {
		return "", err
	}

	getTrace, err := httpx.DoTrace(c.httpClient, getReq, c.httpRetries, nil, -1)
	if err != nil {
		return "", err
	}

	if getTrace.Response.StatusCode >= 400 {
		return "", fmt.Errorf("failed to download image: %d", getTrace.Response.StatusCode)
	}

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
	_, err = io.Copy(part, bytes.NewReader(getTrace.ResponseBody))
	if err != nil {
		return "", err
	}
	writer.Close()

	endpoint := c.baseURL + "/v2/images/upload"
	postReq, err := http.NewRequest("POST", endpoint, &buf)
	if err != nil {
		return "", err
	}
	postReq.Header.Set("Authorization", "Bearer "+c.authToken)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.Header.Set("accept", "application/json")

	postTrace, err := httpx.DoTrace(c.httpClient, postReq, c.httpRetries, nil, -1)
	if err != nil {
		return "", err
	}

	if postTrace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		err := json.Unmarshal(postTrace.ResponseBody, response)
		if err != nil {
			return "", err
		}
		return "", errors.New(response.Error)
	}

	var imgResp Image
	if err := json.Unmarshal(postTrace.ResponseBody, &imgResp); err != nil {
		return "", err
	}
	return imgResp.URL, nil
}

func (c *Client) UploadFile(fileURL string) (*File, error) {
	getReq, err := httpx.NewRequest("GET", fileURL, nil, nil)
	if err != nil {
		return nil, err
	}

	getTrace, err := httpx.DoTrace(c.httpClient, getReq, c.httpRetries, nil, -1)
	if err != nil {
		return nil, err
	}

	if getTrace.Response.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to download file: %d", getTrace.Response.StatusCode)
	}

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
	_, err = io.Copy(part, bytes.NewReader(getTrace.ResponseBody))
	if err != nil {
		return nil, err
	}
	writer.Close()

	endpoint := c.baseURL + "/v2/files/upload"
	postReq, err := http.NewRequest("POST", endpoint, &buf)
	if err != nil {
		return nil, err
	}
	postReq.Header.Set("Authorization", "Bearer "+c.authToken)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.Header.Set("accept", "application/json")

	postTrace, err := httpx.DoTrace(c.httpClient, postReq, c.httpRetries, nil, -1)
	if err != nil {
		return nil, err
	}

	if postTrace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		err := json.Unmarshal(postTrace.ResponseBody, response)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(response.Error)
	}

	// Parse the JSON response into a struct for the raw response.
	var fr struct {
		FileName         string `json:"file_name"`
		FileSize         int64  `json:"file_size"`
		FileContentType  string `json:"file_content_type"`
		FileExtension    string `json:"file_extension_type"`
		FileHash         string `json:"file_hash"`
		FileSecurityStat string `json:"file_security_status"`
	}
	if err := json.Unmarshal(postTrace.ResponseBody, &fr); err != nil {
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
	ConversationID     string                   `json:"conversation_id,omitempty"`
	Status             string                   `json:"status,omitempty"`
	ChannelID          string                   `json:"channel_id,omitempty"`
	AssignedAgentID    string                   `json:"assigned_agent_id,omitempty"`
	AssignedOrgAgentID string                   `json:"assigned_org_agent_id,omitempty"`
	AssignedGroupID    string                   `json:"assigned_group_id,omitempty"`
	AssignedOrgGroupID string                   `json:"assigned_org_group_id,omitempty"`
	Messages           []Message                `json:"messages,omitempty"`
	Properties         []map[string]interface{} `json:"properties,omitempty"` //Array of custom properties
	Users              []User                   `json:"users,omitempty"`
	UserID             string                   `json:"user_id,omitempty"`
}

type Message struct {
	ID               string                 `json:"id,omitempty"`
	CreatedTime      string                 `json:"created_time,omitempty"`
	MessagesPart     []MessagesPart         `json:"messages_part,omitempty"`
	ReplyParts       []MessagesPart         `json:"reply_parts,omitempty"`
	AppID            string                 `json:"app_id,omitempty"`
	ActorType        string                 `json:"actor_type,omitempty"`
	ActorID          string                 `json:"actor_id,omitempty"`
	OrgActorID       string                 `json:"org_actor_id,omitempty"`
	ChannelID        string                 `json:"channel_id,omitempty"`
	ConversationID   string                 `json:"conversation_id,omitempty"`
	InterationID     string                 `json:"interation_id,omitempty"`
	MessageType      string                 `json:"message_type,omitempty"`
	UserID           string                 `json:"user_id,omitempty"`
	MetaData         map[string]interface{} `json:"meta_data,omitempty"`
	InReplyTo        map[string]interface{} `json:"in_reply_to,omitempty"`
	ErrorCode        int                    `json:"error_code,omitempty"`
	Status           string                 `json:"status,omitempty"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	ErrorCategory    string                 `json:"error_category,omitempty"`
	RestrictResponse bool                   `json:"restrict_response,omitempty"`
	BotsPrivateNote  bool                   `json:"bots_private_note,omitempty"`
}

type MessagesPart struct {
	File             *File             `json:"file,omitempty"`
	Text             *Text             `json:"text,omitempty"`
	Image            *Image            `json:"image,omitempty"`
	Video            *Video            `json:"video,omitempty"`
	Collection       *Collection       `json:"collection,omitempty"`
	URLButton        *URLButton        `json:"url_button,omitempty"`
	QuickReplyButton *QuickReplyButton `json:"quick_reply_button,omitempty"`
	TemplateContent  *TemplateContent  `json:"template_content,omitempty"`
	Callback         *Callback         `json:"callback,omitempty"`
	// AttachmentInput *AttachmentInput `json:"attachment_input"`
	// Reference *Reference `json:"reference"`
	// TextInput *TextInput `json:"text_input"`
	// HelpText *HelpText `json:"help_text"`
}

type File struct {
	FileHash        string `json:"fileHash,omitempty"`
	FileSource      string `json:"fileSource,omitempty"` //Possible values: FRESHCHAT, FRESHBOTS
	Name            string `json:"name,omitempty"`
	URL             string `json:"url,omitempty"`
	FileSizeInBytes int    `json:"file_size_in_bytes,omitempty"`
	ContentType     string `json:"content_type,omitempty"`
	FileExtension   string `json:"file_extension,omitempty"`
}

type Text struct {
	Content string `json:"content,omitempty"`
}

type Image struct {
	URL string `json:"url,omitempty"`
}

type Video struct {
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type Collection struct {
	SubParts []MessagesPart `json:"sub_parts,omitempty"`
}

type URLButton struct {
	URL    string `json:"url,omitempty"`
	Label  string `json:"label,omitempty"`
	Target string `json:"target,omitempty"` //Possible values:_self, _blank | By default, _blank is the value of url_button.target.
}

type QuickReplyButton struct {
	CustomReplyText string `json:"custom_reply_text,omitempty"`
	Label           string `json:"label,omitempty"`
	Payload         string `json:"payload,omitempty"`
	DisplayOrder    string `json:"display_order,omitempty"`
	Type            string `json:"type,omitempty"` //Possible value: RESEND_OTP
}

type TemplateContent struct {
	Type     string    `json:"type,omitempty"` //Possible value: carousel, carousel_card_default, quick_reply_dropdown
	Sections []Section `json:"sections,omitempty"`
}

type Section struct {
	Name  string         `json:"name,omitempty"`
	Parts []MessagesPart `json:"parts,omitempty"`
}

type Callback struct {
	Payload string `json:"payload,omitempty"`
	Label   string `json:"label,omitempty"`
}

type Choice struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
}

type User struct {
	ID          string         `json:"id,omitempty"`
	Email       string         `json:"email,omitempty"`
	FirstName   string         `json:"first_name,omitempty"`
	LastName    string         `json:"last_name,omitempty"`
	Phone       string         `json:"phone,omitempty"`
	CreatedTime string         `json:"created_time,omitempty"`
	UpdatedTime string         `json:"updated_time,omitempty"`
	ReferenceID string         `json:"reference_id,omitempty"`
	Properties  []UserProperty `json:"properties,omitempty"`
}

type UserProperty struct {
	Name  string      `json:"name,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

type Channel struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	UpdatedTime string `json:"updated_time,omitempty"`
	Icon        struct {
		URL string `json:"url,omitempty"`
	} `json:"icon,omitempty"`
	Enabled        bool     `json:"enabled,omitempty"`
	Public         bool     `json:"public,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	WelcomeMessage struct {
		MessageParts []MessagesPart `json:"message_parts,omitempty"`
		MessageType  string         `json:"message_type,omitempty"`
	} `json:"welcome_message,omitempty"`
	Locale string `json:"locale,omitempty"`
}

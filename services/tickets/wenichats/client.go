package wenichats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type baseClient struct {
	httpClient   *http.Client
	httpRetries  *httpx.RetryConfig
	authToken    string
	baseURL      string
	rt           *runtime.Runtime
	clientID     string
	clientSecret string
	mu           sync.RWMutex
	token        string
	expiresAt    time.Time
}

func newBaseClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, authToken string) baseClient {

	return baseClient{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		authToken:   authToken,
		baseURL:     baseURL,
	}
}

type errorResponse struct {
	Detail string `json:"detail"`
}

func (c *baseClient) request(method, url string, params *url.Values, payload, response interface{}) (*httpx.Trace, error) {
	pjson, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	data := strings.NewReader(string(pjson))
	req, err := httpx.NewRequest(method, url, data, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.authToken)

	if params != nil {
		req.URL.RawQuery = params.Encode()
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		response := &errorResponse{}
		err := jsonx.Unmarshal(trace.ResponseBody, response)
		if err != nil {
			return trace, errors.Wrap(err, fmt.Sprintf("couldn't parse error response: %v", string(trace.ResponseBody)))
		}
		return trace, errors.New(response.Detail)
	}

	if response != nil {
		err = json.Unmarshal(trace.ResponseBody, response)
		return trace, errors.Wrap(err, "couldn't parse response body")
	}

	return trace, nil
}

func (c *baseClient) post(url string, payload, response interface{}) (*httpx.Trace, error) {
	return c.request("POST", url, nil, payload, response)
}

func (c *baseClient) get(url string, params *url.Values, response interface{}) (*httpx.Trace, error) {
	return c.request("GET", url, params, nil, response)
}

func (c *baseClient) patch(url string, params *url.Values, payload, response interface{}) (*httpx.Trace, error) {
	return c.request("PATCH", url, nil, payload, response)
}

type Client struct {
	baseClient
}

func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, authToken string) *Client {
	return &Client{
		baseClient: newBaseClient(httpClient, httpRetries, baseURL, authToken),
	}
}

func (c *Client) CreateRoom(room *RoomRequest) (*RoomResponse, *httpx.Trace, error) {
	url := c.baseURL + "/rooms/"
	response := &RoomResponse{}
	trace, err := c.post(url, room, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) UpdateRoom(roomUUID string, room *RoomRequest) (*RoomResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("%s/rooms/%s/", c.baseURL, roomUUID)
	response := &RoomResponse{}
	trace, err := c.patch(url, nil, room, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) CloseRoom(roomUUID string) (*RoomResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("%s/rooms/%s/close/", c.baseURL, roomUUID)
	response := &RoomResponse{}
	trace, err := c.patch(url, nil, nil, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) SendHistoryBatch(roomUUID string, history []HistoryMessage) (*httpx.Trace, error) {
	url := fmt.Sprintf("%s/rooms/%s/history/", c.baseURL, roomUUID)
	return c.post(url, history, nil)
}

func (c *Client) CreateMessage(msg *MessageRequest) (*MessageResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("%s/msgs/", c.baseURL)
	response := &MessageResponse{}
	trace, err := c.post(url, msg, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

func (c *Client) GetQueues(params *url.Values) (*QueuesResponse, *httpx.Trace, error) {
	url := fmt.Sprintf("%s/queues/", c.baseURL)
	response := &QueuesResponse{}
	trace, err := c.get(url, params, response)
	if err != nil {
		return nil, trace, err
	}
	return response, trace, nil
}

type ProjectInfo struct {
	ProjectUUID string `json:"uuid,omitempty"`
	ProjectName string `json:"name,omitempty"`
}

type RoomRequest struct {
	TicketUUID   string                 `json:"ticket_uuid,omitempty"`
	QueueUUID    string                 `json:"queue_uuid,omitempty"`
	UserEmail    string                 `json:"user_email,omitempty"`
	SectorUUID   string                 `json:"sector_uuid,omitempty"`
	Contact      *Contact               `json:"contact,omitempty"`
	CreatedOn    *time.Time             `json:"created_on,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	CallbackURL  string                 `json:"callback_url,omitempty"`
	FlowUUID     assets.FlowUUID        `json:"flow_uuid,omitempty"`
	IsAnon       bool                   `json:"is_anon,omitempty"`
	History      []HistoryMessage       `json:"history,omitempty"`
	ProjectInfo  *ProjectInfo           `json:"project_info,omitempty"`
}

type Contact struct {
	ExternalID   string                 `json:"external_id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Email        string                 `json:"email,omitempty"`
	Phone        string                 `json:"phone,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	URN          string                 `json:"urn,omitempty"`
	Groups       []Group                `json:"groups,omitempty"`
}

type RoomResponse struct {
	UUID string `json:"uuid"`
	User struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
	} `json:"user"`
	Contact struct {
		ExternalID   string                 `json:"external_id"`
		Name         string                 `json:"name"`
		Email        string                 `json:"email"`
		Status       string                 `json:"status"`
		Phone        string                 `json:"phone"`
		CustomFields map[string]interface{} `json:"custom_fields"`
		CreatedOn    time.Time              `json:"created_on"`
	} `json:"contact"`
	Queue struct {
		UUID       string    `json:"uuid"`
		CreatedOn  time.Time `json:"created_on"`
		ModifiedOn time.Time `json:"modified_on"`
		Name       string    `json:"name"`
		Sector     string    `json:"sector"`
	} `json:"queue"`
	CreatedOn    time.Time              `json:"created_on"`
	ModifiedOn   time.Time              `json:"modified_on"`
	IsActive     bool                   `json:"is_active"`
	CustomFields map[string]interface{} `json:"custom_fields"`
	CallbackURL  string                 `json:"callback_url"`
}

type HistoryMessage struct {
	Text        string       `json:"text"`
	Direction   string       `json:"direction"`
	Attachments []Attachment `json:"attachments"`
	CreatedOn   time.Time    `json:"created_on"`
}

type MessageRequest struct {
	Room        string          `json:"room"`
	Text        string          `json:"text"`
	CreatedOn   time.Time       `json:"created_on"`
	Direction   string          `json:"direction"`
	Attachments []Attachment    `json:"attachments"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type MessageResponse struct {
	UUID    string      `json:"uuid"`
	User    interface{} `json:"user"`
	Room    string      `json:"room"`
	Contact struct {
		UUID         string `json:"uuid"`
		Name         string `json:"name"`
		Email        string `json:"email"`
		Status       string `json:"status"`
		Phone        string `json:"phone"`
		CustomFields struct {
		} `json:"custom_fields"`
		CreatedOn time.Time `json:"created_on"`
	} `json:"contact"`
	Text      string       `json:"text"`
	Seen      bool         `json:"seen"`
	Media     []Attachment `json:"media"`
	CreatedOn string       `json:"created_on"`
}

type Attachment struct {
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

type baseResponse struct {
	Count    int    `json:"count"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

type QueuesResponse struct {
	baseResponse
	Results []Queue `json:"results"`
}

type Queue struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type Group struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

const (
	tokenCacheKey = "internal-user-token"
	tokenExpiry   = 6 * time.Hour
	lockTimeout   = 10 * time.Second
)

type authResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getToken returns a valid authentication token, fetching a new one if necessary
func (c *Client) getToken(ctx context.Context) (string, error) {
	// First check memory cache
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.expiresAt) {
		token := c.token
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Check Redis cache
	rc := c.rt.RP.Get()
	defer rc.Close()

	// Try to get lock for token renewal
	lockKey := tokenCacheKey + ":lock"
	lockAcquired, err := redis.String(rc.Do("SET", lockKey, "1", "NX", "EX", int(lockTimeout.Seconds())))
	if err != nil && err != redis.ErrNil {
		return "", errors.Wrap(err, "error acquiring lock for token renewal")
	}

	if lockAcquired != "" {
		// We have the lock, fetch a new token
		defer rc.Do("DEL", lockKey) // Ensure lock is released

		logrus.WithFields(logrus.Fields{
			"component": "wenichats_client",
		}).Debug("Acquired lock for token renewal")

		token, expiry, err := c.fetchNewToken(ctx)
		if err != nil {
			return "", errors.Wrap(err, "error fetching new token")
		}

		// Save token to Redis
		_, err = rc.Do("SETEX", tokenCacheKey, int(tokenExpiry.Seconds()), token)
		if err != nil {
			logrus.WithError(err).Error("Failed to save token to Redis")
		}

		// Update in-memory cache
		c.mu.Lock()
		c.token = token
		c.expiresAt = expiry
		c.mu.Unlock()

		return token, nil
	}

	// We didn't get the lock, check if token exists in Redis
	token, err := redis.String(rc.Do("GET", tokenCacheKey))
	if err == nil && token != "" {
		// Token found in Redis
		ttl, _ := redis.Int(rc.Do("TTL", tokenCacheKey))

		// Update in-memory cache
		c.mu.Lock()
		c.token = token
		c.expiresAt = time.Now().Add(time.Duration(ttl) * time.Second)
		c.mu.Unlock()

		return token, nil
	}

	if err != redis.ErrNil {
		logrus.WithError(err).Error("Error getting token from Redis")
	}

	// Wait briefly and try again (another process may be renewing)
	time.Sleep(time.Millisecond * 100)
	return c.getToken(ctx)
}

// fetchNewToken calls the auth API to get a new token
func (c *Client) fetchNewToken(ctx context.Context) (string, time.Time, error) {
	authURL := fmt.Sprintf("%s/auth/token", c.baseURL)

	reqBody := strings.NewReader(fmt.Sprintf(
		"client_id=%s&client_secret=%s&grant_type=client_credentials",
		c.clientID, c.clientSecret,
	))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, reqBody)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "error creating auth request")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, "error making auth request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, errors.Errorf("auth request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", time.Time{}, errors.Wrap(err, "error decoding auth response")
	}

	// Calculate expiration time (use 90% of actual expiry to refresh before it expires)
	expiryDuration := time.Duration(authResp.ExpiresIn) * time.Second
	safeExpiry := time.Now().Add(expiryDuration * 9 / 10)

	logrus.WithFields(logrus.Fields{
		"component":  "wenichats_client",
		"expires_in": authResp.ExpiresIn,
	}).Debug("Successfully fetched new token")

	return authResp.AccessToken, safeExpiry, nil
}

// GetTickets retrieves tickets from Wenichats API
func (c *Client) GetTickets(ctx context.Context, filter map[string]string) ([]map[string]interface{}, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get auth token")
	}

	endpoint := fmt.Sprintf("%s/tickets", c.baseURL)

	// Add query parameters for filtering
	if len(filter) > 0 {
		queryParams := make([]string, 0, len(filter))
		for k, v := range filter {
			queryParams = append(queryParams, fmt.Sprintf("%s=%s", k, v))
		}
		endpoint = endpoint + "?" + strings.Join(queryParams, "&")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating request")
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching tickets")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("tickets request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tickets []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tickets); err != nil {
		return nil, errors.Wrap(err, "error decoding tickets response")
	}

	return tickets, nil
}

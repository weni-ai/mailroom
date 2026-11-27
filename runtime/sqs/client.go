package sqs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Message represents a publishable message payload.
// Different formats can implement this interface.
type Message interface {
	Marshal() ([]byte, error)
	ContentType() string
}

// RawMessage implements Message for already-encoded payloads.
type RawMessage struct {
	Body []byte
	Type string
}

func (m RawMessage) Marshal() ([]byte, error) { return m.Body, nil }
func (m RawMessage) ContentType() string      { return m.Type }

// Client is a thin SQS publisher with robust connection handling.
type Client struct {
	region          string
	accessKeyID     string
	secretAccessKey string
	endpoint        string

	// mu only guards the pointer to the current service and its generation, it is never held during a network call,
	// so a send never waits for another send or for a reconnect.
	mu  sync.RWMutex
	svc *sqs.Client
	gen uint64 // incremented every time svc is replaced, tells a failing send whether somebody already reconnected

	// connectMu serializes reconnects only
	connectMu sync.Mutex

	// newService builds a service; nil means buildService. Replaced in tests.
	newService func() (*sqs.Client, error)
}

// ClientConfig holds the configuration for creating an SQS client.
type ClientConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // Optional: for local testing with LocalStack
}

// New creates a client connected to SQS.
func New(cfg ClientConfig) (*Client, error) {
	c := &Client{
		region:          cfg.Region,
		accessKeyID:     cfg.AccessKeyID,
		secretAccessKey: cfg.SecretAccessKey,
		endpoint:        cfg.Endpoint,
	}
	if _, err := c.reconnect(0); err != nil {
		return nil, err
	}
	return c, nil
}

// SendTo publishes a message to the specified SQS queue URL.
func (c *Client) SendTo(queueURL string, msg Message) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}
	if queueURL == "" {
		return errors.New("queueURL must be non-empty")
	}

	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	return c.send(buildInput(queueURL, body, msg.ContentType(), nil))
}

// SendToWithAttributes publishes a message to the specified SQS queue URL with message attributes.
func (c *Client) SendToWithAttributes(queueURL string, msg Message, attributes map[string]string) error {
	if msg == nil {
		return errors.New("message cannot be nil")
	}
	if queueURL == "" {
		return errors.New("queueURL must be non-empty")
	}

	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	return c.send(buildInput(queueURL, body, msg.ContentType(), attributes))
}

// Close releases the SQS client resources.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// sends already in flight keep their own reference to the service and finish normally
	c.svc = nil
}

// internal helpers

// isFIFOQueue returns true if the queue URL indicates a FIFO queue
func isFIFOQueue(queueURL string) bool {
	return strings.HasSuffix(queueURL, ".fifo")
}

// sqsFIFOMessageGroupIDLimit is the maximum length SQS accepts for MessageGroupId.
// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_SendMessage.html
const sqsFIFOMessageGroupIDLimit = 128

// generateDeduplicationID generates a SHA256 hash of the message body for deduplication
func generateDeduplicationID(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// fitMessageGroupID returns a MessageGroupId that SQS FIFO will accept.
// Keys within the 128-character limit are kept so existing conversations stay
// in the same group. Longer keys (typically a long contact URN) are replaced
// with their SHA-256 hex digest, which is 64 characters. The same input always
// maps to the same digest, so messages for that contact remain ordered.
func fitMessageGroupID(groupID string) string {
	if len(groupID) <= sqsFIFOMessageGroupIDLimit {
		return groupID
	}
	sum := sha256.Sum256([]byte(groupID))
	logrus.WithField("message_group_id_length", len(groupID)).Warn("sqs message group id exceeds 128 characters; using sha256 digest")
	return hex.EncodeToString(sum[:])
}

// sendTimeout bounds every SendMessage call
const sendTimeout = 5 * time.Second

// buildInput builds the SendMessage request. The same request is used for the retry after a reconnect.
func buildInput(queueURL string, body []byte, contentType string, attributes map[string]string) *sqs.SendMessageInput {
	msgAttrs := map[string]types.MessageAttributeValue{
		"ContentType": {
			DataType:    aws.String("String"),
			StringValue: aws.String(contentType),
		},
	}

	for k, v := range attributes {
		msgAttrs[k] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(v),
		}
	}

	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: msgAttrs,
	}

	// Add FIFO-specific parameters if this is a FIFO queue
	if isFIFOQueue(queueURL) {
		// Use MessageGroupId from attributes if provided, otherwise use "default"
		messageGroupId := "default"
		if groupId, ok := attributes["MessageGroupId"]; ok {
			messageGroupId = groupId
		}
		input.MessageGroupId = aws.String(fitMessageGroupID(messageGroupId))
		input.MessageDeduplicationId = aws.String(generateDeduplicationID(body))
	}

	return input
}

// send sends the message and, if that fails, reconnects and tries once more.
//
// The service is read under a short lock and then used without any lock, so a slow or hanging send never holds
// back the others. When several sends fail together they all see the same generation, and only the first one to
// reach reconnect rebuilds the client; the others just pick up the new one.
func (c *Client) send(input *sqs.SendMessageInput) error {
	svc, gen := c.snapshot()

	var err error
	if svc == nil {
		err = errors.New("sqs client not connected")
	} else {
		err = sendOnce(svc, input)
	}
	if err == nil {
		return nil
	}

	logrus.WithError(err).Warn("sqs publish failed, attempting reconnect")
	svc, rerr := c.reconnect(gen)
	if rerr != nil {
		return rerr
	}
	return sendOnce(svc, input)
}

func sendOnce(svc *sqs.Client, input *sqs.SendMessageInput) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	_, err := svc.SendMessage(ctx, input)
	return err
}

// snapshot returns the current service and its generation
func (c *Client) snapshot() (*sqs.Client, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.svc, c.gen
}

// reconnect returns a service newer than the generation the caller saw failing. If somebody already replaced it, that
// one is returned and nothing is rebuilt; otherwise a new one is built (outside mu, it may reach the metadata
// service) and swapped in.
func (c *Client) reconnect(seen uint64) (*sqs.Client, error) {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()

	if svc, gen := c.snapshot(); svc != nil && gen != seen {
		return svc, nil
	}

	build := c.newService
	if build == nil {
		build = c.buildService
	}
	svc, err := build()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.svc = svc
	c.gen++
	c.mu.Unlock()
	return svc, nil
}

// buildService creates the SQS service from the client configuration
func (c *Client) buildService() (*sqs.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build config options
	var opts []func(*config.LoadOptions) error

	// Set region if provided
	if c.region != "" {
		opts = append(opts, config.WithRegion(c.region))
	}

	// Use explicit credentials if provided
	if c.accessKeyID != "" && c.secretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretAccessKey, ""),
		))
	}

	// Load default config with options (supports IRSA, EC2 metadata, env vars, etc.)
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS config")
	}

	// Create SQS client with optional custom endpoint
	var sqsOpts []func(*sqs.Options)
	if c.endpoint != "" {
		sqsOpts = append(sqsOpts, func(o *sqs.Options) {
			o.BaseEndpoint = aws.String(c.endpoint)
		})
	}

	return sqs.NewFromConfig(cfg, sqsOpts...), nil
}

// JSONMessage is a helper struct for JSON messages.
type JSONMessage struct {
	Data interface{}
}

func (m JSONMessage) Marshal() ([]byte, error) {
	return json.Marshal(m.Data)
}

func (m JSONMessage) ContentType() string {
	return "application/json"
}

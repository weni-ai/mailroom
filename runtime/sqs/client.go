package sqs

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
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

	mu      sync.Mutex
	session *session.Session
	svc     *sqs.SQS
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
	if err := c.connect(); err != nil {
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

	if err := c.publish(queueURL, body, msg.ContentType()); err != nil {
		logrus.WithError(err).Warn("sqs publish failed, attempting reconnect")
		if rerr := c.reconnect(); rerr != nil {
			return rerr
		}
		return c.publish(queueURL, body, msg.ContentType())
	}
	return nil
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

	if err := c.publishWithAttributes(queueURL, body, msg.ContentType(), attributes); err != nil {
		logrus.WithError(err).Warn("sqs publish failed, attempting reconnect")
		if rerr := c.reconnect(); rerr != nil {
			return rerr
		}
		return c.publishWithAttributes(queueURL, body, msg.ContentType(), attributes)
	}
	return nil
}

// Close releases the SQS client resources.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.svc = nil
	c.session = nil
}

// internal helpers

func (c *Client) publish(queueURL string, body []byte, contentType string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.svc == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(body)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"ContentType": {
				DataType:    aws.String("String"),
				StringValue: aws.String(contentType),
			},
		},
	}

	_, err := c.svc.SendMessageWithContext(ctx, input)
	return err
}

func (c *Client) publishWithAttributes(queueURL string, body []byte, contentType string, attributes map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.svc == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgAttrs := map[string]*sqs.MessageAttributeValue{
		"ContentType": {
			DataType:    aws.String("String"),
			StringValue: aws.String(contentType),
		},
	}

	for k, v := range attributes {
		msgAttrs[k] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(v),
		}
	}

	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: msgAttrs,
	}

	_, err := c.svc.SendMessageWithContext(ctx, input)
	return err
}

func (c *Client) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) connectLocked() error {
	// close any existing
	c.svc = nil
	c.session = nil

	awsConfig := &aws.Config{
		Region: aws.String(c.region),
	}

	// Use explicit credentials if provided
	if c.accessKeyID != "" && c.secretAccessKey != "" {
		awsConfig.Credentials = credentials.NewStaticCredentials(c.accessKeyID, c.secretAccessKey, "")
	}

	// Use custom endpoint if provided (for LocalStack or other S3-compatible services)
	if c.endpoint != "" {
		awsConfig.Endpoint = aws.String(c.endpoint)
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create AWS session")
	}

	c.session = sess
	c.svc = sqs.New(sess)
	return nil
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

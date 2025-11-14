package rmq

import (
	"context"
	"errors"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

// Message represents a publishable message payload.
// Different formats can implement this interface.
type Message interface {
	Marshal() ([]byte, error)
	ContentType() string
}

// (moved TicketMessage to core/tasks/rabbitmq)

// RawMessage implements Message for already-encoded payloads.
type RawMessage struct {
	Body []byte
	Type string
}

func (m RawMessage) Marshal() ([]byte, error) { return m.Body, nil }
func (m RawMessage) ContentType() string      { return m.Type }

// Client is a thin AMQP publisher with robust connection handling.
type Client struct {
	url string

	mu       sync.Mutex
	conn     *amqp.Connection
	ch       *amqp.Channel
	confirms <-chan amqp.Confirmation
}

// New creates a client connected to the provided URL.
func New(url string) (*Client, error) {
	c := &Client{url: url}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// SendTo publishes a message to the specified exchange and routing key, regardless of defaults.
func (c *Client) SendTo(exchange string, routingKey string, msg Message) error {
	if msg == nil {
		return amqp.ErrClosed
	}
	if exchange == "" || routingKey == "" {
		return errors.New("exchange and routingKey must be non-empty")
	}

	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	if err := c.publishTo(exchange, routingKey, body, msg.ContentType()); err != nil {
		logrus.WithError(err).Warn("rmq publish failed, attempting reconnect")
		if rerr := c.reconnect(); rerr != nil {
			return rerr
		}
		return c.publishTo(exchange, routingKey, body, msg.ContentType())
	}
	return nil
}

// Close releases channel and connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ch != nil {
		_ = c.ch.Close()
		c.ch = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// internal helpers

func (c *Client) publishTo(exchange string, routingKey string, body []byte, contentType string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.conn.IsClosed() || c.ch == nil {
		if err := c.connectLocked(); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  contentType,
		Body:         body,
		Timestamp:    time.Now(),
	}); err != nil {
		return err
	}

	// wait for broker confirm to ensure durability
	select {
	case conf, ok := <-c.confirms:
		if !ok || !conf.Ack {
			return amqp.ErrClosed
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	if c.ch != nil {
		_ = c.ch.Close()
		c.ch = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}

	conn, err := amqp.DialConfig(c.url, amqp.Config{Heartbeat: 10 * time.Second, Properties: amqp.Table{"product": "mailroom"}})
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	c.conn = conn
	c.ch = ch
	c.confirms = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	return nil
}

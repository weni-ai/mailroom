package rmq

import (
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingMessage struct{}

func (f failingMessage) Marshal() ([]byte, error) { return nil, errors.New("boom") }
func (f failingMessage) ContentType() string      { return "application/json" }

func TestRawMessage(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	m := RawMessage{Body: body, Type: "application/json"}

	b, err := m.Marshal()
	require.NoError(t, err)
	assert.Equal(t, body, b)
	assert.Equal(t, "application/json", m.ContentType())
}

func TestClientSendTo_ArgumentValidation(t *testing.T) {
	c := &Client{}

	// nil message
	err := c.SendTo("ex", "rk", nil)
	assert.Equal(t, amqp.ErrClosed, err)

	// empty exchange
	err = c.SendTo("", "rk", RawMessage{Body: []byte("{}"), Type: "application/json"})
	require.Error(t, err)
	assert.Equal(t, "exchange and routingKey must be non-empty", err.Error())

	// empty routing key
	err = c.SendTo("ex", "", RawMessage{Body: []byte("{}"), Type: "application/json"})
	require.Error(t, err)
	assert.Equal(t, "exchange and routingKey must be non-empty", err.Error())
}

func TestClientSendTo_MarshalError(t *testing.T) {
	c := &Client{}
	err := c.SendTo("ex", "rk", failingMessage{})
	assert.EqualError(t, err, "boom")
}

func TestClientSendTo_PublishFailureReconnectError(t *testing.T) {
	// zero-value client with empty URL will fail connect/publish and return an error
	c := &Client{}

	err := c.SendTo("ex", "rk", RawMessage{Body: []byte("{}"), Type: "application/json"})
	assert.Error(t, err)
}

func TestClientClose_NoPanic(t *testing.T) {
	c := &Client{}
	c.Close()
}

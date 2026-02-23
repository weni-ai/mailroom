package sqs

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// LocalStack default configuration
	localStackEndpoint  = "http://localhost:4566"
	localStackRegion    = "us-east-1"
	localStackAccessKey = "test"
	localStackSecretKey = "test"
	testQueueName       = "test-queue-foo"
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

func TestJSONMessage(t *testing.T) {
	data := map[string]string{"hello": "world"}
	m := JSONMessage{Data: data}

	b, err := m.Marshal()
	require.NoError(t, err)
	assert.Equal(t, `{"hello":"world"}`, string(b))
	assert.Equal(t, "application/json", m.ContentType())
}

func TestJSONMessage_MarshalError(t *testing.T) {
	// channels cannot be marshaled to JSON
	m := JSONMessage{Data: make(chan int)}
	_, err := m.Marshal()
	assert.Error(t, err)
}

func TestClientSendTo_ArgumentValidation(t *testing.T) {
	c := &Client{}

	// nil message
	err := c.SendTo("http://localhost:4566/000000000000/test-queue", nil)
	require.Error(t, err)
	assert.Equal(t, "message cannot be nil", err.Error())

	// empty queue URL
	err = c.SendTo("", RawMessage{Body: []byte("{}"), Type: "application/json"})
	require.Error(t, err)
	assert.Equal(t, "queueURL must be non-empty", err.Error())
}

func TestClientSendToWithAttributes_ArgumentValidation(t *testing.T) {
	c := &Client{}

	// nil message
	err := c.SendToWithAttributes("http://localhost:4566/000000000000/test-queue", nil, nil)
	require.Error(t, err)
	assert.Equal(t, "message cannot be nil", err.Error())

	// empty queue URL
	err = c.SendToWithAttributes("", RawMessage{Body: []byte("{}"), Type: "application/json"}, nil)
	require.Error(t, err)
	assert.Equal(t, "queueURL must be non-empty", err.Error())
}

func TestClientSendTo_MarshalError(t *testing.T) {
	c := &Client{}
	err := c.SendTo("http://localhost:4566/000000000000/test-queue", failingMessage{})
	assert.EqualError(t, err, "boom")
}

func TestClientSendToWithAttributes_MarshalError(t *testing.T) {
	c := &Client{}
	err := c.SendToWithAttributes("http://localhost:4566/000000000000/test-queue", failingMessage{}, nil)
	assert.EqualError(t, err, "boom")
}

func TestClientClose_NoPanic(t *testing.T) {
	c := &Client{}
	c.Close()
}

func TestClientNew_InvalidConfig(t *testing.T) {
	// empty region should still work (AWS SDK handles defaults)
	c, err := New(ClientConfig{
		Region:          "",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	})
	// This may succeed or fail depending on environment, just ensure no panic
	if err == nil {
		c.Close()
	}
}

// getTestQueueURL creates the test queue in LocalStack and returns its URL.
// Returns empty string if LocalStack is not available.
func getTestQueueURL(t *testing.T) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(localStackRegion),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(localStackAccessKey, localStackSecretKey, ""),
		),
	)
	if err != nil {
		return ""
	}

	svc := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(localStackEndpoint)
	})

	// Try to create the queue (will return existing queue URL if already exists)
	result, err := svc.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(testQueueName),
	})
	if err != nil {
		return ""
	}

	return *result.QueueUrl
}

// skipIfNoLocalStack skips the test if LocalStack is not available
func skipIfNoLocalStack(t *testing.T) string {
	if os.Getenv("TEST_SQS_LOCALSTACK") == "" {
		t.Skip("Skipping LocalStack integration test. Set TEST_SQS_LOCALSTACK=1 to run.")
	}

	queueURL := getTestQueueURL(t)
	if queueURL == "" {
		t.Skip("LocalStack SQS not available")
	}
	return queueURL
}

func TestClientNew_WithLocalStack(t *testing.T) {
	skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()

	assert.NotNil(t, c.svc)
}

func TestClientSendTo_WithLocalStack(t *testing.T) {
	queueURL := skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)
	defer c.Close()

	msg := RawMessage{
		Body: []byte(`{"test":"message"}`),
		Type: "application/json",
	}

	err = c.SendTo(queueURL, msg)
	assert.NoError(t, err)
}

func TestClientSendToWithAttributes_WithLocalStack(t *testing.T) {
	queueURL := skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)
	defer c.Close()

	msg := RawMessage{
		Body: []byte(`{"test":"message with attributes"}`),
		Type: "application/json",
	}

	attrs := map[string]string{
		"CustomAttribute": "custom-value",
		"AnotherAttr":     "another-value",
	}

	err = c.SendToWithAttributes(queueURL, msg, attrs)
	assert.NoError(t, err)
}

func TestClientSendTo_JSONMessage_WithLocalStack(t *testing.T) {
	queueURL := skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)
	defer c.Close()

	msg := JSONMessage{
		Data: map[string]interface{}{
			"action":  "test",
			"payload": map[string]string{"key": "value"},
		},
	}

	err = c.SendTo(queueURL, msg)
	assert.NoError(t, err)
}

func TestClientReconnect_WithLocalStack(t *testing.T) {
	queueURL := skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)
	defer c.Close()

	// Force a reconnect by clearing the service
	c.mu.Lock()
	c.svc = nil
	c.mu.Unlock()

	// SendTo should reconnect automatically
	msg := RawMessage{
		Body: []byte(`{"test":"after reconnect"}`),
		Type: "application/json",
	}

	err = c.SendTo(queueURL, msg)
	assert.NoError(t, err)
}

func TestClientClose_WithLocalStack(t *testing.T) {
	skipIfNoLocalStack(t)

	c, err := New(ClientConfig{
		Region:          localStackRegion,
		AccessKeyID:     localStackAccessKey,
		SecretAccessKey: localStackSecretKey,
		Endpoint:        localStackEndpoint,
	})
	require.NoError(t, err)

	// Close should not panic and should clear resources
	c.Close()

	assert.Nil(t, c.svc)
}

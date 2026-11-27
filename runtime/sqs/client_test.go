package sqs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
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

func TestFitMessageGroupID(t *testing.T) {
	shortID := "f06a5496-b00b-4f79-877b-511fd549ebc8:8b3d29f3-3ac5-49d3-a6a8-43c44ee7fc55:whatsapp:5511999999999"
	assert.LessOrEqual(t, len(shortID), 128)
	assert.Equal(t, shortID, fitMessageGroupID(shortID))

	// Real case: mailto URN pushed project:channel:urn past the FIFO limit (134).
	longID := "f06a5496-b00b-4f79-877b-511fd549ebc8:8b3d29f3-3ac5-49d3-a6a8-43c44ee7fc55:mailto:vinicius.conceicao+wt-8aae20dc@funchalnegocios.com.br"
	assert.Greater(t, len(longID), 128)

	fitted := fitMessageGroupID(longID)
	assert.Len(t, fitted, 64)
	assert.Equal(t, fitted, fitMessageGroupID(longID))
	assert.NotEqual(t, fitted, fitMessageGroupID(longID+"x"))
}

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

// fakeSQS starts an endpoint answering with the given handler and returns a client connected to it. The handler gets
// the 1-based number of the request and answers 200 when it returns 0.
func fakeSQS(t *testing.T, handle func(n int32, r *http.Request) (status int)) *Client {
	t.Helper()

	var n int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := handle(atomic.AddInt32(&n, 1), r)

		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"__type":"InvalidParameterValue","message":"test failure"}`))
			return
		}
		_, _ = w.Write([]byte(`{"MessageId":"00000000-0000-0000-0000-000000000000"}`))
	}))
	t.Cleanup(server.Close)

	c, err := New(ClientConfig{Region: "us-east-1", AccessKeyID: "test", SecretAccessKey: "test", Endpoint: server.URL})
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

func testMessage() RawMessage {
	return RawMessage{Body: []byte(`{"a":1}`), Type: "application/json"}
}

func generation(c *Client) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gen
}

// Q-10: sends failing at the same moment rebuild the client exactly once
func TestClientConcurrentFailuresReconnectOnce(t *testing.T) {
	const sends = 10

	// the first `sends` requests are held until all of them arrived, then all fail together
	release := make(chan struct{})
	c := fakeSQS(t, func(n int32, r *http.Request) int {
		if n > sends {
			return http.StatusOK
		}
		if n == sends {
			close(release)
		}
		<-release
		return http.StatusBadRequest
	})

	var builds int32
	c.newService = func() (*sqs.Client, error) {
		atomic.AddInt32(&builds, 1)
		return c.buildService()
	}
	before := generation(c)

	errs := make(chan error, sends)
	for i := 0; i < sends; i++ {
		go func() { errs <- c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()) }()
	}
	for i := 0; i < sends; i++ {
		assert.NoError(t, <-errs)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&builds), "one failure burst, one rebuild")
	assert.Equal(t, before+1, generation(c))
}

// Q-10: a send that did not fail never waits for the rebuild triggered by another one
func TestClientHealthySendDoesNotWaitForReconnect(t *testing.T) {
	c := fakeSQS(t, func(n int32, r *http.Request) int {
		if n == 1 {
			return http.StatusBadRequest
		}
		return http.StatusOK
	})

	entered := make(chan struct{})
	unblock := make(chan struct{})
	c.newService = func() (*sqs.Client, error) {
		close(entered)
		<-unblock
		return c.buildService()
	}

	failed := make(chan error, 1)
	go func() { failed <- c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()) }()

	// the failing send is now rebuilding the client and will stay there
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not start")
	}

	start := time.Now()
	require.NoError(t, c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()))
	assert.Less(t, time.Since(start), time.Second)

	close(unblock)
	assert.NoError(t, <-failed)
}

// no network call happens under a lock: a slow send does not hold back another one, and Close does not break it
func TestClientSlowSendDoesNotBlockOthers(t *testing.T) {
	c := fakeSQS(t, func(n int32, r *http.Request) int {
		if n == 1 {
			time.Sleep(700 * time.Millisecond)
		}
		return http.StatusOK
	})

	slow := make(chan error, 1)
	go func() { slow <- c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()) }()
	time.Sleep(150 * time.Millisecond)

	start := time.Now()
	require.NoError(t, c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()))
	assert.Less(t, time.Since(start), 400*time.Millisecond)

	// closing while the slow send is in flight must not break it
	c.Close()
	assert.NoError(t, <-slow)

	// and the client is usable again afterwards, reconnecting on demand
	assert.NoError(t, c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage()))
}

func TestClientReconnectKeepsConcurrentCallersSafe(t *testing.T) {
	c := fakeSQS(t, func(n int32, r *http.Request) int { return http.StatusOK })

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%5 == 0 {
				c.Close()
			}
			_ = c.SendTo("http://localhost:4566/000000000000/test-queue", testMessage())
		}(i)
	}
	wg.Wait()
}

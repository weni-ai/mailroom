package testsuite

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// SQSRequest is a SendMessage request captured by SQSServer.
type SQSRequest struct {
	QueueURL               string
	MessageBody            string
	MessageAttributes      map[string]string // attribute name -> string value
	MessageGroupID         string
	MessageDeduplicationID string
}

// SQSResponse tells SQSServer how to answer one request.
type SQSResponse struct {
	// Status is the HTTP status, 200 when zero.
	Status int

	// Delay holds the response for this long before answering.
	Delay time.Duration
}

// SQSServer is a fake SQS endpoint (JSON protocol) for tests. Point runtime.Config.SqsEndpoint or the
// sqs client Endpoint at its URL. Failing responses should use a 4xx status because the AWS SDK retries 5xx
// responses with backoff unless AWS_MAX_ATTEMPTS=1 is set.
type SQSServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []SQSRequest
	handler  func(n int, req SQSRequest) SQSResponse
}

// NewSQSServer starts a fake SQS endpoint. The handler receives the 1-based request number and the captured
// request; a nil handler answers 200 to everything.
func NewSQSServer(handler func(n int, req SQSRequest) SQSResponse) *SQSServer {
	s := &SQSServer{handler: handler}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Requests returns a copy of the requests received so far.
func (s *SQSServer) Requests() []SQSRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SQSRequest(nil), s.requests...)
}

func (s *SQSServer) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)

	var in struct {
		QueueURL          string `json:"QueueUrl"`
		MessageBody       string `json:"MessageBody"`
		MessageGroupID    string `json:"MessageGroupId"`
		MessageDedupID    string `json:"MessageDeduplicationId"`
		MessageAttributes map[string]struct {
			StringValue string `json:"StringValue"`
		} `json:"MessageAttributes"`
	}
	_ = json.Unmarshal(raw, &in)

	req := SQSRequest{
		QueueURL:               in.QueueURL,
		MessageBody:            in.MessageBody,
		MessageGroupID:         in.MessageGroupID,
		MessageDeduplicationID: in.MessageDedupID,
		MessageAttributes:      map[string]string{},
	}
	for name, attr := range in.MessageAttributes {
		req.MessageAttributes[name] = attr.StringValue
	}

	s.mu.Lock()
	s.requests = append(s.requests, req)
	n := len(s.requests)
	s.mu.Unlock()

	resp := SQSResponse{}
	if s.handler != nil {
		resp = s.handler(n, req)
	}
	if resp.Delay > 0 {
		time.Sleep(resp.Delay)
	}

	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	if resp.Status != 0 && resp.Status != http.StatusOK {
		w.WriteHeader(resp.Status)
		_, _ = w.Write([]byte(`{"__type":"InvalidParameterValue","message":"test failure"}`))
		return
	}
	_, _ = w.Write([]byte(`{"MessageId":"00000000-0000-0000-0000-000000000000"}`))
}

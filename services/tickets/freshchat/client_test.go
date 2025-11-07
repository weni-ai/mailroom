package freshchat_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/tickets/freshchat"
	"github.com/stretchr/testify/assert"
)

const (
	baseURL = "https://api.freshchat.com"
	apiKey  = "1234567890"
)

func TestCreateConversation(t *testing.T) {

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/conversations", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"conversation_id": "1234567890",
				"status": "new",
				"channel_id": "1234567890"
			}`),
		},
	}))

	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	conversation := &freshchat.Conversation{
		Status:    "new",
		ChannelID: "1234567890",
		Messages: []freshchat.Message{
			{
				ID: "1234567890",
			},
		},
		Properties: freshchat.Properties{
			Value: []map[string]interface{}{
				{
					"key": "value",
				},
			},
		},
		Users: []freshchat.User{
			{
				ID: "1234567890",
			},
		},
		UserID: "1234567890",
	}
	_, _, err := client.CreateConversation(conversation)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateConversation(conversation)
	assert.EqualError(t, err, "Something went wrong")

	conversation, trace, err := client.CreateConversation(conversation)
	assert.NoError(t, err)
	assert.Equal(t, "1234567890", conversation.ConversationID)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 95\r\n\r\n", string(trace.ResponseTrace))
}

func TestCreateUser(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "1234567890",
				"email": "test@test.com",
				"first_name": "Test",
				"last_name": "Test",
				"phone": "1234567890",
				"created_time": "2022-03-08T22:38:30Z",
				"updated_time": "2022-03-08T22:38:30Z",
				"reference_id": "1234567890",
				"properties": [
					{
						"name": "key",
						"value": "value"
					}
				]
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	user := &freshchat.User{
		ID:          "1234567890",
		Email:       "test@test.com",
		FirstName:   "Test",
		LastName:    "Test",
		Phone:       "1234567890",
		CreatedTime: "2022-03-08T22:38:30Z",
		UpdatedTime: "2022-03-08T22:38:30Z",
		ReferenceID: "1234567890",
		Properties: []freshchat.UserProperty{
			{
				Name:  "key",
				Value: "value",
			},
		},
	}
	_, _, err := client.CreateUser(user)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateUser(user)
	assert.EqualError(t, err, "Something went wrong")

	user, trace, err := client.CreateUser(user)
	assert.NoError(t, err)
	assert.Equal(t, "1234567890", user.ID)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 344\r\n\r\n", string(trace.ResponseTrace))
}

func TestGetUser(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users?reference_id=5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "1234567890",
						"email": "test@test.com",
						"first_name": "Test",
						"last_name": "Test",
						"phone": "1234567890",
						"created_time": "2022-03-08T22:38:30Z",
						"updated_time": "2022-03-08T22:38:30Z",
						"reference_id": "5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f"
					}
				]
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	_, _, err := client.GetUser("5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f")
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.GetUser("5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f")
	assert.EqualError(t, err, "Something went wrong")

	user, trace, err := client.GetUser("5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f")
	assert.NoError(t, err)
	assert.Equal(t, "5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f", user.ReferenceID)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 336\r\n\r\n", string(trace.ResponseTrace))
}

func TestGetChannels(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/channels", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{
						"id": "1234567890",
						"name": "Channel 1"
					}
				]
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	_, _, err := client.GetChannels()
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.GetChannels()
	assert.EqualError(t, err, "Something went wrong")

	channels, trace, err := client.GetChannels()
	assert.NoError(t, err)
	assert.Equal(t, "1234567890", channels[0].ID)
	assert.Equal(t, "Channel 1", channels[0].Name)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 96\r\n\r\n", string(trace.ResponseTrace))
}

func TestCreateMessage(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/conversations/1234567890/messages", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"channel_id": "1234567890123",
				"conversation_id": "1234567890",
				"message_parts": [
					{
						"text": {
							"content": "Hello, world!"
						}
					}
				],
				"actor_type": "user",
				"actor_id": "123456789023",
				"id": "123id"
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	message := &freshchat.Message{
		ConversationID: "1234567890",
		MessageParts: []freshchat.MessageParts{
			{
				Text: &freshchat.Text{
					Content: "Hello, world!",
				},
			},
		},
	}
	_, _, err := client.CreateMessage(message)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateMessage(message)
	assert.EqualError(t, err, "Something went wrong")

	message, trace, err := client.CreateMessage(message)
	assert.NoError(t, err)
	assert.Equal(t, "123id", message.ID)
	assert.Equal(t, "Hello, world!", message.MessageParts[0].Text.Content)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 256\r\n\r\n", string(trace.ResponseTrace))
}

func TestUpdateConversation(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/conversations/1234567890", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"conversation_id": "1234567890",
				"status": "reopen",
				"channel_id": "1234567890123"
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	conversation := &freshchat.Conversation{
		ConversationID: "1234567890",
		Status:         "reopen",
	}
	_, err := client.UpdateConversation(conversation)
	assert.EqualError(t, err, "unable to connect to server")

	_, err = client.UpdateConversation(conversation)
	assert.EqualError(t, err, "Something went wrong")

	trace, err := client.UpdateConversation(conversation)
	assert.NoError(t, err)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 101\r\n\r\n", string(trace.ResponseTrace))
}

func TestUploadImage(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://foo.bar.com/images/image1.png": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `fake image data`),
			httpx.NewMockResponse(200, nil, `fake image data`),
		},
		fmt.Sprintf("%s/v2/images/upload", baseURL): {
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"url": "https://foo.bar.com/images/image2.png"
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	_, err := client.UploadImage("https://foo.bar.com/images/image1.png")
	assert.EqualError(t, err, "unable to connect to server")

	_, err = client.UploadImage("https://foo.bar.com/images/image1.png")
	assert.EqualError(t, err, "Something went wrong")

	imageURL, err := client.UploadImage("https://foo.bar.com/images/image1.png")
	assert.NoError(t, err)
	assert.Equal(t, "https://foo.bar.com/images/image2.png", imageURL)
}

func TestUploadFile(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://foo.bar.com/files/file1.pdf": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `fake image data`),
			httpx.NewMockResponse(200, nil, `fake image data`),
		},
		fmt.Sprintf("%s/v2/files/upload", baseURL): {
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.freshchat.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"file_name": "file1.pdf",
				"file_size": 100,
				"file_content_type": "application/pdf",
				"file_extension_type": "pdf",
				"file_hash": "1234567890",
				"file_security_status": "safe"
			}`),
		},
	}))
	client := freshchat.NewClient(http.DefaultClient, nil, baseURL, apiKey)
	_, err := client.UploadFile("https://foo.bar.com/files/file1.pdf")
	assert.EqualError(t, err, "unable to connect to server")

	_, err = client.UploadFile("https://foo.bar.com/files/file1.pdf")
	assert.EqualError(t, err, "Something went wrong")

	file, err := client.UploadFile("https://foo.bar.com/files/file1.pdf")
	assert.NoError(t, err)
	assert.Equal(t, "file1.pdf", file.Name)
	assert.Equal(t, 100, file.FileSizeInBytes)
	assert.Equal(t, "application/pdf", file.ContentType)
	assert.Equal(t, "pdf", file.FileExtension)
	assert.Equal(t, "1234567890", file.FileHash)
}

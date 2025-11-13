package twilioflex2_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/tickets/twilioflex2"
	"github.com/stretchr/testify/assert"
)

const (
	authToken   = "test-auth-token"
	accountSid  = "AC81d44315e19372138bdaffcc13cf3b94"
	instanceSid = "IS38067ec392f1486bb6e4de4610f26fb3"
)

func TestCreateInteractionScopedWebhook(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://flex-api.twilio.com/v1/Instances/%s/InteractionWebhooks", instanceSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"ttid": "WH12345678901234567890123456789012",
				"account_sid": "AC81d44315e19372138bdaffcc13cf3b94",
				"instance_sid": "IS38067ec392f1486bb6e4de4610f26fb3",
				"type": "interaction",
				"webhook_url": "https://your-app.com/webhook",
				"webhook_method": "POST",
				"webhook_events": ["onChannelStatusUpdated"],
				"url": "https://your-app.com/webhook"
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.CreateInteractionWebhookRequest{
		Type:          "interaction",
		WebhookUrl:    "https://your-app.com/webhook",
		WebhookMethod: "POST",
		WebhookEvents: []string{"onChannelStatusUpdated"},
	}

	_, _, err := client.CreateInteractionScopedWebhook(instanceSid, params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateInteractionScopedWebhook(instanceSid, params)
	assert.EqualError(t, err, "Something went wrong")

	webhook, trace, err := client.CreateInteractionScopedWebhook(instanceSid, params)
	assert.NoError(t, err)
	assert.Equal(t, "WH12345678901234567890123456789012", webhook.Ttid)
	assert.Equal(t, []string{"onChannelStatusUpdated"}, webhook.WebhookEvents)
	assert.Equal(t, "interaction", webhook.Type)
	assert.Equal(t, "https://your-app.com/webhook", webhook.WebhookUrl)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 371\r\n\r\n", string(trace.ResponseTrace))
}

func TestCreateInteraction(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://flex-api.twilio.com/v1/Interactions": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"channel": {
					"sid": "UOxx123456789012345678901234567890",
					"type": "web"
				},
				"interaction_context_sid": "IC12345678901234567890123456789012",
				"links": {
					"channels": "https://flex-api.twilio.com/v1/Interactions/KDfbcc36df16d504c234b2c46db61a3464/Channels"
				},
				"routing": {
					"properties": {
						"account_sid": "ACxx123456789012345678901234567890",
						"addons": "{}",
						"age": 0,
						"age_in_queue": 0,
						"assignmentCounter": 0,
						"assignment_status": "pending",
						"attributes": "{\"flexChannelInviteSid\":\"KG991456db3f867c9506ba9597e88afac7\",\"customerId\":\"100_4996b4a0-6b9e-4b9d-8316-9a4a2095f445\",\"channelType\":\"web\",\"conversationSid\":\"CH54dcbcf4e6b24cf28ff55ecd4556015f\",\"customers\":{},\"taskCreateSource\":\"interaction\",\"conversations\":{\"initiated_by\":\"API\",\"conversation_id\":\"KDfbcc36df16d504c234b2c46db61a3464\",\"media\":[{\"type\":\"ChatTranscript\",\"sid\":\"CH54dcbcf4e6b24cf28ff55ecd4556015f\"}]},\"initiatedBy\":\"api\",\"flexInteractionChannelSid\":\"UOad4bf4740fdd394f145e049e183460c5\",\"flexInteractionSid\":\"KDfbcc36df16d504c234b2c46db61a3464\",\"custom_attributes\":\"\",\"direction\":\"inbound\"}",
						"custom_context": null,
						"date_created": 1761934290,
						"date_updated": 1761934290,
						"ignore_capacity": false,
						"priority": 0,
						"queue_name": "Everyone",
						"queue_sid": "WQab27293ea5d899e465b513b6cca00717",
						"reason": "",
						"required_attention": null,
						"routing_target": null,
						"sid": "WTb7a01b7bc8a8a38c1ac16f6bd412ee07",
						"task_channel_sid": "TC2fe62df21997126abe4abb680751b738",
						"task_channel_unique_name": "chat",
						"task_queue_entered_date": 1761934290,
						"timeout": 86400,
						"twilio_context": null,
						"version": 0,
						"virtual_start_time": 1761934290,
						"workflow_name": "Assign to Anyone",
						"workflow_sid": "WWe73e5e5c644cf2b887a1ad3d39c755a6",
						"workspace_sid": "WS2df090d8808b181baf01b5640ecf014c"
					},
					"reservation": null
				},
				"sid": "KDfbcc36df16d504c234b2c46db61a3464",
				"url": "https://flex-api.twilio.com/v1/Interactions/KDfbcc36df16d504c234b2c46db61a3464",
				"webhook_ttid": "flex_interactionwebhook_01k8xqfbaxfh5tpnqm2psh6238"
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.CreateInteractionRequest{
		Channel: twilioflex2.InteractionChannelParam{
			Type:        "whatsapp",
			InitiatedBy: "customer",
			Properties:  map[string]any{"customer_id": "12345"},
		},
		Routing: twilioflex2.InteractionRoutingParam{
			Type: "studio",
			Properties: twilioflex2.InteractionRoutingProperties{
				WorkspaceSid:          "WS12345678901234567890123456789012",
				WorkflowSid:           "WW12345678901234567890123456789012",
				TaskChannelUniqueName: "voice",
				Attributes:            map[string]any{"priority": "high"},
			},
		},
		WebhookTtid: "WH12345678901234567890123456789012",
	}

	_, _, err := client.CreateInteraction(params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateInteraction(params)
	assert.EqualError(t, err, "Something went wrong")

	interaction, trace, err := client.CreateInteraction(params)
	assert.NoError(t, err)
	assert.Equal(t, "KDfbcc36df16d504c234b2c46db61a3464", interaction.Sid)
	assert.Equal(t, "flex_interactionwebhook_01k8xqfbaxfh5tpnqm2psh6238", interaction.WebhookTtid)
	assert.Equal(t, "IC12345678901234567890123456789012", interaction.InteractionContextSid)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 2270\r\n\r\n", string(trace.ResponseTrace))
}

func TestCreateConversationScopedWebhook(t *testing.T) {
	conversationSid := "CH12345678901234567890123456789012"
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://conversations.twilio.com/v1/Conversations/%s/Webhooks", conversationSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"sid": "WH23456789012345678901234567890123",
				"account_sid": "AC81d44315e19372138bdaffcc13cf3b94",
				"conversation_sid": "CH12345678901234567890123456789012",
				"target": "webhook",
				"configuration": {
					"url": "https://your-app.com/conversation-webhook",
					"method": "POST",
					"filters": ["onMessageAdded"]
				},
				"date_created": "2024-01-01T00:00:00Z",
				"date_updated": "2024-01-01T00:00:00Z",
				"url": "https://conversations.twilio.com/v1/Conversations/CH12345678901234567890123456789012/Webhooks/WH23456789012345678901234567890123"
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.CreateConversationWebhookRequest{
		Target:               "webhook",
		ConfigurationUrl:     "https://your-app.com/conversation-webhook",
		ConfigurationMethod:  "POST",
		ConfigurationFilters: []string{"onMessageAdded"},
	}

	_, _, err := client.CreateConversationScopedWebhook(conversationSid, params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateConversationScopedWebhook(conversationSid, params)
	assert.EqualError(t, err, "Something went wrong")

	webhook, trace, err := client.CreateConversationScopedWebhook(conversationSid, params)
	assert.NoError(t, err)
	assert.Equal(t, "WH23456789012345678901234567890123", webhook.Sid)
	assert.Equal(t, conversationSid, webhook.ConversationSid)
	assert.Equal(t, "webhook", webhook.Target)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 574\r\n\r\n", string(trace.ResponseTrace))
}

func TestSendCustomerMessage(t *testing.T) {
	conversationSid := "CH12345678901234567890123456789012"
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://conversations.twilio.com/v1/Conversations/%s/Messages", conversationSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"sid": "IM34567890123456789012345678901234",
				"account_sid": "AC81d44315e19372138bdaffcc13cf3b94",
				"conversation_sid": "CH12345678901234567890123456789012",
				"body": "Hello, I need help with my order",
				"author": "customer",
				"media": null,
				"participant_sid": "MB45678901234567890123456789012345",
				"index": 1
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.CreateConversationMessageRequest{
		Author: "customer",
		Body:   "Hello, I need help with my order",
	}

	_, _, err := client.SendCustomerMessage(conversationSid, params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.SendCustomerMessage(conversationSid, params)
	assert.EqualError(t, err, "Something went wrong")

	message, trace, err := client.SendCustomerMessage(conversationSid, params)
	assert.NoError(t, err)
	assert.Equal(t, "IM34567890123456789012345678901234", message.Sid)
	assert.Equal(t, conversationSid, message.ConversationSid)
	assert.Equal(t, "customer", message.Author)
	assert.Equal(t, "Hello, I need help with my order", message.Body)
	assert.Equal(t, 1, message.Index)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 343\r\n\r\n", string(trace.ResponseTrace))
}

func TestUpdateInteractionChannel(t *testing.T) {
	interactionSid := "KD12345678901234567890123456789012"
	channelSid := "UO45678901234567890123456789012345"
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://flex-api.twilio.com/v1/Interactions/%s/Channels/%s", interactionSid, channelSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(200, nil, `{
				"sid": "UO45678901234567890123456789012345",
				"account_sid": "AC81d44315e19372138bdaffcc13cf3b94",
				"interaction_sid": "KD12345678901234567890123456789012",
				"type": "voice",
				"status": "wrapup",
				"participants": [
					{
						"sid": "UT56789012345678901234567890123456",
						"identity": "agent",
						"type": "agent"
					},
					{
						"sid": "UT67890123456789012345678901234567",
						"identity": "customer",
						"type": "customer"
					}
				],
				"date_created": "2024-01-01T00:00:00Z",
				"date_updated": "2024-01-01T00:00:00Z",
				"url": "https://flex-api.twilio.com/v1/Interactions/KD12345678901234567890123456789012/Channels/UO45678901234567890123456789012345"
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.UpdateInteractionChannelRequest{
		Status:  "wrapup",
		Routing: "most-recent",
	}

	_, _, err := client.UpdateInteractionChannel(interactionSid, channelSid, params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.UpdateInteractionChannel(interactionSid, channelSid, params)
	assert.EqualError(t, err, "Something went wrong")

	channel, trace, err := client.UpdateInteractionChannel(interactionSid, channelSid, params)
	assert.NoError(t, err)
	assert.Equal(t, channelSid, channel.Sid)
	assert.Equal(t, interactionSid, channel.InteractionSid)
	assert.Equal(t, "wrapup", channel.Status)
	assert.Equal(t, "voice", channel.Type)
	assert.Len(t, channel.Participants, 2)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 706\r\n\r\n", string(trace.ResponseTrace))
}

func TestCreateMedia(t *testing.T) {
	serviceSid := "IS12345678901234567890123456789012"
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media", serviceSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"message": "Something went wrong", "detail": "Unknown", "code": 1234, "more_info": "https://www.twilio.com/docs/errors/1234"}`),
			httpx.NewMockResponse(201, nil, `{
				"sid": "ME34567890123456789012345678901234",
				"service_sid": "IS12345678901234567890123456789012",
				"date_created": "2024-01-01T00:00:00Z",
				"date_upload_updated": "2024-01-01T00:00:00Z",
				"date_updated": "2024-01-01T00:00:00Z",
				"links": {
					"content": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234/Content",
					"content_direct_temporary": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234/Content?Temp=true"
				},
				"size": 1024,
				"content_type": "image/jpeg",
				"filename": "test.jpg",
				"author": "customer",
				"category": "media",
				"message_sid": null,
				"channel_sid": null,
				"url": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234",
				"is_multipart_upstream": false
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)
	params := &twilioflex2.CreateMediaParams{
		FileName:    "test.jpg",
		Media:       []byte("fake image content"),
		Author:      "customer",
		ContentType: "image/jpeg",
	}

	_, _, err := client.CreateMedia(serviceSid, params)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.CreateMedia(serviceSid, params)
	assert.EqualError(t, err, "Something went wrong")

	media, trace, err := client.CreateMedia(serviceSid, params)
	assert.NoError(t, err)
	assert.Equal(t, "ME34567890123456789012345678901234", media.Sid)
	assert.Equal(t, serviceSid, media.ServiceSid)
	assert.Equal(t, "image/jpeg", media.ContentType)
	assert.Equal(t, "test.jpg", media.Filename)
	assert.Equal(t, "customer", media.Author)
	assert.Equal(t, 1024, media.Size)
	assert.Equal(t, "HTTP/1.0 201 Created\r\nContent-Length: 928\r\n\r\n", string(trace.ResponseTrace))
}

func TestFetchMedia(t *testing.T) {
	serviceSid := "IS12345678901234567890123456789012"
	mediaSid := "ME34567890123456789012345678901234"
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("https://mcs.us1.twilio.com/v1/Services/%s/Media/%s", serviceSid, mediaSid): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(404, nil, `{"message": "Media resource not found", "detail": "Unknown", "code": 20404, "more_info": "https://www.twilio.com/docs/errors/20404"}`),
			httpx.NewMockResponse(200, nil, `{
				"sid": "ME34567890123456789012345678901234",
				"service_sid": "IS12345678901234567890123456789012",
				"date_created": "2024-01-01T00:00:00Z",
				"date_upload_updated": "2024-01-01T00:00:00Z",
				"date_updated": "2024-01-01T00:00:00Z",
				"links": {
					"content": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234/Content",
					"content_direct_temporary": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234/Content?Temp=true"
				},
				"size": 1024,
				"content_type": "image/jpeg",
				"filename": "test.jpg",
				"author": "customer",
				"category": "media",
				"message_sid": null,
				"channel_sid": null,
				"url": "https://mcs.us1.twilio.com/v1/Services/IS12345678901234567890123456789012/Media/ME34567890123456789012345678901234",
				"is_multipart_upstream": false
			}`),
		},
	}))

	client := twilioflex2.NewClient(http.DefaultClient, nil, authToken, accountSid)

	_, _, err := client.FetchMedia(serviceSid, mediaSid)
	assert.EqualError(t, err, "unable to connect to server")

	_, _, err = client.FetchMedia(serviceSid, mediaSid)
	assert.EqualError(t, err, "Media resource not found")

	media, trace, err := client.FetchMedia(serviceSid, mediaSid)
	assert.NoError(t, err)
	assert.Equal(t, mediaSid, media.Sid)
	assert.Equal(t, serviceSid, media.ServiceSid)
	assert.Equal(t, "image/jpeg", media.ContentType)
	assert.Equal(t, "test.jpg", media.Filename)
	assert.Equal(t, "customer", media.Author)
	assert.Equal(t, 1024, media.Size)
	assert.Equal(t, "HTTP/1.0 200 OK\r\nContent-Length: 928\r\n\r\n", string(trace.ResponseTrace))
}

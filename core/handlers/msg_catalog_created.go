package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/core/hooks"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/external/openai/chatgpt"
	"github.com/nyaruka/mailroom/services/external/weni/wenigpt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	models.RegisterEventPreWriteHandler(events.TypeMsgCatalogCreated, handlePreMsgCatalogCreated)
	models.RegisterEventHandler(events.TypeMsgCatalogCreated, handleMsgCatalogCreated)
}

// handlePreMsgCatalogCreated clears our timeout on our session so that courier can send it when the message is sent, that will be set by courier when sent
func handlePreMsgCatalogCreated(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.MsgCatalogCreatedEvent)

	// we only clear timeouts on messaging flows
	if scene.Session().SessionType() != models.FlowTypeMessaging {
		return nil
	}

	// get our channel
	var channel *models.Channel

	if event.Msg.Channel() != nil {
		channel = oa.ChannelByUUID(event.Msg.Channel().UUID)
		if channel == nil {
			return errors.Errorf("unable to load channel with uuid: %s", event.Msg.Channel().UUID)
		}
	}

	// no channel? this is a no-op
	if channel == nil {
		return nil
	}

	// android channels get normal timeouts
	if channel.Type() == models.ChannelTypeAndroid {
		return nil
	}

	// everybody else gets their timeout cleared, will be set by courier
	scene.Session().ClearTimeoutOn()

	return nil
}

// handleMsgCreated creates the db msg for the passed in event
func handleMsgCatalogCreated(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scene *models.Scene, e flows.Event) error {
	event := e.(*events.MsgCatalogCreatedEvent)

	// must be in a session
	if scene.Session() == nil {
		return errors.Errorf("cannot handle msg created event without session")
	}

	logrus.WithFields(logrus.Fields{
		"contact_uuid": scene.ContactUUID(),
		"session_id":   scene.SessionID(),
		"text":         event.Msg.Text(),
		"header":       event.Msg.Header(),
		"products":     event.Msg.Products(),
		"urn":          event.Msg.URN(),
		"action":       event.Msg.Action(),
	}).Debug("msg created event")

	// messages in messaging flows must have urn id set on them, if not, go look it up
	if scene.Session().SessionType() == models.FlowTypeMessaging && event.Msg.URN() != urns.NilURN {
		urn := event.Msg.URN()
		if models.GetURNInt(urn, "id") == 0 {
			urn, err := models.GetOrCreateURN(ctx, tx, oa, scene.ContactID(), event.Msg.URN())
			if err != nil {
				return errors.Wrapf(err, "unable to get or create URN: %s", event.Msg.URN())
			}
			// update our Msg with our full URN
			event.Msg.SetURN(urn)
		}
	}

	// get our channel
	var channel *models.Channel
	if event.Msg.Channel() != nil {
		channel = oa.ChannelByUUID(event.Msg.Channel().UUID)
		if channel == nil {
			return errors.Errorf("unable to load channel with uuid: %s", event.Msg.Channel().UUID)
		} else {
			if fmt.Sprint(channel.Type()) == "WAC" || fmt.Sprint(channel.Type()) == "WA" {
				country := envs.DeriveCountryFromTel("+" + event.Msg.URN().Path())
				locale := envs.NewLocale(scene.Contact().Language(), country)
				languageCode := locale.ToBCP47()

				if _, valid := validLanguageCodes[languageCode]; !valid {
					languageCode = ""
				}

				event.Msg.TextLanguage = envs.Language(languageCode)
			}
		}
	} else {
		return errors.New("channel is not defined")
	}

	if len(event.Msg.Products()) == 0 && event.Msg.Smart() {
		content := event.Msg.ProductSearch()
		productList, err := GetProductListFromWeniGPT(ctx, rt, content)
		if err != nil {
			return err
		}
		fmt.Println(productList)
		// TODO: implement SentenX call to retrieve product list
		catalog, err := models.GetActiveCatalogFromChannel(ctx, *rt.DB, channel.ID())
		if err != nil {
			return err
		}
		threshold := channel.ConfigValue("threshold", "1.5")

		productRetailerIDS := []string{}

		for _, product := range productList {
			searchResult, err := GetProductListFromSentenX(product, catalog.FacebookCatalogID(), threshold, rt)
			if err != nil {
				return errors.Wrapf(err, "on iterate to search products on sentenx")
			}
			for _, prod := range searchResult {
				productRetailerIDS = append(productRetailerIDS, prod["product_retailer_id"])
			}
		}

		event.Msg.Products_ = productRetailerIDS
	}

	msg, err := models.NewOutgoingFlowMsgCatalog(rt, oa.Org(), channel, scene.Session(), event.Msg, event.CreatedOn())
	if err != nil {
		return errors.Wrapf(err, "error creating outgoing message to %s", event.Msg.URN())
	}

	// register to have this message committed
	scene.AppendToEventPreCommitHook(hooks.CommitMessagesHook, msg)

	// don't send messages for surveyor flows
	if scene.Session().SessionType() != models.FlowTypeSurveyor {
		scene.AppendToEventPostCommitHook(hooks.SendMessagesHook, msg)
	}

	return nil
}

func GetProductListFromWeniGPT(ctx context.Context, rt *runtime.Runtime, content string) ([]string, error) {
	httpClient, httpRetries, _ := goflow.HTTP(rt.Config)
	weniGPTClient := wenigpt.NewClient(httpClient, httpRetries, rt.Config.WeniGPTBaseURL, rt.Config.WeniGPTAuthToken, rt.Config.WeniGPTCookie)

	prompt := fmt.Sprintf(`Give me an unformatted JSON list containing strings with the name of each product taken from the user prompt. Never repeat the same product. Always use this pattern: {\"products\": []}. Request: %s. Response:`, content)

	dr := wenigpt.NewWenigptRequest(
		prompt,
		0,
		0.0,
		0.0,
		true,
		wenigpt.DefaultStopSequences,
	)

	response, _, err := weniGPTClient.WeniGPTRequest(dr)
	if err != nil {
		return nil, errors.Wrapf(err, "error on wewnigpt call fot list products")
	}

	productsJson := response.Output.Text

	var products map[string][]string
	err = json.Unmarshal([]byte(productsJson), &products)
	if err != nil {
		return nil, errors.Wrapf(err, "error on unmarshalling product list")
	}
	return products["products"], nil
}

func GetProductListFromChatGPT(ctx context.Context, rt *runtime.Runtime, content string) ([]string, error) {
	httpClient, httpRetries, _ := goflow.HTTP(rt.Config)
	chatGPTClient := chatgpt.NewClient(httpClient, httpRetries, rt.Config.ChatGPTBaseURL, rt.Config.ChatGPTKey)

	prompt1 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "Give me an unformatted JSON list containing strings with the name of each product taken from the user prompt.",
	}
	prompt2 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "Never repeat the same product.",
	}
	prompt3 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "Always use this pattern: {\"products\": []}",
	}
	question := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleUser,
		Content: content,
	}
	completionRequest := chatgpt.NewChatCompletionRequest([]chatgpt.ChatCompletionMessage{prompt1, prompt2, prompt3, question})
	response, _, err := chatGPTClient.CreateChatCompletion(completionRequest)
	if err != nil {
		return nil, errors.Wrapf(err, "error on chatgpt call for list products")
	}

	productsJson := response.Choices[0].Message.Content

	var products map[string][]string
	err = json.Unmarshal([]byte(productsJson), &products)
	if err != nil {
		return nil, errors.Wrapf(err, "error on unmarshalling product list")
	}
	return products["products"], nil
}

func GetProductListFromSentenX(productSearch string, catalogID string, threshold string, rt *runtime.Runtime) ([]map[string]string, error) {
	// TODO: implement this GetProductListFromSentenX
	return nil, nil

}

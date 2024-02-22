package catalogs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/external/openai/chatgpt"
	"github.com/nyaruka/mailroom/services/external/weni/sentenx"
	"github.com/nyaruka/mailroom/services/external/weni/wenigpt"
	"github.com/pkg/errors"
)

const (
	serviceType = "msg_catalog"
)

var db *sqlx.DB
var mu = &sync.Mutex{}

func initDB(dbURL string) error {
	mu.Lock()
	defer mu.Unlock()
	if db == nil {
		newDB, err := sqlx.Open("postgres", dbURL)
		if err != nil {
			return errors.Wrap(err, "unable to open database connection")
		}
		SetDB(newDB)
	}
	return nil
}

func SetDB(newDB *sqlx.DB) {
	db = newDB
}

func init() {
	models.RegisterMsgCatalogService(serviceType, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *http.Client
	redactor   utils.Redactor
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, msgCatalog *flows.MsgCatalog, config map[string]string) (models.MsgCatalogService, error) {

	if err := initDB(rtCfg.DB); err != nil {
		return nil, err
	}

	return &service{
		rtConfig:   rtCfg,
		restClient: httpClient,
		redactor:   utils.NewRedactor(flows.RedactionMask),
	}, nil
}

func (s *service) Call(session flows.Session, params assets.MsgCatalogParam, logHTTP flows.HTTPLogCallback) (*flows.MsgCatalogCall, error) {
	callResult := &flows.MsgCatalogCall{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	content := params.ProductSearch
	productList, traceWeniGPT, err := GetProductListFromChatGPT(ctx, s.rtConfig, content)
	callResult.Traces = append(callResult.Traces, traceWeniGPT)
	if err != nil {
		return callResult, err
	}
	channelUUID := params.ChannelUUID
	channel, err := models.GetActiveChannelByUUID(ctx, db, channelUUID)
	if err != nil {
		return callResult, err
	}

	catalog, err := models.GetActiveCatalogFromChannel(ctx, *db, channel.ID())
	if err != nil {
		return callResult, err
	}
	channelThreshold := channel.ConfigValue("threshold", "1.5")
	searchThreshold, err := strconv.ParseFloat(channelThreshold, 64)
	if err != nil {
		return callResult, err
	}

	productRetailerIDS := []string{}
	productRetailerIDMap := make(map[string]struct{})
	var productEntries []flows.ProductEntry
	var productEntry flows.ProductEntry
	searchResult := []string{}
	var trace *httpx.Trace
	var traces []*httpx.Trace

	for _, product := range productList {
		if params.SearchType == "default" {
			searchResult, trace, err = GetProductListFromSentenX(product, catalog.FacebookCatalogID(), searchThreshold, s.rtConfig)
			callResult.Traces = append(callResult.Traces, trace)
		} else if params.SearchType == "vtex" {
			searchResult, traces, err = GetProductListFromVtex(product, params.SearchUrl, params.ApiType, params.SellerId)
			callResult.Traces = append(callResult.Traces, traces...)
			if searchResult == nil {
				continue
			}
		}
		if err != nil {
			return callResult, errors.Wrapf(err, "on iterate to search products")
		}
		for _, prod := range searchResult {
			productRetailerID := prod
			_, exists := productRetailerIDMap[productRetailerID]
			if !exists {
				productRetailerIDS = append(productRetailerIDS, productRetailerID)
				productRetailerIDMap[productRetailerID] = struct{}{}
			}
		}

		if len(productRetailerIDS) > 0 {
			productEntry = flows.ProductEntry{
				Product:            product,
				ProductRetailerIDs: productRetailerIDS,
			}
			productEntries = append(productEntries, productEntry)
			productRetailerIDS = nil
		}
		productRetailerIDMap = make(map[string]struct{})
	}

	url := params.SearchUrl

	client := &http.Client{}
	req, err := httpx.NewRequest("POST", url, nil, nil)
	if err != nil {
		return nil, err
	}

	trace, err = httpx.DoTrace(client, req, nil, nil, -1)
	if err != nil {
		return nil, err
	}

	response := []struct {
		Items []struct {
			ItemId string `json:"itemId"`
		} `json:"items"`
	}{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, err
	}

	callResult.ProductRetailerIDS = productEntries

	return callResult, nil
}

func GetProductListFromWeniGPT(rtConfig *runtime.Config, content string) ([]string, *httpx.Trace, error) {
	httpClient, httpRetries, _ := goflow.HTTP(rtConfig)
	weniGPTClient := wenigpt.NewClient(httpClient, httpRetries, rtConfig.WenigptBaseURL, rtConfig.WenigptAuthToken, rtConfig.WenigptCookie)

	prompt := fmt.Sprintf(`Give me an unformatted JSON list containing strings with the name of each product taken from the user prompt. Never repeat the same product. Always return a valid json using this pattern: {\"products\": []} Request: %s. Response:`, content)

	dr := wenigpt.NewWenigptRequest(
		prompt,
		0,
		0.0,
		0.0,
		true,
		wenigpt.DefaultStopSequences,
	)

	response, trace, err := weniGPTClient.WeniGPTRequest(dr)
	if err != nil {
		return nil, trace, errors.Wrapf(err, "error on wenigpt call fot list products")
	}

	productsJson := response.Output.Text[0]

	var products map[string][]string
	err = json.Unmarshal([]byte(productsJson), &products)
	if err != nil {
		return nil, trace, errors.Wrapf(err, "error on unmarshalling product list")
	}
	return products["products"], trace, nil
}

func GetProductListFromSentenX(productSearch string, catalogID string, threshold float64, rtConfig *runtime.Config) ([]string, *httpx.Trace, error) {
	client := sentenx.NewClient(http.DefaultClient, nil, rtConfig.SentenxBaseURL)

	searchParams := sentenx.NewSearchRequest(productSearch, catalogID, threshold)

	searchResponse, trace, err := client.SearchProducts(searchParams)
	if err != nil {
		return nil, trace, err
	}

	if len(searchResponse.Products) < 1 {
		return nil, trace, errors.New("no products found on sentenx")
	}

	pmap := []string{}
	for _, p := range searchResponse.Products {
		pmap = append(pmap, p.ProductRetailerID)
	}

	return pmap, trace, nil
}

func GetProductListFromChatGPT(ctx context.Context, rtConfig *runtime.Config, content string) ([]string, *httpx.Trace, error) {
	httpClient, httpRetries, _ := goflow.HTTP(rtConfig)
	chatGPTClient := chatgpt.NewClient(httpClient, httpRetries, rtConfig.ChatgptBaseURL, rtConfig.ChatgptKey)

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
	prompt4 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "Ensure that no product names are repeated, and each product should be in singular form without any numbers or quantities.",
	}
	prompt5 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "Preserve the order of products as they appear in the user prompt.",
	}
	question := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleUser,
		Content: content,
	}
	completionRequest := chatgpt.NewChatCompletionRequest([]chatgpt.ChatCompletionMessage{prompt1, prompt2, prompt3, prompt4, prompt5, question})
	response, trace, err := chatGPTClient.CreateChatCompletion(completionRequest)
	if err != nil {
		return nil, trace, errors.Wrapf(err, "error on chatgpt call for list products")
	}

	productsJson := response.Choices[0].Message.Content

	var products map[string][]string
	err = json.Unmarshal([]byte(productsJson), &products)
	if err != nil {
		return nil, trace, errors.Wrapf(err, "error on unmarshalling product list")
	}
	return products["products"], trace, nil
}

func GetProductListFromVtex(productSearch string, searchUrl string, apiType string, sellerID string) ([]string, []*httpx.Trace, error) {
	var result []string
	var traces []*httpx.Trace
	var err error

	sellerID = strings.TrimSpace(sellerID)

	if apiType == "legacy" {
		result, traces, err = VtexLegacySearch(searchUrl, productSearch, sellerID)
		if err != nil {
			return nil, traces, err
		}
	} else if apiType == "intelligent" {
		result, traces, err = VtexIntelligentSearch(searchUrl, productSearch, sellerID)
		if err != nil {
			return nil, traces, err
		}
	}

	return result, traces, nil
}

type SearchSeller struct {
	Items []struct {
		ID           string `json:"id"`
		Quantity     int    `json:"quantity"`
		Seller       string `json:"seller"`
		Availability string `json:"availability,omitempty"`
	} `json:"items"`
}

type ProductsList struct {
	Items []struct {
		ItemId string `json:"itemId"`
	} `json:"items"`
}

func VtexLegacySearch(searchUrl string, productSearch string, sellerID string) ([]string, []*httpx.Trace, error) {
	urlAfter := strings.TrimSuffix(searchUrl, "/")
	url := fmt.Sprintf("%s/%s", urlAfter, productSearch)

	traces := []*httpx.Trace{}

	req, err := httpx.NewRequest("GET", url, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &http.Client{}
	trace, err := httpx.DoTrace(client, req, nil, nil, -1)
	traces = append(traces, trace)
	if err != nil {
		return nil, traces, err
	}

	response := []ProductsList{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, traces, err
	}

	result := []string{}

	if len(response) == 0 {
		return result, traces, nil
	}

	result, traces, err = CartSimulation(response, sellerID, traces, urlAfter)
	if err != nil {
		return nil, traces, err
	}

	return result, traces, nil
}

func VtexIntelligentSearch(searchUrl string, productSearch string, sellerID string) ([]string, []*httpx.Trace, error) {

	traces := []*httpx.Trace{}

	query := url.Values{}
	query.Add("query", productSearch)
	query.Add("locale", "pt-BR")
	query.Add("hideUnavailableItems", "true")

	urlAfter := strings.TrimSuffix(searchUrl, "/")

	url_ := fmt.Sprintf("%s?%s", urlAfter, query.Encode())

	req, err := httpx.NewRequest("GET", url_, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &http.Client{}
	trace, err := httpx.DoTrace(client, req, nil, nil, -1)
	traces = append(traces, trace)
	if err != nil {
		return nil, traces, err
	}

	response := &struct {
		Products []ProductsList `json:"products"`
	}{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, traces, err
	}

	result := []string{}

	if len(response.Products) == 0 {
		return result, traces, nil
	}

	result, traces, err = CartSimulation(response.Products, sellerID, traces, urlAfter)
	if err != nil {
		return nil, traces, err
	}

	return result, traces, nil
}

func CartSimulation(products []ProductsList, sellerID string, traces []*httpx.Trace, url string) ([]string, []*httpx.Trace, error) {
	var body SearchSeller
	result := []string{}

	for i, product := range products {
		if i == 5 {
			break
		}
		product_retailer_id := product.Items[0].ItemId

		if sellerID != "" {
			result = append(result, product_retailer_id)
			body.Items = append(body.Items, struct {
				ID           string "json:\"id\""
				Quantity     int    "json:\"quantity\""
				Seller       string "json:\"seller\""
				Availability string "json:\"availability,omitempty\""
			}{ID: product_retailer_id, Quantity: 1, Seller: sellerID})
		} else {
			result = append(result, product_retailer_id+"#1")
		}
	}

	if len(body.Items) > 0 {

		urlSplit := strings.Split(url, "api")

		urlSimulation := urlSplit[0] + "api/checkout/pub/orderForms/simulation"

		headers := map[string]string{
			"Accept": "application/json",
		}
		var b io.Reader
		data, err := jsonx.Marshal(body)
		if err != nil {
			return nil, traces, err
		}
		b = bytes.NewReader(data)
		headers["Content-Type"] = "application/json"
		req, err := httpx.NewRequest("POST", urlSimulation, b, headers)
		if err != nil {
			return nil, traces, err
		}

		client := &http.Client{}
		trace, err := httpx.DoTrace(client, req, nil, nil, -1)
		traces = append(traces, trace)
		if err != nil {
			return nil, traces, err
		}

		if trace.Response.StatusCode >= 400 {
			return nil, traces, fmt.Errorf("error when searching with seller: status code %d", trace.Response.StatusCode)
		}

		response := &SearchSeller{}

		err = json.Unmarshal(trace.ResponseBody, response)
		if err != nil {
			return nil, traces, err
		}

		availableProducts := []string{}
		for _, item := range response.Items {
			if item.Availability == "available" {
				availableProducts = append(availableProducts, item.ID+"#"+sellerID)
			}
		}
		return availableProducts, traces, nil
	}

	return result, traces, nil
}

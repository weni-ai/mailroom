package catalogs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	hasVtexAds := params.HasVtexAds
	hideUnavailableItems := params.HideUnavailable
	productRetailerIDS := []string{}
	productRetailerIDMap := make(map[string]struct{})
	var productEntries []flows.ProductEntry
	var productEntry flows.ProductEntry
	searchResult := []string{}
	var searchResultSponsored string
	var trace *httpx.Trace
	var traces []*httpx.Trace
	var sellerID string
	var allProducts []string
	existingProductsIds := []string{}
	qttProducts := 5

	postalCode_ := strings.TrimSpace(params.PostalCode)
	if params.PostalCode != "" {
		postalCode_ = strings.ReplaceAll(params.PostalCode, "-", "")
		postalCode_ = strings.ReplaceAll(postalCode_, ".", "")

	}

	sellerID = strings.TrimSpace(params.SellerId)
	if sellerID == "" {
		sellerID = "1"
	}

	allProductsSponsored := []flows.ProductEntry{
		{
			Product:            languages[params.Language],
			ProductRetailerIDs: []string{},
		},
	}

	hasSponsored := false

	for _, product := range productList {
		if params.SearchType == "default" {
			searchResult, trace, err = GetProductListFromSentenX(product, catalog.FacebookCatalogID(), searchThreshold, s.rtConfig)
			callResult.Traces = append(callResult.Traces, trace)
		} else if params.SearchType == "vtex" {
			searchResult, searchResultSponsored, traces, err = GetProductListFromVtex(product, params.SearchUrl, params.ApiType, catalog.FacebookCatalogID(), s.rtConfig, hasVtexAds, hideUnavailableItems, sellerID)
			callResult.Traces = append(callResult.Traces, traces...)
			allProducts = append(allProducts, searchResult...)
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

		if len(searchResultSponsored) > 0 {
			hasSponsored = true
			allProductsSponsored[0].ProductRetailerIDs = append(allProductsSponsored[0].ProductRetailerIDs, searchResultSponsored)
		}
	}

	callResult.ProductRetailerIDS = productEntries

	fmt.Println("SIZE ProductRetailerIDS:", len(callResult.ProductRetailerIDS))

	// simulates cart in VTEX with all products
	hasSimulation := false
	if postalCode_ != "" && sellerID != "1" {
		var tracesSimulation []*httpx.Trace
		hasSimulation = true
		existingProductsIds, tracesSimulation, err = CartSimulation(allProducts, sellerID, params.SearchUrl, postalCode_)
		callResult.Traces = append(callResult.Traces, tracesSimulation...)
		if err != nil {
			return callResult, err
		}
	}

	// adds '#sellerID' formatting to the end of all retailer IDs
	for _, productEntry := range callResult.ProductRetailerIDS {
		for i, retailerID := range productEntry.ProductRetailerIDs {
			productEntry.ProductRetailerIDs[i] = retailerID + "#" + sellerID
		}
	}

	// search for products in Meta
	retries := 2
	var productSections []flows.ProductEntry
	var tracesMeta []*httpx.Trace
	for i := 0; i < retries; i++ {
		productSections, tracesMeta, err = ProductsSearchMeta(callResult.ProductRetailerIDS, fmt.Sprint(catalog.FacebookCatalogID()), s.rtConfig.WhatsappSystemUserToken)
		callResult.Traces = append(callResult.Traces, tracesMeta...)
		if err != nil {
			continue
		}
		break
	}
	if err != nil {
		return callResult, err
	}

	finalResult := &flows.MsgCatalogCall{}
	finalResult.Traces = callResult.Traces
	finalResult.ResponseJSON = callResult.ResponseJSON
	if hasSponsored {
		finalResult.ProductRetailerIDS = allProductsSponsored
	}

	// checks available products and limits to 5 per section
	for _, productEntry := range productSections {
		newEntry := productEntry
		newEntry.ProductRetailerIDs = []string{}
		for _, productRetailerID := range productEntry.ProductRetailerIDs {
			if hasSimulation {
				for _, existingProductId := range existingProductsIds {
					if productRetailerID == existingProductId+"#"+sellerID {
						if len(newEntry.ProductRetailerIDs) < qttProducts {
							newEntry.ProductRetailerIDs = append(newEntry.ProductRetailerIDs, productRetailerID)
						}
					}
				}
			} else {
				if len(newEntry.ProductRetailerIDs) < qttProducts {
					newEntry.ProductRetailerIDs = append(newEntry.ProductRetailerIDs, productRetailerID)
				}
			}
		}

		if len(newEntry.ProductRetailerIDs) > 0 {
			finalResult.ProductRetailerIDS = append(finalResult.ProductRetailerIDS, newEntry)
		}
	}

	return finalResult, nil
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
		Content: "Give me an unformatted JSON list containing strings with the full name of each product taken from the user prompt, preserving any multiple-word product names.",
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
	prompt6 := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleSystem,
		Content: "If the user does not provide a list of products or provides an invalid input, return an empty list of products.",
	}
	question := chatgpt.ChatCompletionMessage{
		Role:    chatgpt.ChatMessageRoleUser,
		Content: content,
	}
	completionRequest := chatgpt.NewChatCompletionRequest([]chatgpt.ChatCompletionMessage{prompt1, prompt2, prompt3, prompt4, prompt5, prompt6, question})
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

func GetProductListFromVtex(productSearch string, searchUrl string, apiType string, catalog string, rt *runtime.Config, hasVtexAds bool, hideUnavailableItems bool, sellerID string) ([]string, string, []*httpx.Trace, error) {
	var result []string
	var traces []*httpx.Trace
	var err error
	var productSponsored string

	if apiType == "legacy" {
		result, traces, err = VtexLegacySearch(searchUrl, productSearch)
		if err != nil {
			return nil, productSponsored, traces, err
		}
	} else if apiType == "intelligent" {
		result, traces, err = VtexIntelligentSearch(searchUrl, productSearch, hideUnavailableItems)
		if err != nil {
			return nil, productSponsored, traces, err
		}
	}
	if hasVtexAds {
		resultSponsored, tracesAds, err := VtexSponsoredSearch(searchUrl, productSearch, hideUnavailableItems)
		traces = append(traces, tracesAds...)
		if err != nil {
			return nil, productSponsored, traces, err
		}

		productRetailerIDS := []string{}
		var productEntries []flows.ProductEntry
		var productEntry flows.ProductEntry

		for _, productRetailerID := range resultSponsored {
			productRetailerIDS = append(productRetailerIDS, productRetailerID+"#"+sellerID)
		}

		if len(productRetailerIDS) > 0 {
			productEntry = flows.ProductEntry{
				Product:            searchUrl,
				ProductRetailerIDs: productRetailerIDS,
			}
			productEntries = append(productEntries, productEntry)

			retries := 2
			var newProductRetailerIDS []flows.ProductEntry
			var tracesMeta []*httpx.Trace
			for i := 0; i < retries; i++ {
				newProductRetailerIDS, tracesMeta, err = ProductsSearchMeta(productEntries, fmt.Sprint(catalog), rt.WhatsappSystemUserToken)
				if err != nil {
					continue
				}
				break
			}
			if len(newProductRetailerIDS) > 0 {
				productSponsored = newProductRetailerIDS[0].ProductRetailerIDs[0]
				traces = append(traces, tracesMeta...)
			}
		}

	}

	return result, productSponsored, traces, nil
}

type SearchSeller struct {
	Items      []Item `json:"items"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
}

type Item struct {
	ID           string `json:"id"`
	Quantity     int    `json:"quantity"`
	Seller       string `json:"seller"`
	Availability string `json:"availability,omitempty"`
}

type VtexProduct struct {
	ItemId string `json:"itemId"`
}

type VtexIntelligentProduct struct {
	Items []VtexProduct `json:"items"`
}

func VtexLegacySearch(searchUrl string, productSearch string) ([]string, []*httpx.Trace, error) {
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

	response := &[]struct {
		Items []VtexProduct `json:"items"`
	}{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, traces, err
	}

	allItems := []string{}

	for _, items := range *response {
		for _, item := range items.Items {
			allItems = append(allItems, item.ItemId)
		}
	}

	if len(allItems) == 0 {
		return nil, traces, nil
	}

	return allItems, traces, nil
}

func VtexIntelligentSearch(searchUrl string, productSearch string, hideUnavailableItems bool) ([]string, []*httpx.Trace, error) {

	traces := []*httpx.Trace{}

	searchUrlParts := strings.Split(searchUrl, "?")
	searchUrl = searchUrlParts[0]
	queryParams := map[string][]string{}
	var err error
	if len(searchUrlParts) > 1 {
		queryParams, err = url.ParseQuery(searchUrlParts[1])
		if err != nil {
			return nil, nil, err
		}
	}

	query := url.Values{}
	query.Add("query", productSearch)

	hideUnavailable := "true"
	if !hideUnavailableItems {
		hideUnavailable = "false"
	}

	query.Add("hideUnavailableItems", hideUnavailable)

	for key, value := range queryParams {
		query.Add(key, value[0])
	}

	// add default pt-BR locale
	if _, ok := queryParams["locale"]; !ok {
		query.Add("locale", "pt-BR")
	}

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
		Products []VtexIntelligentProduct `json:"products"`
	}{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, traces, err
	}

	allItems := []string{}

	for _, items := range response.Products {
		for _, item := range items.Items {
			allItems = append(allItems, item.ItemId)
		}
	}

	if len(allItems) == 0 {
		return nil, traces, nil
	}

	return allItems, traces, nil
}

func VtexSponsoredSearch(searchUrl string, productSearch string, hideUnavailableItems bool) ([]string, []*httpx.Trace, error) {
	traces := []*httpx.Trace{}

	query := url.Values{}
	query.Add("query", productSearch)
	query.Add("locale", "pt-BR")

	hideUnavailable := "true"
	if !hideUnavailableItems {
		hideUnavailable = "false"
	}

	query.Add("hideUnavailableItems", hideUnavailable)

	parsedURL, err := url.Parse(searchUrl)
	if err != nil {
		fmt.Println("Erro ao fazer parse da URL:", err)
		return nil, nil, err
	}
	domain := parsedURL.Host

	url_ := fmt.Sprintf("http://%s/api/io/_v/api/intelligent-search/sponsored_products?%s", domain, query.Encode())

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

	response := &[]struct {
		Items []VtexProduct `json:"items"`
	}{}

	err = jsonx.Unmarshal(trace.ResponseBody, &response)
	if err != nil {
		return nil, traces, err
	}

	allItems := []string{}

	for _, items := range *response {
		for _, item := range items.Items {
			allItems = append(allItems, item.ItemId)
		}
	}

	if len(allItems) == 0 {
		return nil, traces, nil
	}

	return allItems, traces, nil

}

func CartSimulation(products []string, sellerID string, url string, postalCode string) ([]string, []*httpx.Trace, error) {
	const batchSize = 300
	var traces []*httpx.Trace
	var availableProducts []string

	urlSplit := strings.Split(url, "api")
	urlSimulation := urlSplit[0] + "api/checkout/pub/orderForms/simulation"

	for i := 0; i < len(products); i += batchSize {
		end := i + batchSize
		if end > len(products) {
			end = len(products)
		}
		batchProducts := products[i:end]

		var searchSeller SearchSeller
		if postalCode != "" {
			searchSeller.PostalCode = postalCode
			searchSeller.Country = "BRA"
		}
		for _, product := range batchProducts {
			searchSeller.Items = append(searchSeller.Items, Item{ID: product, Quantity: 1, Seller: sellerID})
		}

		batchAvailableProducts, trace, err := sendBatchRequest(searchSeller, urlSimulation)
		traces = append(traces, trace)
		if err != nil {
			return nil, traces, err
		}

		availableProducts = append(availableProducts, batchAvailableProducts...)
	}

	return availableProducts, traces, nil
}

func sendBatchRequest(body SearchSeller, url string) ([]string, *httpx.Trace, error) {
	client := &http.Client{}

	headers := map[string]string{
		"Accept": "application/json",
	}

	data, err := jsonx.Marshal(body)
	if err != nil {
		return nil, nil, err
	}

	req, err := httpx.NewRequest("POST", url, bytes.NewReader(data), headers)
	if err != nil {
		return nil, nil, err
	}

	trace, err := httpx.DoTrace(client, req, nil, nil, -1)
	if err != nil {
		return nil, trace, err
	}

	if trace.Response.StatusCode >= 400 {
		return nil, trace, fmt.Errorf("error when searching with seller: status code %d", trace.Response.StatusCode)
	}

	response := &SearchSeller{}
	err = json.Unmarshal(trace.ResponseBody, response)
	if err != nil {
		return nil, trace, err
	}

	availableProducts := []string{}
	for _, item := range response.Items {
		if item.Availability == "available" {
			availableProducts = append(availableProducts, item.ID)
		}
	}

	return availableProducts, trace, nil
}

// Filter represents the structure of the filter for the API request
type Filter struct {
	Or []OrCondition `json:"or"`
}

// OrCondition represents an OR condition
type OrCondition struct {
	And []AndCondition `json:"and"`
}

// AndCondition represents an AND condition
type AndCondition struct {
	RetailerID   map[string]string `json:"retailer_id,omitempty"`
	Availability map[string]string `json:"availability,omitempty"`
	Visibility   map[string]string `json:"visibility,omitempty"`
}

// createFilter creates the filter JSON based on the list of retailer IDs
func createFilter(productEntryList []string) (string, error) {
	var filter Filter

	for _, id := range productEntryList {
		andCondition := []AndCondition{
			{
				RetailerID: map[string]string{"i_contains": id},
			},
			{
				Availability: map[string]string{"i_contains": "in stock"},
			},
			{
				Visibility: map[string]string{"i_contains": "published"},
			},
		}
		filter.Or = append(filter.Or, OrCondition{And: andCondition})

	}

	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}

	return string(filterJSON), nil
}

// Response represents the structure of the API response
type Response struct {
	Data []struct {
		RetailerID string `json:"retailer_id"`
	} `json:"data"`
}

func fetchProducts(url string) (*Response, *httpx.Trace, error) {
	client := &http.Client{}

	req, err := httpx.NewRequest("GET", url, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	t, err := httpx.DoTrace(client, req, nil, nil, -1)
	t.Request.URL = truncateURL(t.Request.URL)
	if err != nil {
		return nil, t, err
	}

	if t.Response.StatusCode >= 400 {
		return nil, t, fmt.Errorf("error when searching in meta: status code %d", t.Response.StatusCode)
	}

	var response Response
	err = json.Unmarshal(t.ResponseBody, &response)
	if err != nil {
		return nil, t, err
	}

	return &response, t, err

}

func ProductsSearchMeta(productEntryList []flows.ProductEntry, catalog string, whatsappSystemUserToken string) ([]flows.ProductEntry, []*httpx.Trace, error) {
	const batchSize = 15
	validProductIds := []string{}
	traces := []*httpx.Trace{}
	allIds := []string{}
	for _, productEntry := range productEntryList {
		allIds = append(allIds, productEntry.ProductRetailerIDs...)
	}

	fmt.Println("SIZE ALLIDS: ", len(allIds))

	newProductEntryList := []flows.ProductEntry{}

	for i := 0; i < len(allIds); i += batchSize {
		end := i + batchSize
		if end > len(allIds) {
			end = len(allIds)
		}

		batchIds := allIds[i:end]
		filter, err := createFilter(batchIds)
		if err != nil {
			return nil, nil, err
		}

		params := url.Values{}
		params.Add("fields", "[\"category\",\"name\",\"retailer_id\",\"availability\"]")
		params.Add("summary", "true")
		params.Add("access_token", whatsappSystemUserToken)
		params.Add("filter", filter)

		url_ := fmt.Sprintf("https://graph.facebook.com/v14.0/%s/products?%s", catalog, params.Encode())

		response, trace, err := fetchProducts(url_)
		traces = append(traces, trace)
		if err != nil {
			return nil, traces, err
		}

		for _, id := range response.Data {
			validProductIds = append(validProductIds, id.RetailerID)
		}
	}

	for i, productEntry := range productEntryList {
		newProductEntryList = append(newProductEntryList, flows.ProductEntry{Product: productEntry.Product, ProductRetailerIDs: []string{}})
		for _, retailerId := range productEntry.ProductRetailerIDs {
			for _, id := range validProductIds {
				if retailerId == id {
					newProductEntryList[i].ProductRetailerIDs = append(newProductEntryList[i].ProductRetailerIDs, id)
					break
				}
			}
		}
	}

	return newProductEntryList, traces, nil
}

var languages = map[string]string{
	"eng": "You may also like:",
	"por": "Você também pode gostar:",
	"spa": "También te puede interesar:",
}

func truncateURL(u *url.URL) *url.URL {
	const maxLength = 2048
	if len(u.String()) > maxLength {
		excessLength := len(u.String()) - maxLength
		if excessLength < len(u.RawQuery) {
			u.RawQuery = u.RawQuery[:len(u.RawQuery)-excessLength]
		} else {
			u.RawQuery = ""
		}
	}
	return u
}

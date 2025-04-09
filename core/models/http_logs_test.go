package models_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPLogs(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer func() { db.MustExec(`DELETE FROM request_logs_httplog`) }()

	// insert a classifier log
	log := models.NewClassifierCalledLog(testdata.Org1.ID, testdata.Wit.ID, "http://foo.bar", 200, "GET /", "STATUS 200", false, time.Second, 0, time.Now())
	err := models.InsertHTTPLogs(ctx, db, []*models.HTTPLog{log})
	assert.Nil(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from request_logs_httplog WHERE org_id = $1 AND status_code = 200 AND classifier_id = $2 AND is_error = FALSE`, testdata.Org1.ID, testdata.Wit.ID).Returns(1)

	// insert a log with nil response
	log = models.NewClassifierCalledLog(testdata.Org1.ID, testdata.Wit.ID, "http://foo.bar", 0, "GET /", "", true, time.Second, 0, time.Now())
	err = models.InsertHTTPLogs(ctx, db, []*models.HTTPLog{log})
	assert.Nil(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from request_logs_httplog WHERE org_id = $1 AND status_code = 0 AND classifier_id = $2 AND is_error = TRUE AND response IS NULL`, testdata.Org1.ID, testdata.Wit.ID).Returns(1)

	// insert a webhook log
	log = models.NewWebhookCalledLog(testdata.Org1.ID, testdata.Favorites.ID, "http://foo.bar", 400, "GET /", "HTTP 200", false, time.Second, 2, time.Now(), testdata.Cathy.ID)
	err = models.InsertHTTPLogs(ctx, db, []*models.HTTPLog{log})
	assert.Nil(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from request_logs_httplog WHERE org_id = $1 AND status_code = 400 AND flow_id = $2 AND num_retries = 2`, testdata.Org1.ID, testdata.Favorites.ID).Returns(1)
}

func TestHTTPLogger(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://temba.io": {
			httpx.NewMockResponse(200, nil, `hello`),
			httpx.NewMockResponse(400, nil, `world`),
		},
	}))

	mailgun, err := models.LookupTicketerByUUID(ctx, db, testdata.Mailgun.UUID)
	require.NoError(t, err)

	logger := &models.HTTPLogger{}
	log := logger.Ticketer(mailgun)

	// make and log a few HTTP requests
	req1, err := http.NewRequest("GET", "https://temba.io", nil)
	require.NoError(t, err)
	trace1, err := httpx.DoTrace(http.DefaultClient, req1, nil, nil, -1)
	require.NoError(t, err)
	log(flows.NewHTTPLog(trace1, flows.HTTPStatusFromCode, nil))

	req2, err := http.NewRequest("GET", "https://temba.io", nil)
	require.NoError(t, err)
	trace2, err := httpx.DoTrace(http.DefaultClient, req2, nil, nil, -1)
	require.NoError(t, err)
	log(flows.NewHTTPLog(trace2, flows.HTTPStatusFromCode, nil))

	err = logger.Insert(ctx, db)
	assert.NoError(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from request_logs_httplog WHERE org_id = $1 AND ticketer_id = $2`, testdata.Org1.ID, testdata.Mailgun.ID).Returns(2)
}

func TestHTTPLogsWithTruncatedURL(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer func() { db.MustExec(`DELETE FROM request_logs_httplog`) }()

	bigURL := buildBigURL()
	log := models.NewWebhookCalledLog(testdata.Org1.ID, testdata.Favorites.ID, bigURL, 400, "GET /", "HTTP 200", false, time.Second, 2, time.Now(), testdata.Cathy.ID)
	err := models.InsertHTTPLogs(ctx, db, []*models.HTTPLog{log})
	assert.Nil(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from request_logs_httplog WHERE org_id = $1 AND status_code = 400 AND flow_id = $2 AND num_retries = 2`, testdata.Org1.ID, testdata.Favorites.ID).Returns(1)
}

func TestTruncateURL(t *testing.T) {
	url := buildBigURL()

	assert.Equal(t, 2484, len(url))

	truncatedUrl := models.TruncateURL(url)

	assert.Equal(t, 2048, len(truncatedUrl))
}

func buildBigURL() string {
	baseUrl := "https://foo.bar.com"
	queryParams := map[string]string{
		"foo":     "0b053110-acd8-4206-86de-e7af7baefab6",
		"bar":     "64f82885-3d92-443b-8806-eae7ce32d97f",
		"baz":     "37902130-a08c-48f4-875f-596a1f5b8669",
		"qux":     "9382c977-b8a9-4678-9555-47256875407e",
		"quux":    "46238899-7256-4241-8c8e-59d9b779588a",
		"corge":   "86401927-7429-4085-875f-d8344759f523",
		"grault":  "75839504-906b-43b6-859b-780f5653a788",
		"garply":  "98204827-6704-4727-8370-765d47647655",
		"waldo":   "47417555-7c81-4147-8825-355175060527",
		"fred":    "b62b244e-6270-4c97-8508-396d91904560",
		"plugh":   "97681141-50b8-456a-8561-021549077024",
		"xyzzy":   "99999999-9999-9999-9999-999999999999",
		"thud":    "12345678-90ab-cdef-1234-567890abcdef0",
		"quuz":    "abcdef01-2345-6789-abcd-ef0123456789",
		"xyzzyy":  "12345678-90ab-cdef-1234-567890abcdef0",
		"thudd":   "12345678-90ab-cdef-1234-567890abcdef0",
		"ping":    "2116fd91-64ff-4d7a-bbe8-f381d0f80b2e",
		"pong":    "a3c7f453-f172-4030-8864-4758709d14b1",
		"fong":    "12345678-90ab-cdef-1234-567890abcdef0",
		"foo2":    "0b053110-acd8-4206-86de-e7af7baefab6",
		"bar2":    "64f82885-3d92-443b-8806-eae7ce32d97f",
		"baz2":    "37902130-a08c-48f4-875f-596a1f5b8669",
		"qux2":    "9382c977-b8a9-4678-9555-47256875407e",
		"quux2":   "46238899-7256-4241-8c8e-59d9b779588a",
		"corge2":  "86401927-7429-4085-875f-d8344759f523",
		"grault2": "75839504-906b-43b6-859b-780f5653a788",
		"garply2": "98204827-6704-4727-8370-765d47647655",
		"waldo2":  "47417555-7c81-4147-8825-355175060527",
		"fred2":   "b62b244e-6270-4c97-8508-396d91904560",
		"plugh2":  "97681141-50b8-456a-8561-021549077024",
		"xyzzy2":  "99999999-9999-9999-9999-999999999999",
		"thud2":   "12345678-90ab-cdef-1234-567890abcdef0",
		"quuz2":   "abcdef01-2345-6789-abcd-ef0123456789",
		"xyzzyy2": "12345678-90ab-cdef-1234-567890abcdef0",
		"thudd2":  "12345678-90ab-cdef-1234-567890abcdef0",
		"ping2":   "2116fd91-64ff-4d7a-bbe8-f381d0f80b2e",
		"pong2":   "a3c7f453-f172-4030-8864-4758709d14b1",
		"fong2":   "12345678-90ab-cdef-1234-567890abcdef0",
		"foo3":    "0b053110-acd8-4206-86de-e7af7baefab6",
		"bar3":    "64f82885-3d92-443b-8806-eae7ce32d97f",
		"baz3":    "37902130-a08c-48f4-875f-596a1f5b8669",
		"qux3":    "9382c977-b8a9-4678-9555-47256875407e",
		"quux3":   "46238899-7256-4241-8c8e-59d9b779588a",
		"corge3":  "86401927-7429-4085-875f-d8344759f523",
		"grault3": "75839504-906b-43b6-859b-780f5653a788",
		"garply3": "98204827-6704-4727-8370-765d47647655",
		"waldo3":  "47417555-7c81-4147-8825-355175060527",
		"fred3":   "b62b244e-6270-4c97-8508-396d91904560",
		"plugh3":  "97681141-50b8-456a-8561-021549077024",
		"xyzzy3":  "99999999-9999-9999-9999-999999999999",
		"thud3":   "12345678-90ab-cdef-1234-567890abcdef0",
		"quuz3":   "abcdef01-2345-6789-abcd-ef0123456789",
		"xyzzyy3": "12345678-90ab-cdef-1234-567890abcdef0",
		"thudd3":  "12345678-90ab-cdef-1234-567890abcdef0",
		"ping3":   "2116fd91-64ff-4d7a-bbe8-f381d0f80b2e",
		"pong3":   "a3c7f453-f172-4030-8864-4758709d14b1",
		"fong3":   "12345678-90ab-cdef-1234-567890abcdef0",
	}

	var queryString = ""
	for key, value := range queryParams {
		if queryString != "" {
			queryString += "&"
		}
		queryString += fmt.Sprintf("%s=%s", key, value)
	}

	return fmt.Sprintf("%s?%s", baseUrl, queryString)
}

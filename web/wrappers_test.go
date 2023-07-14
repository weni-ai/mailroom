package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithHTTPLogs(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer db.MustExec(`DELETE FROM request_logs_httplog`)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://temba.io": {
			httpx.NewMockResponse(200, nil, `hello`),
			httpx.NewMockResponse(400, nil, `world`),
		},
	}))

	handler := func(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
		ticketer, _ := models.LookupTicketerByUUID(ctx, rt.DB, testdata.Mailgun.UUID)

		logger := l.Ticketer(ticketer)

		// make and log a few HTTP requests
		req1, err := http.NewRequest("GET", "https://temba.io", nil)
		require.NoError(t, err)
		trace1, err := httpx.DoTrace(http.DefaultClient, req1, nil, nil, -1)
		require.NoError(t, err)
		logger(flows.NewHTTPLog(trace1, flows.HTTPStatusFromCode, nil))

		req2, err := http.NewRequest("GET", "https://temba.io", nil)
		require.NoError(t, err)
		trace2, err := httpx.DoTrace(http.DefaultClient, req2, nil, nil, -1)
		require.NoError(t, err)
		logger(flows.NewHTTPLog(trace2, flows.HTTPStatusFromCode, nil))

		return map[string]string{"status": "OK"}, http.StatusOK, nil
	}

	// simulate handler being invoked by server
	wrapped := web.WithHTTPLogs(handler)
	response, status, err := wrapped(ctx, rt, nil)

	// check response from handler
	assert.Equal(t, map[string]string{"status": "OK"}, response)
	assert.Equal(t, http.StatusOK, status)
	assert.NoError(t, err)

	// check HTTP logs were created
	testsuite.AssertQuery(t, db, `select count(*) from request_logs_httplog where ticketer_id = $1;`, testdata.Mailgun.ID).Returns(2)
}

var rtTest *runtime.Runtime

type RequireUserTokenTC struct {
	Title         string
	PrepareDBSQL  string
	Request       *RequestCase
	HandlerReturn *HandlerReturn

	Body string
	Code int
}

type RequestCase struct {
	Method string
	Body   string
	// Header *http.Header
	Header map[string]string
}

type HandlerReturn struct {
	Value  interface{}
	Status int
	Err    error
}

var RequireUserTokenTCs = []RequireUserTokenTC{
	{
		Title:        "Test Valid Token",
		PrepareDBSQL: `INSERT INTO api_apitoken	(is_active, "key", created, org_id, role_id, user_id) VALUES(true, '123456-abcdef', '2023-07-10 18:45:39.678', 1, 8, 3);`,
		Request: &RequestCase{
			Method: "GET",
			Header: map[string]string{"key": "Authorization", "value": "Token 123456-abcdef"},
		},
		HandlerReturn: &HandlerReturn{
			Value:  map[string]interface{}{},
			Status: 200,
			Err:    nil,
		},
		Body: "{}",
		Code: 200,
	},

	{
		Title: "Test Without Token",
		Request: &RequestCase{
			Method: "GET",
		},
		HandlerReturn: &HandlerReturn{
			Value:  map[string]interface{}{},
			Status: 200,
			Err:    nil,
		},
		Body: "{}",
		Code: 401,
	},

	{
		Title: "Test With Invalid Unauthorized Token",
		Request: &RequestCase{
			Method: "GET",
			Header: map[string]string{"key": "Authorization", "value": "Token 00000-00000"},
		},
		HandlerReturn: &HandlerReturn{
			Value:  map[string]interface{}{},
			Status: 200,
			Err:    nil,
		},
		Body: "{}",
		Code: 401,
	},

	{
		Title:        "Test error looking up Token",
		PrepareDBSQL: `DROP TABLE api_apitoken;`,
		Request: &RequestCase{
			Method: "GET",
			Header: map[string]string{"key": "Authorization", "value": "Token 00000-00000"},
		},
		HandlerReturn: &HandlerReturn{
			Value:  map[string]interface{}{},
			Status: 200,
			Err:    nil,
		},
		Body: "{}",
		Code: 401,
	},

	{
		Title: "Test Valid Token",
		PrepareDBSQL: `
		CREATE TABLE api_apitoken (
			is_active bool NOT NULL,
			"key" varchar(40) NOT NULL,
			created timestamptz NOT NULL,
			org_id varchar(10) NOT NULL,
			role_id int4 NOT NULL,
			user_id int4 NOT NULL
		);

		ALTER TABLE orgs_org RENAME TO orgs_org_temp;

		CREATE TABLE orgs_org (
			id varchar(40) NOT NULL,
			is_active bool NOT NULL
		);

		INSERT INTO public.orgs_org (id, is_active) VALUES('abc', true);

		INSERT INTO api_apitoken	(is_active, "key", created, org_id, role_id, user_id) VALUES(true, '123456-abcdef', '2023-07-10 18:45:39.678', 'abc', 8, 3);
		`,
		Request: &RequestCase{
			Method: "GET",
			Header: map[string]string{"key": "Authorization", "value": "Token 123456-abcdef"},
		},
		HandlerReturn: &HandlerReturn{
			Value:  map[string]interface{}{},
			Status: 200,
			Err:    nil,
		},
		Body: `{
    "error": "error scanning auth row: sql: Scan error on column index 1, name \"org_id\": converting driver.Value type string (\"abc\") to a int: invalid syntax"
}`,
		Code: 500,
	},
}

func TestRequireUserTokenCases(t *testing.T) {
	_, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)
	rtTest = rt

	for _, tc := range RequireUserTokenTCs {
		if tc.PrepareDBSQL != "" {
			_, err := db.Exec(tc.PrepareDBSQL)
			assert.NoError(t, err)
			if err != nil {
				return
			}
		}

		var body io.Reader = nil
		req, err := http.NewRequest(tc.Request.Method, "/", body)
		assert.NoError(t, err)

		if tc.Request.Header != nil {
			req.Header.Add(tc.Request.Header["key"], tc.Request.Header["value"])
		}

		rr := httptest.NewRecorder()

		jh := web.RequireUserToken(func(context context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
			return tc.HandlerReturn.Value, tc.HandlerReturn.Status, tc.HandlerReturn.Err
		})

		rUserTokenHandler := WrapJSONHandler(jh)
		rUserTokenHandler.ServeHTTP(rr, req)

		assert.Equal(t, tc.Code, rr.Code)
		rrBody := rr.Body.String()
		assert.Equal(t, tc.Body, rrBody)
	}
}

func WrapJSONHandler(handler web.JSONHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		value, status, err := handler(r.Context(), rtTest, r)

		if err != nil {
			value = web.NewErrorResponse(err)
		}

		serialized, serr := jsonx.MarshalPretty(value)
		if serr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "error serializing handler response"}`))
			return
		}

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(serialized)
			return
		}

		w.WriteHeader(status)
		w.Write(serialized)
	}
}

func TestRequireAuthToken(t *testing.T) {
	_, rt, _, _ := testsuite.Get()
	rtTest = rt

	t.Run("test with no authorization token configured", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/", nil)
		assert.NoError(t, err)

		rr := httptest.NewRecorder()

		rth := web.RequireAuthToken(func(context context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
			return nil, 200, nil
		})

		wp := WrapJSONHandler(rth)
		wp.ServeHTTP(rr, req)

		assert.Equal(t, 200, rr.Code)
	})

	t.Run("test with no authorization token", func(t *testing.T) {
		rtTest.Config.AuthToken = "abcdef-123456"
		req, err := http.NewRequest("GET", "/", nil)
		assert.NoError(t, err)

		rr := httptest.NewRecorder()

		rth := web.RequireAuthToken(func(context context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
			return nil, 401, nil
		})

		wp := WrapJSONHandler(rth)
		wp.ServeHTTP(rr, req)

		assert.Equal(t, 401, rr.Code)
	})

	t.Run("test with valid http token", func(t *testing.T) {
		rtTest.Config.AuthToken = "abcdef-123456"
		req, err := http.NewRequest("GET", "/", nil)
		assert.NoError(t, err)

		req.Header.Set("Authorization", "Token abcdef-123456")

		rr := httptest.NewRecorder()

		rth := web.RequireAuthToken(func(context context.Context, rt *runtime.Runtime, r *http.Request) (interface{}, int, error) {
			return nil, 401, nil
		})

		wp := WrapJSONHandler(rth)
		wp.ServeHTTP(rr, req)

		assert.Equal(t, 401, rr.Code)
	})

	t.Run("test with httplogs", func(t *testing.T) {
		rtTest.Config.AuthToken = "abcdef-123456"
		req, err := http.NewRequest("GET", "/", nil)
		assert.NoError(t, err)

		req.Header.Set("Authorization", "Token abcdef-123456")

		rr := httptest.NewRecorder()

		rth := web.RequireAuthToken(web.WithHTTPLogs(func(context context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
			return nil, 401, nil
		}))

		wp := WrapJSONHandler(rth)
		wp.ServeHTTP(rr, req)

		assert.Equal(t, 401, rr.Code)
	})
}

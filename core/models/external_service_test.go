package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
)

var esTest = &struct {
	UUID assets.ExternalServiceUUID
	ID   models.ExternalServiceID
}{
	UUID: "4a771dac-b65b-4236-97b3-c785de3f65d3",
	ID:   1000,
}

func TestExternalServices(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)

	es, err := models.LookupExternalServiceByUUID(ctx, db, esTest.UUID)
	assert.NoError(t, err)
	assert.Equal(t, esTest.ID, es.ID())
	assert.Equal(t, esTest.UUID, es.UUID())
	assert.Equal(t, "External Service Weni", es.Name())
	assert.Equal(t, "1234-abcd", es.Config("service_id"))
	assert.Equal(t, "543210", es.Config("service_token"))

	org1, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	assert.NoError(t, err)

	es = org1.ExternalServiceByID(esTest.ID)
	assert.Equal(t, esTest.UUID, es.UUID())
	assert.Equal(t, "External Service Weni", es.Name())
	assert.Equal(t, "1234-abcd", es.Config("service_id"))

	es = org1.ExternalServiceByUUID(esTest.UUID)
	assert.Equal(t, esTest.UUID, es.UUID())
	assert.Equal(t, "External Service Weni", es.Name())
	assert.Equal(t, "1234-abcd", es.Config("service_id"))

	es.UpdateConfig(ctx, db, map[string]string{"new_key": "foo"}, map[string]bool{"service_id": true})

	org1, _ = models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshExternalServices)
	es = org1.ExternalServiceByID(esTest.ID)

	assert.Equal(t, "foo", es.Config("new_key"))
	assert.Equal(t, "", es.Config("service_id"))

	_, err = db.Exec(promptDDL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO public.externals_externalservice
	(is_active, created_on, modified_on, uuid, external_service_type, "name", config, created_by_id, modified_by_id, org_id)
	VALUES(true, '2023-06-16 19:14:03.690', '2023-06-16 19:14:03.690', '0f7f704f-b50a-4bf6-8903-9ae34df14ebd'::uuid, 'chatgpt', 'chatgpt_test', '{"rules": "rule test", "top_p": "0.1", "api_key": "sk-123", "ai_model": "gpt-3.5-turbo", "temperature": "0.1", "knowledge_base": "knowledge base test"}'::jsonb, $1, $2, $3);`,
		testdata.Admin.ID, testdata.Admin.ID, testdata.Org1.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO public.externals_prompt
	(is_active, created_on, modified_on, uuid, "text", chat_gpt_service_id, created_by_id, modified_by_id)
	VALUES(true, '2023-06-19 18:01:31.357', '2023-06-19 18:01:31.357', '1ca7a60b-3408-47c7-a232-8f7a84140572', 'prompt test', $1, $2, $3);`,
		2, testdata.Admin.ID, testdata.Org1.ID)
	if err != nil {
		t.Fatal(err)
	}

	ctxp, cancelp := context.WithTimeout(ctx, time.Second*5)
	defer cancelp()
	prompts, err := models.SelectPromptsByExternalServiceID(ctxp, db, 2)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(prompts))
}

const (
	promptDDL = `
	CREATE TABLE public.externals_prompt (
		id serial4 NOT NULL,
		is_active bool NOT NULL,
		created_on timestamptz NOT NULL,
		modified_on timestamptz NOT NULL,
		uuid varchar(36) NOT NULL,
		"text" text NOT NULL,
		chat_gpt_service_id int4 NOT NULL,
		created_by_id int4 NOT NULL,
		modified_by_id int4 NOT NULL,
		CONSTRAINT externals_prompt_pkey PRIMARY KEY (id),
		CONSTRAINT externals_prompt_uuid_key UNIQUE (uuid)
	);
	CREATE INDEX externals_prompt_chat_gpt_service_id_b582ecb2 ON public.externals_prompt USING btree (chat_gpt_service_id);
	CREATE INDEX externals_prompt_created_by_id_f24926d4 ON public.externals_prompt USING btree (created_by_id);
	CREATE INDEX externals_prompt_modified_by_id_c69ce3ab ON public.externals_prompt USING btree (modified_by_id);
	CREATE INDEX externals_prompt_uuid_f141c6b8_like ON public.externals_prompt USING btree (uuid varchar_pattern_ops);
`
)

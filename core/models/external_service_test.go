package models_test

import (
	"testing"

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
	assert.Equal(t, "543210", es.Config("service_id"))
}

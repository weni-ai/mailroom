package models_test

import (
	"testing"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
)

func TestTokens(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	_, err := models.LookupOrgByUUIDAndToken(ctx, db, "123", "Administrators", "123")
	assert.EqualError(t, err, `pq: invalid input syntax for type uuid: "123"`)

	adminToken := "5c26a50841ff48237238bbdd021150f6a33a4199"

	orNone, err := models.LookupOrgByUUIDAndToken(ctx, db, testdata.Org1.UUID, "Administrators", adminToken)
	assert.NoError(t, err)
	assert.Nil(t, orNone)

	db.MustExec(`INSERT INTO public.api_apitoken
	(is_active, "key", created, org_id, role_id, user_id)
	VALUES(true, $1, 'now()', $2, 8, 3);
	`, adminToken, testdata.Org1.ID)

	or, err := models.LookupOrgByUUIDAndToken(ctx, db, testdata.Org1.UUID, "Administrators", adminToken)
	assert.NoError(t, err)
	assert.Equal(t, testdata.Org1.ID, or.ID)
	assert.Equal(t, testdata.Org1.UUID, or.UUID)
}

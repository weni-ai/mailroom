package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
)

func TestCatalogProducts(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetDB)

	// _, err := db.Exec(catalogProductDDL)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	_, err := db.Exec(`INSERT INTO public.wpp_products_catalog
	(uuid, facebook_catalog_id, "name", created_on, modified_on, is_active, channel_id, org_id)
	VALUES('2be9092a-1c97-4b24-906f-f0fbe3e1e93e', '123456789', 'Catalog Dummy', now(), now(), true, $1, $2);
	`, testdata.Org2Channel.ID, testdata.Org2.ID)
	assert.NoError(t, err)

	ctp, err := models.GetActiveCatalogFromChannel(ctx, *db, testdata.Org2Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, ctp.IsActive())

	_, err = db.Exec(`INSERT INTO public.wpp_products_catalog
	(uuid, facebook_catalog_id, "name", created_on, modified_on, is_active, channel_id, org_id)
	VALUES('9bbe354d-cea6-408b-ba89-9ce28999da3f', '1234567891', 'Catalog Dummy2', now(), now(), false, $1, $2);
	`, 123, testdata.Org2.ID)

	assert.NoError(t, err)

	ctpn, err := models.GetActiveCatalogFromChannel(ctx, *db, 123)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, &models.MsgCatalog{}, ctpn)

}

func TestChannelUUIDForChannelID(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)

	ctxp, cancelp := context.WithTimeout(ctx, time.Second*5)
	defer cancelp()

	ctp, err := models.ChannelUUIDForChannelID(ctxp, db, testdata.TwilioChannel.ID)
	assert.NoError(t, err)
	assert.Equal(t, ctp, testdata.TwilioChannel.UUID)
}

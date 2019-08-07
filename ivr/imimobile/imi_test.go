package imi

import (
	"fmt"
	"encoding/xml"
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	// "github.com/nyaruka/goflow/flows/routers/waits"
	// "github.com/nyaruka/goflow/flows/routers/waits/hints"
	// "github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/goflow/utils/uuids"
	"github.com/nyaruka/mailroom/config"
	// "github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/models"
	"github.com/stretchr/testify/assert"
)

func TestResponseForSprint(t *testing.T) {
	// for tests it is more convenient to not have formatted output
	// _, db, rp := testsuite.Reset()
	// rc := rp.Get()
	// defer rc.Close()
	models.FlushCache()

	indentMarshal = false

	urn := urns.URN("tel:+12067799294")
	channelUUID := assets.ChannelUUID(uuids.New())
	channelRef := assets.NewChannelReference(channelUUID, "IMIMobile Channel")

	resumeURL := "http://temba.io/resume?session=1"

	// db.MustExec(`UPDATE channels_channel SET config = '{"send_url": "http://eapps.imimobile.com/rapidpro/api/outbound/OBDInit", "phone_number": "91123456789", "username": "unicef", "password": "HmGWbdCFiJBj5bui"}' WHERE id = $1`, channelUUID)

	vxmlResponse := `<vxml version="2.1"><property name="confidencelevel" value="0.5" /><property name="maxage" value="30" /><property name="inputmodes" value="dtmf" /><property name="interdigittimeout" value="12s" /><property name="timeout" value="12s" /><property name="termchar" value="#" /><var name="recieveddtmf" /><var name="ExecuteVXML" /><form id="AcceptDigits"><var name="ExecuteVXML" /><var name="nResult" /><var name="nResultCode" expr="0" />%s</form></vxml>`

	// set our attachment domain for testing
	config.Mailroom.AttachmentDomain = "mailroom.io"
	defer func() { config.Mailroom.AttachmentDomain = "" }()

	tcs := []struct {
		Events   []flows.Event
		Wait     flows.ActivatedWait
		Expected string
	}{
		{
			[]flows.Event{events.NewIVRCreatedEvent(flows.NewMsgOut(urn, channelRef, "hello world", nil, nil, nil))},
			nil,
			fmt.Sprintf(vxmlResponse, `<block><prompt bargein="true">hello world</prompt></block><exit />`),
		},
	}

	for i, tc := range tcs {
		response, err := responseForSprint(resumeURL, tc.Wait, tc.Events)
		assert.NoError(t, err, "%d: unexpected error")
		assert.Equal(t, xml.Header+tc.Expected, response, "%d: unexpected response", i)
	}
}

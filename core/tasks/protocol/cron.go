package protocol

import (
	"context"
	"sync"
	"time"

	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/utils/cron"
	"github.com/sirupsen/logrus"
)

func init() {
	mailroom.AddInitFunction(startProtocolCron)
}

func startProtocolCron(rt *runtime.Runtime, wg *sync.WaitGroup, quit chan bool) error {
	cron.Start(quit, rt, "protocol_expiry", time.Minute, false, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := models.ExpireDueProtocols(ctx, rt.DB)
		if err != nil {
			logrus.WithError(err).Error("error expiring protocol timers")
		}
		return err
	})
	return nil
}

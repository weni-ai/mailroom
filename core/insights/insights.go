package insights

import (
	"os"
	"strconv"

	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
)

var (
	RunKey  string = "flowruns:wait"
	Enabled bool   = false
)

func init() {
	rk := os.Getenv("MAILROOM_INSIGHTS_RUNS_KEY")
	if rk != "" {
		RunKey = rk
	}
	en := os.Getenv("MAILROOM_INSIGHTS_ENABLED")
	if en != "" {
		enabled, _ := strconv.ParseBool(en)
		if enabled {
			Enabled = enabled
		}
	}

}

func PushRun(rc redis.Conn, run_uuid string) error {
	return PushData(rc, RunKey, run_uuid)
}

func PushData(rc redis.Conn, key string, data string) error {
	if !Enabled {
		return nil
	}
	logrus.Debugf("send data: %s to insights redis for key: %s", data, key)
	err := rc.Send("rpush", key, data)
	if err != nil {
		logrus.Errorf("error on push data to insights integration: %s", err)
	}
	return nil
}

package insights

import (
	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
)

const (
	RunKey string = "flowruns:wait"
)

func PushRun(rc redis.Conn, run_uuid string) error {
	return PushData(rc, RunKey, run_uuid)
}

func PushData(rc redis.Conn, key string, data string) error {
	logrus.Debugf("send data: %s to insights redis for key: %s", data, key)
	err := rc.Send("rpush", key, data)
	if err != nil {
		logrus.Errorf("error on push data to insights integration: %s", err)
	}
	return nil
}

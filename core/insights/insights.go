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
	logrus.Debugf("send data to insights redis for key %s with data: %s", key, data)
	return rc.Send("rpush", key, data)
}

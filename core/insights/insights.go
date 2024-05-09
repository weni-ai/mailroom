package insights

import (
	"github.com/gomodule/redigo/redis"
)

const (
	RunKey string = "flowruns:wait"
)

func PushRun(rc redis.Conn, run_uuid string) error {
	return PushData(rc, RunKey, run_uuid)
}

func PushData(rc redis.Conn, key string, data string) error {
	return rc.Send("rpush", key, data)
}

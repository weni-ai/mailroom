package insights

import (
	"encoding/json"
	"fmt"

	"github.com/gomodule/redigo/redis"
)

type DataType string

const (
	Prefix  string   = "insights"
	RunType DataType = "run"
)

func PushData(rc redis.Conn, dataType DataType, data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return rc.Send("rpush", fmt.Sprintf("%s:%s", Prefix, dataType), jsonData)
}

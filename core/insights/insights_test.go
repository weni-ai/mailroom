package insights

import (
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestInsights(t *testing.T) {
	Enabled = true
	rc, err := redis.Dial("tcp", "localhost:6379")
	assert.NoError(t, err)
	rc.Do("del", RunKey)

	tcs := []struct {
		Data string
		Err  error
	}{
		{
			"2c76f78a-f2ae-4f91-b803-4514e5f59a53",
			nil,
		},
		{
			"a3c5504d-3e05-4daa-b6e3-4bc7182aa5d5",
			nil,
		},
		{
			"374ed95b-d8d2-442b-b0ce-b950a223e082",
			nil,
		},
	}

	for _, tc := range tcs {
		err = PushData(rc, RunKey, tc.Data)
		assert.Equal(t, err, tc.Err)
	}
	rc.Flush()

	for _, tc := range tcs {
		receivedValue, err := rc.Do("lpop", RunKey)
		value, err := redis.String(receivedValue, err)
		assert.NoError(t, err)
		assert.NoError(t, err)
		assert.Equal(t, tc.Data, value)
	}
}

package insights

// func TestInsights(t *testing.T) {
// 	rc, err := redis.Dial("tcp", "localhost:6379")
// 	assert.NoError(t, err)
// 	rc.Do("del", "insights:run")

// 	tcs := []struct {
// 		QueueDataType DataType
// 		Data          map[string]interface{}
// 		Err           error
// 	}{
// 		{
// 			RunType,
// 			map[string]interface{}{
// 				"uuid":   "2c76f78a-f2ae-4f91-b803-4514e5f59a53",
// 				"status": "C",
// 			},
// 			nil,
// 		},
// 		{
// 			RunType,
// 			map[string]interface{}{
// 				"uuid":   "a3c5504d-3e05-4daa-b6e3-4bc7182aa5d5",
// 				"status": "E",
// 			},
// 			nil,
// 		},
// 		{
// 			RunType,
// 			map[string]interface{}{
// 				"uuid":   "374ed95b-d8d2-442b-b0ce-b950a223e082",
// 				"status": "I",
// 			},
// 			nil,
// 		},
// 	}

// 	for _, tc := range tcs {
// 		err = PushData(rc, tc.QueueDataType, tc.Data)
// 		assert.Equal(t, err, tc.Err)
// 	}
// 	rc.Flush()

// 	for _, tc := range tcs {
// 		data := map[string]interface{}{}
// 		receivedValue, err := rc.Do("lpop", fmt.Sprintf("%s:%s", Prefix, tc.QueueDataType))
// 		value, err := redis.String(receivedValue, err)
// 		assert.NoError(t, err)
// 		err = json.Unmarshal([]byte(value), &data)
// 		assert.NoError(t, err)
// 		assert.Equal(t, tc.Data["uuid"], data["uuid"])
// 	}
// }

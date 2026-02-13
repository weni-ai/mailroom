package models_test

import (
	"testing"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBulkQueryBatches(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`CREATE TABLE foo (id serial NOT NULL PRIMARY KEY, name TEXT, age INT)`)

	type foo struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
		Age  int    `db:"age"`
	}

	sql := `INSERT INTO foo (name, age) VALUES(:name, :age) RETURNING id`

	// noop with zero structs
	err := models.BulkQueryBatches(ctx, "foo inserts", db, sql, 10, nil)
	assert.NoError(t, err)

	// test when structs fit into one batch
	foo1 := &foo{Name: "A", Age: 30}
	foo2 := &foo{Name: "B", Age: 31}
	err = models.BulkQueryBatches(ctx, "foo inserts", db, sql, 2, []interface{}{foo1, foo2})
	assert.NoError(t, err)
	assert.Equal(t, 1, foo1.ID)
	assert.Equal(t, 2, foo2.ID)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'A' AND age = 30`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'B' AND age = 31`).Returns(1)

	// test when multiple batches are required
	foo3 := &foo{Name: "C", Age: 32}
	foo4 := &foo{Name: "D", Age: 33}
	foo5 := &foo{Name: "E", Age: 34}
	foo6 := &foo{Name: "F", Age: 35}
	foo7 := &foo{Name: "G", Age: 36}
	err = models.BulkQueryBatches(ctx, "foo inserts", db, sql, 2, []interface{}{foo3, foo4, foo5, foo6, foo7})
	assert.NoError(t, err)
	assert.Equal(t, 3, foo3.ID)
	assert.Equal(t, 4, foo4.ID)
	assert.Equal(t, 5, foo5.ID)
	assert.Equal(t, 6, foo6.ID)
	assert.Equal(t, 7, foo7.ID)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'C' AND age = 32`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'D' AND age = 33`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'E' AND age = 34`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'F' AND age = 35`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo WHERE name = 'G' AND age = 36`).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM foo `).Returns(7)
}

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		maps     []map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "no maps",
			maps:     []map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "single map",
			maps: []map[string]interface{}{
				{"a": 1, "b": "hello"},
			},
			expected: map[string]interface{}{"a": 1, "b": "hello"},
		},
		{
			name: "two maps no conflicts",
			maps: []map[string]interface{}{
				{"a": 1},
				{"b": 2},
			},
			expected: map[string]interface{}{"a": 1, "b": 2},
		},
		{
			name: "two maps with overwrite",
			maps: []map[string]interface{}{
				{"a": 1, "b": 2},
				{"b": 3, "c": 4},
			},
			expected: map[string]interface{}{"a": 1, "b": 3, "c": 4},
		},
		{
			name: "deep merge nested maps",
			maps: []map[string]interface{}{
				{"config": map[string]interface{}{"x": 1, "y": 2}},
				{"config": map[string]interface{}{"y": 3, "z": 4}},
			},
			expected: map[string]interface{}{
				"config": map[string]interface{}{"x": 1, "y": 3, "z": 4},
			},
		},
		{
			name: "nested map overwritten by non-map",
			maps: []map[string]interface{}{
				{"config": map[string]interface{}{"a": 1}},
				{"config": "replaced"},
			},
			expected: map[string]interface{}{"config": "replaced"},
		},
		{
			name: "non-map overwritten by nested map",
			maps: []map[string]interface{}{
				{"config": "original"},
				{"config": map[string]interface{}{"a": 1}},
			},
			expected: map[string]interface{}{"config": map[string]interface{}{"a": 1}},
		},
		{
			name: "multiple maps",
			maps: []map[string]interface{}{
				{"a": 1},
				{"b": 2},
				{"c": 3},
				{"a": 10},
			},
			expected: map[string]interface{}{"a": 10, "b": 2, "c": 3},
		},
		{
			name: "deeply nested merge",
			maps: []map[string]interface{}{
				{
					"level1": map[string]interface{}{
						"level2": map[string]interface{}{
							"a": 1,
						},
					},
				},
				{
					"level1": map[string]interface{}{
						"level2": map[string]interface{}{
							"b": 2,
						},
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"a": 1,
						"b": 2,
					},
				},
			},
		},
		{
			name: "nil values",
			maps: []map[string]interface{}{
				{"a": nil},
				{"b": 2},
			},
			expected: map[string]interface{}{"a": nil, "b": 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := models.MergeMaps(tc.maps...)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestMergeMaps_DoesNotModifyOriginal(t *testing.T) {
	original := map[string]interface{}{
		"a": 1,
		"nested": map[string]interface{}{
			"x": 10,
		},
	}

	other := map[string]interface{}{
		"b": 2,
		"nested": map[string]interface{}{
			"y": 20,
		},
	}

	result := models.MergeMaps(original, other)

	// result should have merged values
	assert.Equal(t, 1, result["a"])
	assert.Equal(t, 2, result["b"])
	nestedResult := result["nested"].(map[string]interface{})
	assert.Equal(t, 10, nestedResult["x"])
	assert.Equal(t, 20, nestedResult["y"])

	// original should remain unchanged
	assert.Equal(t, 1, original["a"])
	assert.Nil(t, original["b"])
	nestedOriginal := original["nested"].(map[string]interface{})
	assert.Equal(t, 10, nestedOriginal["x"])
	assert.Nil(t, nestedOriginal["y"])
}

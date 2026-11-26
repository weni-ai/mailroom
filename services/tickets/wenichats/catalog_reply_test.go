package wenichats

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogReplyBody(t *testing.T) {
	body, header, err := catalogReplyBody("  Check out our carousel  ", "Weekly highlights")
	require.NoError(t, err)
	assert.Equal(t, "Check out our carousel", body)
	assert.Equal(t, "Weekly highlights", header)

	body, header, err = catalogReplyBody("Only this ring", "   ")
	require.NoError(t, err)
	assert.Equal(t, "Only this ring", body)
	assert.Equal(t, "", header)

	body, header, err = catalogReplyBody("   ", "Gold")
	require.NoError(t, err)
	assert.Equal(t, "Gold", body)
	assert.Equal(t, "Gold", header)

	_, _, err = catalogReplyBody("   ", "  ")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "   ")
	assert.NotContains(t, err.Error(), "Gold")

	long := strings.Repeat("a", catalogBodyMaxRunes) + "é"
	body, header, err = catalogReplyBody(long, "Weekly highlights")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", catalogBodyMaxRunes), body)
	assert.NotContains(t, body, "…")
	assert.NotContains(t, body, "...")
	assert.Equal(t, "Weekly highlights", header)
	assert.Equal(t, catalogBodyMaxRunes, len([]rune(body)))
}

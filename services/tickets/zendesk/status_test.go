package zendesk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZendeskStatusIsClosed(t *testing.T) {
	assert.True(t, zendeskStatusIsClosed("solved"))
	assert.True(t, zendeskStatusIsClosed("closed"))
	assert.True(t, zendeskStatusIsClosed("SOLVED"))
	assert.True(t, zendeskStatusIsClosed("resuelto"))
	assert.True(t, zendeskStatusIsClosed("cerrado"))
	assert.True(t, zendeskStatusIsClosed("resolvido"))
	assert.False(t, zendeskStatusIsClosed("open"))
	assert.False(t, zendeskStatusIsClosed("new"))
	assert.False(t, zendeskStatusIsClosed("pending"))
	assert.False(t, zendeskStatusIsClosed("hold"))
	assert.False(t, zendeskStatusIsClosed(""))
}

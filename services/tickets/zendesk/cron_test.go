package zendesk

import (
	"context"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZendeskSyncInterval(t *testing.T) {
	assert.Equal(t, 12, runtime.NewDefaultConfig().ZendeskTicketSyncInterval)
	assert.Equal(t, 12*time.Hour, zendeskSyncInterval(&runtime.Runtime{Config: runtime.NewDefaultConfig()}))
	assert.Equal(t, 12*time.Hour, zendeskSyncInterval(&runtime.Runtime{Config: &runtime.Config{}}))
	assert.Equal(t, 6*time.Hour, zendeskSyncInterval(&runtime.Runtime{Config: &runtime.Config{ZendeskTicketSyncInterval: 6}}))
}

func TestSyncZendeskTicketsDisabled(t *testing.T) {
	rt := &runtime.Runtime{Config: runtime.NewDefaultConfig()}

	require.False(t, rt.Config.ZendeskTicketSync)
	assert.NoError(t, syncZendeskTickets(context.Background(), rt))
}

func TestSyncZendeskTicketsEnabled(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetData)

	rt.Config.ZendeskTicketSync = true

	assert.NoError(t, syncZendeskTickets(ctx, rt))
}

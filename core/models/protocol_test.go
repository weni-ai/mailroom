package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProtocolTimerMath(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	limit := 96 * time.Hour

	spent := ApplyPause(now, now.Add(86*time.Hour), limit)
	assert.Equal(t, 10*time.Hour, spent)

	resumed := ApplyResume(now.Add(48*time.Hour), spent, limit)
	assert.Equal(t, now.Add(48*time.Hour).Add(86*time.Hour), resumed)

	held := ApplyPause(resumed.Add(-85*time.Hour), resumed, limit)
	assert.Equal(t, 11*time.Hour, held)
}

func TestApplyPauseKeepsAProtocolOpenPastTheOriginalDeadline(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	limit := 96 * time.Hour
	spent := ApplyPause(now, now.Add(time.Hour), limit)
	later := now.Add(48 * time.Hour)
	deadline := ApplyResume(later, spent, limit)
	assert.True(t, deadline.After(later))
	assert.Equal(t, time.Hour, deadline.Sub(later))
}

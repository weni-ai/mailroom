package models

import (
	"testing"

	"github.com/nyaruka/goflow/excellent/types"
	"github.com/nyaruka/goflow/flows"
	"github.com/stretchr/testify/assert"
)

func TestIsMarketingTemplateCategory(t *testing.T) {
	assert.True(t, IsMarketingTemplateCategory("marketing"))
	assert.True(t, IsMarketingTemplateCategory("MARKETING"))
	assert.True(t, IsMarketingTemplateCategory(" Marketing "))
	assert.False(t, IsMarketingTemplateCategory("utility"))
	assert.False(t, IsMarketingTemplateCategory(""))
}

func TestContactAllowsMarketing(t *testing.T) {
	assert.True(t, ContactAllowsMarketing(nil))

	empty := &Contact{}
	assert.True(t, ContactAllowsMarketing(empty))

	withFalse := &Contact{
		fields: map[string]*flows.Value{
			MarketingOptInFieldKey: flows.NewValue(types.NewXText("false"), nil, nil, "", "", ""),
		},
	}
	assert.False(t, ContactAllowsMarketing(withFalse))

	withNo := &Contact{
		fields: map[string]*flows.Value{
			MarketingOptInFieldKey: flows.NewValue(types.NewXText("no"), nil, nil, "", "", ""),
		},
	}
	assert.False(t, ContactAllowsMarketing(withNo))

	withTrue := &Contact{
		fields: map[string]*flows.Value{
			MarketingOptInFieldKey: flows.NewValue(types.NewXText("true"), nil, nil, "", "", ""),
		},
	}
	assert.True(t, ContactAllowsMarketing(withTrue))
}

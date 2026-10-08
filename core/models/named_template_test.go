package models

import (
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateWhatsAppURNVariation(t *testing.T) {
	tcs := []struct {
		urn      urns.URN
		expected urns.URN
	}{
		{"whatsapp:5586981800114", "whatsapp:558681800114"},
		{"whatsapp:558681800114", "whatsapp:5586981800114"},
		{"whatsapp:12065551212", "whatsapp:12065551212"},
		{"tel:5586981800114", "tel:5586981800114"},
		{"whatsapp:5586981800114?channel=abc", "whatsapp:558681800114"},
	}

	for _, tc := range tcs {
		assert.Equal(t, tc.expected, generateWhatsAppURNVariation(tc.urn.Identity()), "variation for %s", tc.urn)
	}
}

func TestRecipientVariablesForURNNinthDigit(t *testing.T) {
	withNinth := "whatsapp:5586981800114"
	withoutNinth := "whatsapp:558681800114"
	vars := map[string]string{"nome": "Matheus", "pedido": "123456789", "horas": "3"}

	tcs := []struct {
		name     string
		urn      urns.URN
		storedAs string
	}{
		{"exact match with ninth digit", urns.URN(withNinth), withNinth},
		{"exact match without ninth digit", urns.URN(withoutNinth), withoutNinth},
		{"send without ninth, request keyed with ninth", urns.URN(withoutNinth), withNinth},
		{"send with ninth, request keyed without ninth", urns.URN(withNinth), withoutNinth},
		{"contact URN with channel query still matches", urns.URN(withoutNinth + "?id=42&channel=abc"), withNinth},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := recipientVariablesForURN(map[string]map[string]string{tc.storedAs: vars}, tc.urn)
			assert.Equal(t, vars, got)
		})
	}

	assert.Empty(t, recipientVariablesForURN(nil, urns.URN(withNinth)))
	assert.Empty(t, recipientVariablesForURN(map[string]map[string]string{withNinth: vars}, urns.NilURN))
	assert.Empty(t, recipientVariablesForURN(map[string]map[string]string{withNinth: vars}, urns.URN("whatsapp:5511999999999")))
}

func TestResolveNamedTemplateValuesNinthDigit(t *testing.T) {
	assetTT := static.NewTemplateTranslation(
		*assets.NewChannelReference("74729f45-7f29-4868-9dc4-90e491e3c7d8", "WhatsApp"),
		envs.Language("por"),
		envs.NilCountry,
		"Olá {{nome}}, seu pedido {{pedido}} será entregue em até {{horas}} horas.",
		3,
		"",
	)
	assetTT.SetParameterFormat(assets.ParameterFormatNamed)
	assetTT.SetParameterNames([]string{"nome", "pedido", "horas"})
	translation := flows.NewTemplateTranslation(assetTT)

	vars := map[string]string{"nome": "Matheus", "pedido": "123456789", "horas": "3"}
	tmpl := WppBroadcastTemplate{
		RecipientVariables: map[string]map[string]string{
			"whatsapp:5586981800114": vars,
		},
	}

	resolved, err := resolveNamedTemplateValues(nil, nil, urns.URN("whatsapp:558681800114"), tmpl, translation)
	require.NoError(t, err)
	assert.Equal(t, vars, resolved)

	resolved, err = resolveNamedTemplateValues(nil, nil, urns.URN("whatsapp:5586981800114"), tmpl, translation)
	require.NoError(t, err)
	assert.Equal(t, vars, resolved)

	resolved, err = resolveNamedTemplateValues(nil, nil, urns.URN("whatsapp:5511999999999"), tmpl, translation)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"nome":   namedTemplateFiller,
		"pedido": namedTemplateFiller,
		"horas":  namedTemplateFiller,
	}, resolved)
}

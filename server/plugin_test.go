package main

import (
	"context"
	"net/url"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
)

func TestParseBotDefinitions(t *testing.T) {
	bots, err := parseBotDefinitions(`[
		{"id":"ocr-bot","username":"ocr-bot","display_name":"OCR Bot","model":"document-parse","mode":"enhanced","ocr":"force","output_formats":["markdown","text"],"chart_recognition":true,"coordinates":true},
		{"id":"html-bot","username":"html-bot","display_name":"HTML Bot","output_formats":["html"]}
	]`)
	require.NoError(t, err)
	require.Len(t, bots, 2)
	require.Equal(t, "ocr-bot", bots[0].ID)
	require.Equal(t, "enhanced", bots[0].Mode)
	require.Equal(t, "force", bots[0].OCR)
	require.Equal(t, []string{"markdown", "text"}, bots[0].OutputFormats)
	require.True(t, bots[0].useChartRecognition())
	require.Equal(t, "html-bot", bots[1].Username)
	require.Equal(t, []string{"html"}, bots[1].OutputFormats)
}

func TestParseBotDefinitionsAutoAssignsIDFromUsername(t *testing.T) {
	bots, err := parseBotDefinitions(`[{"username":"summary-bot","display_name":"Thread Summary"}]`)
	require.NoError(t, err)
	require.Len(t, bots, 1)
	require.Equal(t, "summary-bot", bots[0].ID)
	require.Equal(t, defaultUpstageModel, bots[0].Model)
}

func TestParseBotDefinitionsPreservesEmptyBotAuthMode(t *testing.T) {
	bots, err := parseBotDefinitions(`[{"username":"summary-bot","display_name":"Thread Summary","auth_mode":""}]`)
	require.NoError(t, err)
	require.Len(t, bots, 1)
	require.Equal(t, "", bots[0].AuthMode)
}

func TestConfigurationGetStoredPluginConfigDefaultsWhenEmpty(t *testing.T) {
	cfg := &configuration{}
	stored, source, err := cfg.getStoredPluginConfig()
	require.NoError(t, err)
	require.Equal(t, "config", source)
	require.Equal(t, "https://api.upstage.ai/v1/document-digitization", stored.Service.BaseURL)
	require.Equal(t, "", stored.Service.AuthToken)
	require.Equal(t, defaultTimeoutSeconds, stored.Runtime.DefaultTimeoutSeconds)
	require.True(t, stored.Runtime.EnableUsageLogs)
	require.Empty(t, stored.Bots)
}

func TestConfigurationNormalizeFromConfig(t *testing.T) {
	cfg := &configuration{
		Config: `{
			"service": {
				"base_url": "https://api.upstage.ai",
				"auth_mode": "x-api-key",
				"auth_token": "secret"
			},
			"runtime": {
				"default_timeout_seconds": 55,
				"max_input_length": 5000,
				"max_output_length": 9000,
				"enable_debug_logs": true,
				"enable_usage_logs": false
			},
			"bots": [
				{"username":"summary-bot","display_name":"Thread Summary","model":"document-parse","output_formats":["markdown"]}
			]
		}`,
	}

	runtimeCfg, err := cfg.normalize()
	require.NoError(t, err)
	require.Equal(t, "https://api.upstage.ai/v1/document-digitization", runtimeCfg.ServiceBaseURL)
	require.Equal(t, "x-api-key", runtimeCfg.AuthMode)
	require.Equal(t, "secret", runtimeCfg.AuthToken)
	require.Equal(t, 55, int(runtimeCfg.DefaultTimeout.Seconds()))
	require.False(t, runtimeCfg.EnableUsageLogs)
	require.Len(t, runtimeCfg.BotDefinitions, 1)
	require.Equal(t, "summary-bot", runtimeCfg.BotDefinitions[0].ID)
}

func TestNormalizeUpstageEndpointURL(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "default api root",
			input:    "https://api.upstage.ai",
			expected: "https://api.upstage.ai/v1/document-digitization",
		},
		{
			name:     "api v1 root",
			input:    "https://api.upstage.ai/v1",
			expected: "https://api.upstage.ai/v1/document-digitization",
		},
		{
			name:     "existing endpoint",
			input:    "https://api.upstage.ai/v1/document-digitization",
			expected: "https://api.upstage.ai/v1/document-digitization",
		},
		{
			name:     "console docs url",
			input:    "https://console.upstage.ai/api/parse/document-parsing",
			expected: "https://api.upstage.ai/v1/document-digitization",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, _, err := normalizeUpstageEndpointURL(testCase.input)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, normalized)
		})
	}
}

func TestChoosePreferredUpstageContent(t *testing.T) {
	format, content := choosePreferredUpstageContent(upstageParseResponse{
		Content: upstageParseContent{
			HTML:     "<h1>Invoice</h1>",
			Markdown: "# Invoice",
			Text:     "Invoice",
		},
	}, []string{"markdown", "text"})

	require.Equal(t, "markdown", format)
	require.Equal(t, "# Invoice", content)
}

func TestBuildUpstageFormFieldsUsesMinimalDefaults(t *testing.T) {
	bot, err := (BotDefinition{Username: "parser-bot"}).normalize()
	require.NoError(t, err)

	fields, err := buildUpstageFormFields(bot)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"model":          defaultUpstageModel,
		"output_formats": `["markdown","text"]`,
	}, fields)
}

func TestBuildUpstageFormFieldsIncludesOptionalOverrides(t *testing.T) {
	bot, err := (BotDefinition{
		Username:             "parser-bot",
		Model:                "document-parse",
		Mode:                 "enhanced",
		OCR:                  "force",
		Coordinates:          boolPtr(false),
		ChartRecognition:     boolPtr(false),
		MergeMultipageTables: boolPtr(true),
		OutputFormats:        []string{"html", "text"},
		Base64Encoding:       []string{"table"},
	}).normalize()
	require.NoError(t, err)

	fields, err := buildUpstageFormFields(bot)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"model":                  "document-parse",
		"mode":                   "enhanced",
		"ocr":                    "force",
		"coordinates":            "false",
		"chart_recognition":      "false",
		"merge_multipage_tables": "true",
		"output_formats":         `["html","text"]`,
		"base64_encoding":        `["table"]`,
	}, fields)
}

func TestExtractTextFromBody(t *testing.T) {
	body := []byte(`{"error":{"message":"invalid api key"}}`)
	require.Equal(t, "invalid api key", extractTextFromBody(body))
}

func TestServiceConfigForBotPrefersBotOverrides(t *testing.T) {
	parsedURL, err := url.Parse("https://api.upstage.ai/v1/document-digitization")
	require.NoError(t, err)

	cfg := &runtimeConfiguration{
		ServiceBaseURL: "https://api.upstage.ai/v1/document-digitization",
		ParsedBaseURL:  parsedURL,
		AuthMode:       "bearer",
		AuthToken:      "secret",
		AllowHosts:     []string{"api.upstage.ai"},
	}

	service, err := cfg.serviceConfigForBot(BotDefinition{
		BaseURL:   "https://api.upstage.ai/v1",
		AuthMode:  "x-api-key",
		AuthToken: "override",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.upstage.ai/v1/document-digitization", service.BaseURL)
	require.Equal(t, "x-api-key", service.AuthMode)
	require.Equal(t, "override", service.AuthToken)
}

func TestClassifyUpstageHTTPErrorUnauthorized(t *testing.T) {
	err := classifyUpstageHTTPError(
		"https://api.upstage.ai/v1/document-digitization",
		401,
		nil,
		[]byte(`{"detail":"invalid token"}`),
	)

	require.Equal(t, "auth_failed", err.Code)
	require.Equal(t, "Upstage 인증에 실패했습니다.", err.Summary)
	require.Contains(t, err.Detail, "invalid token")
	require.False(t, err.Retryable)
}

func TestClassifyUpstageRequestErrorTimeout(t *testing.T) {
	err := classifyUpstageRequestError(
		"https://api.upstage.ai/v1/document-digitization",
		context.DeadlineExceeded,
	)

	require.Equal(t, "network_timeout", err.Code)
	require.True(t, err.Retryable)
	require.Contains(t, err.Error(), "시간 초과")
}

func TestExtractPromptFromMessageTriggersFileOnlyDirectMessages(t *testing.T) {
	bot, err := (BotDefinition{Username: "parser-bot", DisplayName: "Parser Bot"}).normalize()
	require.NoError(t, err)

	plugin := &Plugin{}
	plugin.setBotAccounts(map[string]botAccount{
		bot.ID: {
			Definition: bot,
			UserID:     "bot-user-id",
		},
	})

	cfg := &runtimeConfiguration{BotDefinitions: []BotDefinition{bot}}
	channel := &model.Channel{
		Type: model.ChannelTypeDirect,
		Name: "bot-user-id__human-user-id",
	}

	triggeredBot, prompt, triggered := plugin.extractPromptFromMessage(cfg, channel, "")
	require.True(t, triggered)
	require.NotNil(t, triggeredBot)
	require.Equal(t, bot.ID, triggeredBot.ID)
	require.Empty(t, prompt)
}

func TestExtractPromptFromMessageIgnoresEmptyNonDirectMessages(t *testing.T) {
	bot, err := (BotDefinition{Username: "parser-bot", DisplayName: "Parser Bot"}).normalize()
	require.NoError(t, err)

	plugin := &Plugin{}
	cfg := &runtimeConfiguration{BotDefinitions: []BotDefinition{bot}}
	channel := &model.Channel{
		Type: model.ChannelTypeOpen,
		Name: "town-square",
	}

	triggeredBot, prompt, triggered := plugin.extractPromptFromMessage(cfg, channel, "")
	require.False(t, triggered)
	require.Nil(t, triggeredBot)
	require.Empty(t, prompt)
}

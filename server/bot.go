package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

const defaultUpstageModel = "document-parse"

var defaultBotOutputFormats = []string{"markdown", "text"}

type BotDefinition struct {
	ID                   string          `json:"id"`
	Username             string          `json:"username"`
	DisplayName          string          `json:"display_name"`
	Description          string          `json:"description"`
	BaseURL              string          `json:"base_url,omitempty"`
	AuthMode             string          `json:"auth_mode,omitempty"`
	AuthToken            string          `json:"auth_token,omitempty"`
	Model                string          `json:"model,omitempty"`
	Mode                 string          `json:"mode,omitempty"`
	OCR                  string          `json:"ocr,omitempty"`
	OutputFormats        []string        `json:"output_formats,omitempty"`
	Coordinates          *bool           `json:"coordinates,omitempty"`
	ChartRecognition     *bool           `json:"chart_recognition,omitempty"`
	MergeMultipageTables *bool           `json:"merge_multipage_tables,omitempty"`
	Base64Encoding       []string        `json:"base64_encoding,omitempty"`
	MaskSensitiveData    *bool           `json:"mask_sensitive_data,omitempty"`
	VLLMBaseURL          string          `json:"vllm_base_url,omitempty"`
	VLLMAPIKey           string          `json:"vllm_api_key,omitempty"`
	VLLMModel            string          `json:"vllm_model,omitempty"`
	VLLMPrompt           string          `json:"vllm_prompt,omitempty"`
	FlowID               string          `json:"flow_id,omitempty"`
	FileComponentID      string          `json:"file_component_id,omitempty"`
	ImageComponentID     string          `json:"image_component_id,omitempty"`
	AllowedTeams         []string        `json:"allowed_teams"`
	AllowedChannels      []string        `json:"allowed_channels"`
	AllowedUsers         []string        `json:"allowed_users"`
	InputSchema          []BotInputField `json:"input_schema,omitempty"`
}

type BotInputField struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	Placeholder  string `json:"placeholder"`
	DefaultValue any    `json:"default_value"`
}

func (b BotDefinition) normalize() (BotDefinition, error) {
	b.ID = strings.TrimSpace(b.ID)
	b.Username = strings.ToLower(strings.TrimSpace(b.Username))
	b.DisplayName = strings.TrimSpace(b.DisplayName)
	b.Description = strings.TrimSpace(b.Description)
	b.BaseURL = strings.TrimSpace(b.BaseURL)
	b.AuthMode = strings.ToLower(strings.TrimSpace(b.AuthMode))
	if b.AuthMode != "" {
		b.AuthMode = normalizeAuthMode(b.AuthMode)
	}
	b.AuthToken = strings.TrimSpace(b.AuthToken)
	b.Model = defaultIfEmpty(strings.TrimSpace(b.Model), defaultUpstageModel)
	b.Mode = normalizeUpstageMode(b.Mode)
	b.OCR = normalizeUpstageOCRMode(b.OCR)
	b.OutputFormats = normalizeOutputFormats(b.OutputFormats)
	b.Base64Encoding = normalizeStringSlice(b.Base64Encoding)
	b.VLLMBaseURL = strings.TrimSpace(b.VLLMBaseURL)
	b.VLLMAPIKey = strings.TrimSpace(b.VLLMAPIKey)
	b.VLLMModel = strings.TrimSpace(b.VLLMModel)
	b.VLLMPrompt = strings.TrimSpace(b.VLLMPrompt)
	b.FlowID = strings.TrimSpace(b.FlowID)
	b.FileComponentID = strings.TrimSpace(b.FileComponentID)
	b.ImageComponentID = strings.TrimSpace(b.ImageComponentID)

	if b.Username == "" {
		return BotDefinition{}, fmt.Errorf("bot definition is missing username")
	}
	if b.ID == "" {
		b.ID = b.Username
	}
	if b.DisplayName == "" {
		b.DisplayName = b.Username
	}
	if b.Coordinates == nil {
		b.Coordinates = boolPtr(true)
	}
	if b.ChartRecognition == nil {
		b.ChartRecognition = boolPtr(true)
	}
	if b.MergeMultipageTables == nil {
		b.MergeMultipageTables = boolPtr(false)
	}

	b.AllowedTeams = normalizeStringSlice(b.AllowedTeams)
	b.AllowedChannels = normalizeStringSlice(b.AllowedChannels)
	b.AllowedUsers = normalizeStringSlice(b.AllowedUsers)

	inputs := make([]BotInputField, 0, len(b.InputSchema))
	seen := map[string]struct{}{}
	for _, field := range b.InputSchema {
		field.Name = strings.TrimSpace(field.Name)
		field.Label = defaultIfEmpty(strings.TrimSpace(field.Label), field.Name)
		field.Description = strings.TrimSpace(field.Description)
		field.Placeholder = strings.TrimSpace(field.Placeholder)
		field.Type = defaultIfEmpty(strings.ToLower(strings.TrimSpace(field.Type)), "text")
		if field.Name == "" {
			return BotDefinition{}, fmt.Errorf("bot %q has an input field without a name", b.Username)
		}
		if _, ok := seen[field.Name]; ok {
			return BotDefinition{}, fmt.Errorf("bot %q defines duplicate input %q", b.Username, field.Name)
		}
		seen[field.Name] = struct{}{}
		inputs = append(inputs, field)
	}
	b.InputSchema = inputs

	return b, nil
}

func (b BotDefinition) useCoordinates() bool {
	return b.Coordinates != nil && *b.Coordinates
}

func (b BotDefinition) useChartRecognition() bool {
	return b.ChartRecognition == nil || *b.ChartRecognition
}

func (b BotDefinition) useMergeMultipageTables() bool {
	return b.MergeMultipageTables != nil && *b.MergeMultipageTables
}

func (b BotDefinition) shouldMaskSensitiveData(defaultValue bool) bool {
	if b.MaskSensitiveData != nil {
		return *b.MaskSensitiveData
	}
	return defaultValue
}

func (b BotDefinition) hasVLLMPostProcess() bool {
	return b.VLLMBaseURL != "" && b.VLLMModel != ""
}

func (b BotDefinition) publicView() BotDefinition {
	copyBot := b
	copyBot.AuthToken = ""
	copyBot.VLLMAPIKey = ""
	return copyBot
}

func normalizeStringSlice(items []string) []string {
	normalized := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func normalizeOutputFormats(values []string) []string {
	allowed := map[string]struct{}{
		"html":     {},
		"markdown": {},
		"text":     {},
	}
	if len(values) == 0 {
		return []string{}
	}

	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{}
	}
	return normalized
}

func normalizeUpstageMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "standard":
		return "standard"
	case "enhanced":
		return "enhanced"
	case "auto":
		return "auto"
	default:
		return "standard"
	}
}

func normalizeUpstageOCRMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "force":
		return "force"
	default:
		return "auto"
	}
}

func boolPtr(value bool) *bool {
	item := value
	return &item
}

func (cfg *runtimeConfiguration) getBotByID(botID string) *BotDefinition {
	botID = strings.ToLower(strings.TrimSpace(botID))
	for _, bot := range cfg.BotDefinitions {
		if strings.ToLower(bot.ID) == botID || strings.ToLower(bot.Username) == botID {
			item := bot
			return &item
		}
	}
	return nil
}

func (cfg *runtimeConfiguration) getAllowedBots(user *model.User, channel *model.Channel, team *model.Team) []BotDefinition {
	allowed := make([]BotDefinition, 0, len(cfg.BotDefinitions))
	for _, bot := range cfg.BotDefinitions {
		if bot.isAllowedFor(user, channel, team) {
			allowed = append(allowed, bot)
		}
	}
	return allowed
}

func (b BotDefinition) isAllowedFor(user *model.User, channel *model.Channel, team *model.Team) bool {
	if user == nil || channel == nil {
		return false
	}

	if len(b.AllowedUsers) > 0 && !matchesAccessEntry(b.AllowedUsers, user.Id, user.Username) {
		return false
	}
	if len(b.AllowedChannels) > 0 && !matchesAccessEntry(b.AllowedChannels, channel.Id, channel.Name) {
		return false
	}

	teamName := ""
	if team != nil {
		teamName = team.Name
	}
	if len(b.AllowedTeams) > 0 && !matchesAccessEntry(b.AllowedTeams, channel.TeamId, teamName) {
		return false
	}

	return true
}

func matchesAccessEntry(entries []string, values ...string) bool {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		for _, entry := range entries {
			if entry == value {
				return true
			}
		}
	}
	return false
}

func botUsageExamples(bot BotDefinition) []string {
	return []string{
		fmt.Sprintf("- `@%s 계약서를 파싱해줘` 와 함께 파일 첨부", bot.Username),
		fmt.Sprintf("- `@%s markdown으로 변환해줘` 와 함께 PDF 업로드", bot.Username),
		fmt.Sprintf("- DM `%s` 에 문서 파일을 붙여 전송", bot.Username),
	}
}

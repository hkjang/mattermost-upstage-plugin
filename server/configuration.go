package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	defaultAuthMode           = "bearer"
	defaultTimeoutSeconds     = 30
	defaultMaxInputLength     = 4000
	defaultMaxOutputLength    = 8000
	defaultUpstageEndpointURL = "https://api.upstage.ai/v1/document-digitization"
	maxHistoryEntriesPerUser  = 20
)

type configuration struct {
	Config string `json:"Config"`
}

type storedPluginConfig struct {
	Service storedServiceConfig `json:"service"`
	Runtime storedRuntimeConfig `json:"runtime"`
	Bots    []BotDefinition     `json:"bots"`
}

type storedServiceConfig struct {
	BaseURL    string `json:"base_url"`
	AuthMode   string `json:"auth_mode"`
	AuthToken  string `json:"auth_token"`
	AllowHosts string `json:"allow_hosts"`
}

type storedRuntimeConfig struct {
	DefaultTimeoutSeconds int  `json:"default_timeout_seconds"`
	EnableStreaming       bool `json:"enable_streaming"`
	StreamingUpdateMS     int  `json:"streaming_update_ms"`
	MaxInputLength        int  `json:"max_input_length"`
	MaxOutputLength       int  `json:"max_output_length"`
	ContextPostLimit      int  `json:"context_post_limit"`
	MaskSensitiveData     bool `json:"mask_sensitive_data"`
	EnableDebugLogs       bool `json:"enable_debug_logs"`
	EnableUsageLogs       bool `json:"enable_usage_logs"`
}

type runtimeConfiguration struct {
	ServiceBaseURL    string
	ParsedBaseURL     *url.URL
	AuthMode          string
	AuthToken         string
	AllowHosts        []string
	BotDefinitions    []BotDefinition
	DefaultTimeout    time.Duration
	MaxInputLength    int
	MaxOutputLength   int
	MaskSensitiveData bool
	EnableDebugLogs   bool
	EnableUsageLogs   bool
}

func (c *configuration) Clone() *configuration {
	clone := *c
	return &clone
}

func (c *configuration) normalize() (*runtimeConfiguration, error) {
	stored, _, err := c.getStoredPluginConfig()
	if err != nil {
		return nil, err
	}
	return stored.normalize()
}

func (c *configuration) getStoredPluginConfig() (storedPluginConfig, string, error) {
	stored, err := parseStoredPluginConfig(c.Config)
	if err != nil {
		return storedPluginConfig{}, "config", err
	}
	return stored, "config", nil
}

func parseStoredPluginConfig(raw string) (storedPluginConfig, error) {
	cfg := defaultStoredPluginConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return storedPluginConfig{}, fmt.Errorf("invalid Config JSON: %w", err)
	}
	return cfg, nil
}

func defaultStoredPluginConfig() storedPluginConfig {
	return storedPluginConfig{
		Service: storedServiceConfig{
			BaseURL:  defaultUpstageEndpointURL,
			AuthMode: defaultAuthMode,
		},
		Runtime: storedRuntimeConfig{
			DefaultTimeoutSeconds: defaultTimeoutSeconds,
			MaxInputLength:        defaultMaxInputLength,
			MaxOutputLength:       defaultMaxOutputLength,
			EnableUsageLogs:       true,
		},
		Bots: []BotDefinition{},
	}
}

func (c storedPluginConfig) normalize() (*runtimeConfiguration, error) {
	cfg := &runtimeConfiguration{
		AuthMode:          normalizeAuthMode(c.Service.AuthMode),
		AuthToken:         strings.TrimSpace(c.Service.AuthToken),
		MaxInputLength:    positiveOrDefault(c.Runtime.MaxInputLength, defaultMaxInputLength),
		MaxOutputLength:   positiveOrDefault(c.Runtime.MaxOutputLength, defaultMaxOutputLength),
		MaskSensitiveData: c.Runtime.MaskSensitiveData,
		EnableDebugLogs:   c.Runtime.EnableDebugLogs,
		EnableUsageLogs:   c.Runtime.EnableUsageLogs,
	}
	cfg.DefaultTimeout = time.Duration(positiveOrDefault(c.Runtime.DefaultTimeoutSeconds, defaultTimeoutSeconds)) * time.Second

	normalizedURL, parsedURL, err := normalizeUpstageEndpointURL(strings.TrimSpace(c.Service.BaseURL))
	if err != nil {
		return nil, err
	}
	cfg.ServiceBaseURL = normalizedURL
	cfg.ParsedBaseURL = parsedURL

	serializedBots, err := json.Marshal(c.Bots)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize bot definitions: %w", err)
	}

	bots, err := parseBotDefinitions(string(serializedBots))
	if err != nil {
		return nil, err
	}
	cfg.BotDefinitions = bots
	cfg.AllowHosts = normalizeAllowHosts(c.Service.AllowHosts, cfg.ParsedBaseURL)

	return cfg, nil
}

func normalizeAuthMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "bearer":
		return defaultAuthMode
	case "x-api-key":
		return "x-api-key"
	default:
		return defaultAuthMode
	}
}

func normalizeAllowHosts(raw string, parsedBaseURL *url.URL) []string {
	parts := strings.Split(raw, ",")
	hosts := make([]string, 0, len(parts)+1)
	seen := map[string]struct{}{}

	appendHost := func(host string) {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}

	for _, part := range parts {
		appendHost(part)
	}

	if len(hosts) == 0 && parsedBaseURL != nil {
		appendHost(parsedBaseURL.Hostname())
	}

	return hosts
}

func positiveOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseBotDefinitions(raw string) ([]BotDefinition, error) {
	if strings.TrimSpace(raw) == "" {
		return []BotDefinition{}, nil
	}

	var bots []BotDefinition
	if err := json.Unmarshal([]byte(raw), &bots); err != nil {
		return nil, fmt.Errorf("invalid bot definitions JSON: %w", err)
	}

	normalized := make([]BotDefinition, 0, len(bots))
	seenIDs := map[string]struct{}{}
	seenUsernames := map[string]struct{}{}
	for _, bot := range bots {
		item, err := bot.normalize()
		if err != nil {
			return nil, err
		}
		if _, ok := seenIDs[item.ID]; ok {
			return nil, fmt.Errorf("duplicate bot id %q", item.ID)
		}
		if _, ok := seenUsernames[item.Username]; ok {
			return nil, fmt.Errorf("duplicate bot username %q", item.Username)
		}
		seenIDs[item.ID] = struct{}{}
		seenUsernames[item.Username] = struct{}{}
		normalized = append(normalized, item)
	}

	return normalized, nil
}

func hostAllowed(host string, allowHosts []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if len(allowHosts) == 0 {
		return true
	}
	for _, pattern := range allowHosts {
		if pattern == host {
			return true
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(host, suffix) {
				return true
			}
		}
	}
	return false
}

func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

func (p *Plugin) getRuntimeConfiguration() (*runtimeConfiguration, error) {
	return p.getConfiguration().normalize()
}

func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}
		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

func (p *Plugin) OnConfigurationChange() error {
	configuration := new(configuration)
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}

	p.setConfiguration(configuration)

	if p.client != nil {
		if err := p.ensureBots(); err != nil {
			p.API.LogError("Failed to ensure Upstage parser bots after configuration change", "error", err)
		}
	}

	return nil
}

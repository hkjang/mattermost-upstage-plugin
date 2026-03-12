package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type Plugin struct {
	plugin.MattermostPlugin

	client *pluginapi.Client
	router *mux.Router

	configurationLock sync.RWMutex
	configuration     *configuration

	botAccountsLock sync.RWMutex
	botAccounts     map[string]botAccount

	botSyncStateLock sync.RWMutex
	botSyncState     botSyncState
}

type botAccount struct {
	Definition BotDefinition
	UserID     string
}

type botSyncEntry struct {
	BotID         string `json:"bot_id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Model         string `json:"model"`
	UserID        string `json:"user_id,omitempty"`
	Registered    bool   `json:"registered"`
	Active        bool   `json:"active"`
	StatusMessage string `json:"status_message,omitempty"`
}

type botSyncState struct {
	LastError string         `json:"last_error,omitempty"`
	UpdatedAt int64          `json:"updated_at"`
	Entries   []botSyncEntry `json:"entries"`
}

func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	if err := p.OnConfigurationChange(); err != nil {
		return err
	}

	p.router = p.initRouter()

	if err := p.ensureBots(); err != nil {
		p.API.LogError("Failed to ensure Upstage parser bots during activation", "error", err)
	}

	return nil
}

func (p *Plugin) OnDeactivate() error {
	return nil
}

func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *model.Post) {
	if post == nil || post.UserId == "" || p.isManagedBotUserID(post.UserId) {
		return
	}
	if post.GetProp("from_bot") != nil || post.GetProp("from_plugin") != nil || post.GetProp("from_webhook") != nil {
		return
	}
	if post.RemoteId != nil && *post.RemoteId != "" {
		return
	}

	go func() {
		if err := p.handlePostedMessage(post); err != nil {
			p.API.LogError("Failed to process Upstage parser post trigger", "error", err, "post_id", post.Id)
		}
	}()
}

func (p *Plugin) handlePostedMessage(post *model.Post) error {
	cfg, err := p.getRuntimeConfiguration()
	if err != nil {
		return err
	}
	if len(cfg.BotDefinitions) == 0 {
		return nil
	}

	channel, appErr := p.API.GetChannel(post.ChannelId)
	if appErr != nil {
		return fmt.Errorf("failed to get channel: %w", appErr)
	}
	team := p.getTeamForChannel(channel)

	postingUser, appErr := p.API.GetUser(post.UserId)
	if appErr != nil {
		return fmt.Errorf("failed to get posting user: %w", appErr)
	}
	if postingUser.IsBot {
		return nil
	}

	bot, prompt, triggered := p.extractPromptFromMessage(cfg, channel, post.Message)
	if !triggered {
		return nil
	}

	account, ok := p.getBotAccount(bot.ID)
	if !ok {
		if err := p.ensureBots(); err != nil {
			return err
		}
		account, ok = p.getBotAccount(bot.ID)
		if !ok {
			return fmt.Errorf("bot account %q is not available", bot.ID)
		}
	}

	if !bot.isAllowedFor(postingUser, channel, team) {
		return p.postInstruction(channel, responseRootID(post), account, fmt.Sprintf("`@%s` is not available in this conversation.", bot.Username))
	}

	if len(post.FileIds) == 0 {
		return p.postInstruction(channel, responseRootID(post), account, buildBotPromptMessage(*bot))
	}

	request := BotRunRequest{
		BotID:         bot.ID,
		UserID:        postingUser.Id,
		ChannelID:     channel.Id,
		RootID:        responseRootID(post),
		Prompt:        prompt,
		FileIDs:       append([]string{}, post.FileIds...),
		Source:        "message",
		TriggerPostID: post.Id,
		Inputs:        map[string]any{},
	}

	_, runErr := p.executeBotAndPost(context.Background(), request)
	return runErr
}

func (p *Plugin) extractPromptFromMessage(cfg *runtimeConfiguration, channel *model.Channel, message string) (*BotDefinition, string, bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, "", false
	}

	type mentionMatch struct {
		Bot   BotDefinition
		Index int
	}

	matches := make([]mentionMatch, 0, len(cfg.BotDefinitions))
	lowerMessage := strings.ToLower(message)
	for _, bot := range cfg.BotDefinitions {
		mention := "@" + bot.Username
		if index := strings.Index(lowerMessage, mention); index >= 0 {
			matches = append(matches, mentionMatch{Bot: bot, Index: index})
		}
	}

	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].Index < matches[j].Index
		})
		match := matches[0]
		mentionLength := len(match.Bot.Username) + 1
		prompt := strings.TrimSpace(strings.TrimSpace(message[:match.Index]) + " " + strings.TrimSpace(message[match.Index+mentionLength:]))
		return &match.Bot, prompt, true
	}

	if channel != nil && channel.Type == model.ChannelTypeDirect {
		for _, memberID := range strings.Split(channel.Name, "__") {
			account, ok := p.getBotAccountByUserID(memberID)
			if ok {
				bot := account.Definition
				return &bot, message, true
			}
		}
	}

	return nil, "", false
}

func (p *Plugin) getTeamForChannel(channel *model.Channel) *model.Team {
	if channel == nil || channel.TeamId == "" {
		return nil
	}
	team, appErr := p.API.GetTeam(channel.TeamId)
	if appErr != nil {
		return nil
	}
	return team
}

func buildBotPromptMessage(bot BotDefinition) string {
	lines := []string{
		fmt.Sprintf("`@%s` 는 Upstage Document Parsing `%s` 를 사용합니다.", bot.Username, bot.Model),
		"",
		"문서 파일을 같이 첨부한 뒤 메시지를 보내 주세요.",
		"",
		"예시:",
	}
	lines = append(lines, botUsageExamples(bot)...)
	if bot.Description != "" {
		lines = append(lines, "", bot.Description)
	}
	return strings.Join(lines, "\n")
}

func (p *Plugin) setBotAccounts(accounts map[string]botAccount) {
	p.botAccountsLock.Lock()
	defer p.botAccountsLock.Unlock()
	p.botAccounts = accounts
}

func (p *Plugin) getBotAccount(botID string) (botAccount, bool) {
	p.botAccountsLock.RLock()
	defer p.botAccountsLock.RUnlock()

	if p.botAccounts == nil {
		return botAccount{}, false
	}

	account, ok := p.botAccounts[botID]
	return account, ok
}

func (p *Plugin) getBotAccountByUserID(userID string) (botAccount, bool) {
	p.botAccountsLock.RLock()
	defer p.botAccountsLock.RUnlock()

	for _, account := range p.botAccounts {
		if account.UserID == userID {
			return account, true
		}
	}
	return botAccount{}, false
}

func (p *Plugin) isManagedBotUserID(userID string) bool {
	if userID == "" {
		return false
	}
	_, ok := p.getBotAccountByUserID(userID)
	return ok
}

func (p *Plugin) listBotAccounts() []botAccount {
	p.botAccountsLock.RLock()
	defer p.botAccountsLock.RUnlock()

	accounts := make([]botAccount, 0, len(p.botAccounts))
	for _, account := range p.botAccounts {
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Definition.Username < accounts[j].Definition.Username
	})
	return accounts
}

func (p *Plugin) setBotSyncState(state botSyncState) {
	p.botSyncStateLock.Lock()
	defer p.botSyncStateLock.Unlock()
	p.botSyncState = state
}

func (p *Plugin) getBotSyncState() botSyncState {
	p.botSyncStateLock.RLock()
	defer p.botSyncStateLock.RUnlock()
	return p.botSyncState
}

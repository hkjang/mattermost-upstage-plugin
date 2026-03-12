package main

import (
	"encoding/json"
	"fmt"
	"time"
)

const historyKeyPrefix = "history_"

type ExecutionRecord struct {
	CorrelationID string `json:"correlation_id"`
	BotID         string `json:"bot_id"`
	BotUsername   string `json:"bot_username"`
	BotName       string `json:"bot_name"`
	Model         string `json:"model"`
	Source        string `json:"source"`
	ChannelID     string `json:"channel_id"`
	RootID        string `json:"root_id"`
	Status        string `json:"status"`
	PromptPreview string `json:"prompt_preview"`
	ErrorMessage  string `json:"error_message,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	Retryable     bool   `json:"retryable"`
	DurationMS    int64  `json:"duration_ms"`
	StartedAt     int64  `json:"started_at"`
	CompletedAt   int64  `json:"completed_at"`
}

func (p *Plugin) appendExecutionHistory(userID string, record ExecutionRecord) {
	if userID == "" {
		return
	}

	history, err := p.getExecutionHistory(userID, maxHistoryEntriesPerUser)
	if err != nil {
		p.API.LogError("Failed to load execution history", "error", err, "user_id", userID)
		history = []ExecutionRecord{}
	}

	history = append([]ExecutionRecord{record}, history...)
	if len(history) > maxHistoryEntriesPerUser {
		history = history[:maxHistoryEntriesPerUser]
	}

	data, err := json.Marshal(history)
	if err != nil {
		p.API.LogError("Failed to encode execution history", "error", err, "user_id", userID)
		return
	}

	if appErr := p.API.KVSet(historyKeyPrefix+userID, data); appErr != nil {
		p.API.LogError("Failed to persist execution history", "error", appErr, "user_id", userID)
	}
}

func (p *Plugin) getExecutionHistory(userID string, limit int) ([]ExecutionRecord, error) {
	if userID == "" {
		return []ExecutionRecord{}, nil
	}

	data, appErr := p.API.KVGet(historyKeyPrefix + userID)
	if appErr != nil {
		return nil, fmt.Errorf("failed to get execution history: %w", appErr)
	}
	if len(data) == 0 {
		return []ExecutionRecord{}, nil
	}

	var history []ExecutionRecord
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("failed to decode execution history: %w", err)
	}
	if limit > 0 && len(history) > limit {
		return history[:limit], nil
	}
	return history, nil
}

func newExecutionRecord(request BotRunRequest, bot BotDefinition, correlationID, status, prompt, errorMessage, errorCode string, retryable bool, startedAt time.Time, completedAt time.Time) ExecutionRecord {
	return ExecutionRecord{
		CorrelationID: correlationID,
		BotID:         bot.ID,
		BotUsername:   bot.Username,
		BotName:       bot.DisplayName,
		Model:         bot.Model,
		Source:        request.Source,
		ChannelID:     request.ChannelID,
		RootID:        request.RootID,
		Status:        status,
		PromptPreview: truncateString(prompt, 180),
		ErrorMessage:  errorMessage,
		ErrorCode:     errorCode,
		Retryable:     retryable,
		DurationMS:    completedAt.Sub(startedAt).Milliseconds(),
		StartedAt:     startedAt.UnixMilli(),
		CompletedAt:   completedAt.UnixMilli(),
	}
}

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

type runBotAPIRequest struct {
	BotID     string         `json:"bot_id"`
	ChannelID string         `json:"channel_id"`
	RootID    string         `json:"root_id"`
	Prompt    string         `json:"prompt"`
	Inputs    map[string]any `json:"inputs"`
	FileIDs   []string       `json:"file_ids,omitempty"`
}

type pluginStatusResponse struct {
	PluginID    string                   `json:"plugin_id"`
	BaseURL     string                   `json:"base_url"`
	BotCount    int                      `json:"bot_count"`
	AllowHosts  []string                 `json:"allow_hosts"`
	Bots        []BotDefinition          `json:"bots"`
	ManagedBots []botSyncEntry           `json:"managed_bots"`
	BotSync     botSyncState             `json:"bot_sync"`
	ConfigError string                   `json:"config_error,omitempty"`
	Connection  *upstageConnectionStatus `json:"connection,omitempty"`
}

type adminConfigResponse struct {
	Config storedPluginConfig `json:"config"`
	Source string             `json:"source"`
}

func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()
	router.Use(p.MattermostAuthorizationRequired)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/config", p.handleAdminConfig).Methods(http.MethodGet)
	apiRouter.HandleFunc("/status", p.handleStatus).Methods(http.MethodGet)
	apiRouter.HandleFunc("/bots", p.handleBots).Methods(http.MethodGet)
	apiRouter.HandleFunc("/history", p.handleHistory).Methods(http.MethodGet)
	apiRouter.HandleFunc("/run", p.handleRunBot).Methods(http.MethodPost)
	apiRouter.HandleFunc("/test", p.handleTestConnection).Methods(http.MethodPost)

	return router
}

func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Mattermost-User-ID") == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := p.getConfiguration()
	status := pluginStatusResponse{
		PluginID:    manifest.Id,
		BaseURL:     defaultUpstageEndpointURL,
		Bots:        []BotDefinition{},
		ManagedBots: []botSyncEntry{},
		BotSync:     p.getBotSyncState(),
	}

	runtimeCfg, err := cfg.normalize()
	if err != nil {
		status.ConfigError = err.Error()
		writeJSON(w, http.StatusOK, status)
		return
	}

	status.BotCount = len(runtimeCfg.BotDefinitions)
	status.AllowHosts = runtimeCfg.AllowHosts
	status.BaseURL = runtimeCfg.ServiceBaseURL
	status.Bots = sanitizeBotDefinitions(runtimeCfg.BotDefinitions)
	status.ManagedBots = status.BotSync.Entries
	writeJSON(w, http.StatusOK, status)
}

func (p *Plugin) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		writeError(w, http.StatusForbidden, errors.New("only system administrators can access plugin configuration"))
		return
	}

	stored, source, err := p.getConfiguration().getStoredPluginConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, adminConfigResponse{
		Config: stored,
		Source: source,
	})
}

func (p *Plugin) handleBots(w http.ResponseWriter, r *http.Request) {
	cfg, err := p.getRuntimeConfiguration()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"bots": sanitizeBotDefinitions(cfg.BotDefinitions)})
		return
	}

	channel, appErr := p.API.GetChannel(channelID)
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}
	team := p.getTeamForChannel(channel)
	user, appErr := p.API.GetUser(r.Header.Get("Mattermost-User-ID"))
	if appErr != nil {
		writeError(w, http.StatusBadRequest, appErr)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"bots": sanitizeBotDefinitions(cfg.getAllowedBots(user, channel, team)),
	})
}

func sanitizeBotDefinitions(bots []BotDefinition) []BotDefinition {
	sanitized := make([]BotDefinition, 0, len(bots))
	for _, bot := range bots {
		sanitized = append(sanitized, bot.publicView())
	}
	return sanitized
}

func (p *Plugin) handleHistory(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsedLimit, err := strconv.Atoi(rawLimit); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	history, err := p.getExecutionHistory(r.Header.Get("Mattermost-User-ID"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": history})
}

func (p *Plugin) handleRunBot(w http.ResponseWriter, r *http.Request) {
	var request runBotAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := p.executeBotAndPost(r.Context(), BotRunRequest{
		BotID:     request.BotID,
		UserID:    r.Header.Get("Mattermost-User-ID"),
		ChannelID: request.ChannelID,
		RootID:    request.RootID,
		Prompt:    request.Prompt,
		Inputs:    request.Inputs,
		FileIDs:   request.FileIDs,
		Source:    "webapp",
	})
	if err != nil {
		if result == nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusBadGateway, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (p *Plugin) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if !p.client.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		writeError(w, http.StatusForbidden, errors.New("only system administrators can test Upstage connectivity"))
		return
	}

	cfg, err := p.getRuntimeConfiguration()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	status, err := p.testUpstageConnection(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]string{"error": err.Error()})
}

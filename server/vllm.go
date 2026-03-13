package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type vllmServiceConfig struct {
	BaseURL       string
	ParsedBaseURL *url.URL
	APIKey        string
	Model         string
	Prompt        string
	Timeout       time.Duration
}

type vllmChatRequest struct {
	Model    string        `json:"model"`
	Messages []vllmMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type vllmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type vllmChatResponse struct {
	ID      string           `json:"id"`
	Model   string           `json:"model"`
	Choices []vllmChatChoice `json:"choices"`
}

type vllmChatChoice struct {
	Message vllmChoiceMessage `json:"message"`
	Text    string            `json:"text,omitempty"`
}

type vllmChoiceMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type vllmCallError struct {
	Code        string
	Summary     string
	Detail      string
	Hint        string
	RequestURL  string
	StatusCode  int
	Retryable   bool
	InputDebug  string
	OutputDebug string
}

type vllmRequestDebug struct {
	URL             string `json:"url"`
	Model           string `json:"model"`
	PromptTemplate  string `json:"prompt_template,omitempty"`
	RenderedPrompt  string `json:"rendered_prompt_preview,omitempty"`
	UserMessage     string `json:"user_message,omitempty"`
	DocumentPreview string `json:"document_preview,omitempty"`
	DocumentLength  int    `json:"document_length"`
	Correlation     string `json:"correlation_id,omitempty"`
}

type vllmResponseDebug struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Hint       string `json:"hint,omitempty"`
	Body       string `json:"body,omitempty"`
}

func (e *vllmCallError) Error() string {
	if e == nil {
		return ""
	}

	lines := []string{}
	if e.Summary != "" {
		lines = append(lines, e.Summary)
	}
	if e.Detail != "" {
		lines = append(lines, "상세: "+e.Detail)
	}
	if e.Hint != "" {
		lines = append(lines, "조치: "+e.Hint)
	}
	if e.StatusCode > 0 {
		lines = append(lines, fmt.Sprintf("HTTP 상태: %d", e.StatusCode))
	}
	return strings.Join(lines, "\n")
}

func (e *vllmCallError) withDebug(requestDebug vllmRequestDebug, responseDebug vllmResponseDebug) *vllmCallError {
	if e == nil {
		return nil
	}

	copyErr := *e
	copyErr.InputDebug = marshalDebugPayload(requestDebug)
	copyErr.OutputDebug = marshalDebugPayload(responseDebug)
	return &copyErr
}

func (cfg *runtimeConfiguration) serviceConfigForVLLMBot(bot BotDefinition) (vllmServiceConfig, error) {
	normalizedURL, parsedURL, err := normalizeVLLMEndpointURL(bot.VLLMBaseURL)
	if err != nil {
		return vllmServiceConfig{}, err
	}
	if !hostAllowed(parsedURL.Hostname(), cfg.AllowHosts) {
		return vllmServiceConfig{}, fmt.Errorf("vLLM host %q is not allowed by configuration", parsedURL.Hostname())
	}
	if strings.TrimSpace(bot.VLLMModel) == "" {
		return vllmServiceConfig{}, fmt.Errorf("bot %q requires vLLM model when vLLM URL is configured", bot.Username)
	}

	return vllmServiceConfig{
		BaseURL:       normalizedURL,
		ParsedBaseURL: parsedURL,
		APIKey:        strings.TrimSpace(bot.VLLMAPIKey),
		Model:         strings.TrimSpace(bot.VLLMModel),
		Prompt:        strings.TrimSpace(bot.VLLMPrompt),
		Timeout:       cfg.DefaultTimeout,
	}, nil
}

func normalizeVLLMEndpointURL(raw string) (string, *url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, fmt.Errorf("vLLM URL is required")
	}

	parsedURL, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("invalid vLLM URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", nil, fmt.Errorf("vLLM URL must include scheme and host")
	}

	path := strings.TrimRight(parsedURL.Path, "/")
	switch {
	case path == "", path == "/":
		parsedURL.Path = "/v1/chat/completions"
	case path == "/v1":
		parsedURL.Path = "/v1/chat/completions"
	case strings.HasSuffix(path, "/chat/completions"):
		parsedURL.Path = path
	default:
		parsedURL.Path = path + "/chat/completions"
	}

	return parsedURL.String(), parsedURL, nil
}

func (p *Plugin) invokeVLLMPostProcess(
	ctx context.Context,
	service vllmServiceConfig,
	userMessage string,
	documentText string,
	correlationID string,
) (string, string, error) {
	renderedPrompt := renderVLLMPrompt(service.Prompt, userMessage, documentText)
	requestPayload := vllmChatRequest{
		Model: service.Model,
		Messages: []vllmMessage{{
			Role:    "user",
			Content: renderedPrompt,
		}},
		Stream: false,
	}
	requestDebug := buildVLLMRequestDebug(service, service.Prompt, renderedPrompt, userMessage, documentText, correlationID)

	bodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode vLLM request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", fmt.Errorf("failed to build vLLM request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Request-Id", correlationID)
	request.Header.Set("X-Correlation-ID", correlationID)
	if service.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+service.APIKey)
	}

	client := &http.Client{Timeout: resolveUpstageRequestTimeout(service.Timeout)}
	response, err := client.Do(request)
	if err != nil {
		return "", "", attachVLLMDebug(
			classifyVLLMRequestError(service.BaseURL, err),
			requestDebug,
			vllmResponseDebug{},
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		callErr := newVLLMCallError(
			"response_read_failed",
			"vLLM 응답 본문을 읽는 중 오류가 발생했습니다.",
			err.Error(),
			"vLLM 서버 상태와 응답 크기 제한을 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			true,
		)
		return "", "", callErr.withDebug(
			requestDebug,
			buildVLLMResponseDebug(response.StatusCode, response.Header, nil, callErr),
		)
	}

	if response.StatusCode >= http.StatusBadRequest {
		callErr := classifyVLLMHTTPError(service.BaseURL, response.StatusCode, response.Header, responseBody)
		return "", "", attachVLLMDebug(
			callErr,
			requestDebug,
			buildVLLMResponseDebug(response.StatusCode, response.Header, responseBody, callErr),
		)
	}

	var parsed vllmChatResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		callErr := newVLLMCallError(
			"decode_failed",
			"vLLM 응답 JSON을 해석하지 못했습니다.",
			err.Error(),
			"vLLM OpenAI 호환 엔드포인트와 응답 형식을 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			false,
		)
		return "", "", callErr.withDebug(
			requestDebug,
			buildVLLMResponseDebug(response.StatusCode, response.Header, responseBody, callErr),
		)
	}

	content := extractVLLMResponseText(parsed)
	if content == "" {
		callErr := newVLLMCallError(
			"empty_response",
			"vLLM이 비어 있는 응답을 반환했습니다.",
			"choices[0].message.content 에서 텍스트를 찾지 못했습니다.",
			"vLLM 모델과 chat template 구성을 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			false,
		)
		return "", "", callErr.withDebug(
			requestDebug,
			buildVLLMResponseDebug(response.StatusCode, response.Header, responseBody, callErr),
		)
	}

	return strings.TrimSpace(content), marshalDebugPayload(requestDebug), nil
}

func renderVLLMPrompt(template, userMessage, documentText string) string {
	template = strings.TrimSpace(template)
	userMessage = strings.TrimSpace(userMessage)
	documentText = strings.TrimSpace(documentText)

	if strings.Contains(template, "{{document_text}}") || strings.Contains(template, "{{user_message}}") {
		rendered := strings.ReplaceAll(template, "{{document_text}}", documentText)
		rendered = strings.ReplaceAll(rendered, "{{user_message}}", userMessage)
		return strings.TrimSpace(rendered)
	}

	parts := make([]string, 0, 3)
	if template != "" {
		parts = append(parts, template)
	}
	if userMessage != "" {
		parts = append(parts, "[사용자 요청]\n"+userMessage)
	}
	if documentText != "" {
		parts = append(parts, "[문서 텍스트]\n"+documentText)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func buildVLLMRequestDebug(service vllmServiceConfig, promptTemplate, renderedPrompt, userMessage, documentText, correlationID string) vllmRequestDebug {
	return vllmRequestDebug{
		URL:             strings.TrimSpace(service.BaseURL),
		Model:           strings.TrimSpace(service.Model),
		PromptTemplate:  truncateString(strings.TrimSpace(promptTemplate), 2000),
		RenderedPrompt:  truncateString(strings.TrimSpace(renderedPrompt), 4000),
		UserMessage:     truncateString(strings.TrimSpace(userMessage), 1000),
		DocumentPreview: truncateString(strings.TrimSpace(documentText), 4000),
		DocumentLength:  len(documentText),
		Correlation:     strings.TrimSpace(correlationID),
	}
}

func buildVLLMResponseDebug(statusCode int, headers http.Header, body []byte, callErr *vllmCallError) vllmResponseDebug {
	responseDebug := vllmResponseDebug{
		StatusCode: statusCode,
		RequestID:  firstHeaderValue(headers, "X-Request-Id", "X-Request-ID", "X-Correlation-ID"),
		Body:       formatDebugBody(body),
	}
	if callErr != nil {
		responseDebug.ErrorCode = strings.TrimSpace(callErr.Code)
		responseDebug.Summary = strings.TrimSpace(callErr.Summary)
		responseDebug.Detail = strings.TrimSpace(callErr.Detail)
		responseDebug.Hint = strings.TrimSpace(callErr.Hint)
	}
	return responseDebug
}

func extractVLLMResponseText(response vllmChatResponse) string {
	for _, choice := range response.Choices {
		if text := strings.TrimSpace(choice.Text); text != "" {
			return text
		}
		switch content := choice.Message.Content.(type) {
		case string:
			if strings.TrimSpace(content) != "" {
				return content
			}
		case []any:
			parts := make([]string, 0, len(content))
			for _, item := range content {
				if text := extractTextFromValue(item); text != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		default:
			if text := extractTextFromValue(content); text != "" {
				return text
			}
		}
	}
	return ""
}

func attachVLLMDebug(err error, requestDebug vllmRequestDebug, responseDebug vllmResponseDebug) error {
	if err == nil {
		return nil
	}

	var callErr *vllmCallError
	if errors.As(err, &callErr) {
		return callErr.withDebug(requestDebug, responseDebug)
	}
	return err
}

func newVLLMCallError(code, summary, detail, hint, requestURL string, statusCode int, retryable bool) *vllmCallError {
	return &vllmCallError{
		Code:       strings.TrimSpace(code),
		Summary:    strings.TrimSpace(summary),
		Detail:     strings.TrimSpace(detail),
		Hint:       strings.TrimSpace(hint),
		RequestURL: strings.TrimSpace(requestURL),
		StatusCode: statusCode,
		Retryable:  retryable,
	}
}

func classifyVLLMHTTPError(requestURL string, statusCode int, headers http.Header, body []byte) *vllmCallError {
	bodySummary := summarizeResponseBody(body)
	requestID := firstHeaderValue(headers, "X-Request-Id", "X-Request-ID", "X-Correlation-ID")
	if requestID != "" {
		bodySummary = strings.TrimSpace(bodySummary + " (vLLM request id: " + requestID + ")")
	}

	switch statusCode {
	case http.StatusBadRequest:
		return newVLLMCallError(
			"bad_request",
			"vLLM 후처리 요청이 거부되었습니다.",
			defaultIfEmpty(bodySummary, "요청 프롬프트 또는 모델 설정이 vLLM 요구사항과 맞지 않습니다."),
			"vLLM URL, model, prompt 설정과 chat template 지원 여부를 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusUnauthorized, http.StatusForbidden:
		return newVLLMCallError(
			"auth_failed",
			"vLLM 인증에 실패했습니다.",
			defaultIfEmpty(bodySummary, "API 키가 유효하지 않거나 권한이 없습니다."),
			"봇별 vLLM API key 와 프록시 인증 구성을 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusNotFound:
		return newVLLMCallError(
			"not_found",
			"vLLM API 엔드포인트를 찾지 못했습니다.",
			defaultIfEmpty(bodySummary, "chat completions 경로가 올바르지 않습니다."),
			"봇별 vLLM URL에 /v1 또는 /chat/completions 경로가 올바른지 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusTooManyRequests:
		return newVLLMCallError(
			"rate_limited",
			"vLLM 호출 한도에 걸렸습니다.",
			defaultIfEmpty(bodySummary, "잠시 후 다시 시도해야 합니다."),
			"요청 빈도를 줄이거나 잠시 후 다시 시도하세요.",
			requestURL,
			statusCode,
			true,
		)
	default:
		if statusCode >= http.StatusInternalServerError {
			return newVLLMCallError(
				"server_error",
				"vLLM 서버 내부 오류가 발생했습니다.",
				defaultIfEmpty(bodySummary, "vLLM 서버가 5xx 오류를 반환했습니다."),
				"잠시 후 다시 시도하고, 반복되면 vLLM 서버 상태와 로그를 확인하세요.",
				requestURL,
				statusCode,
				true,
			)
		}
		return newVLLMCallError(
			"unexpected_status",
			fmt.Sprintf("vLLM이 예상하지 못한 HTTP 상태 %d 를 반환했습니다.", statusCode),
			bodySummary,
			"응답 본문과 vLLM 설정을 함께 확인하세요.",
			requestURL,
			statusCode,
			statusCode >= 500,
		)
	}
}

func classifyVLLMRequestError(requestURL string, err error) *vllmCallError {
	detail := strings.TrimSpace(err.Error())

	var timeoutError interface{ Timeout() bool }
	if errors.As(err, &timeoutError) && timeoutError.Timeout() {
		return newVLLMCallError(
			"network_timeout",
			"vLLM 서버 연결이 시간 초과되었습니다.",
			detail,
			"vLLM 서버 상태와 네트워크 지연, 플러그인 타임아웃 설정을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newVLLMCallError(
			"network_timeout",
			"vLLM 서버 연결이 시간 초과되었습니다.",
			detail,
			"vLLM 서버 상태와 플러그인 타임아웃 값을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return newVLLMCallError(
			"dns_error",
			"vLLM 호스트 이름을 찾지 못했습니다.",
			detail,
			"봇별 vLLM URL의 도메인 이름과 DNS 설정을 확인하세요.",
			requestURL,
			0,
			false,
		)
	}

	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "connection refused"):
		return newVLLMCallError(
			"connection_refused",
			"vLLM 서버가 연결을 거부했습니다.",
			detail,
			"vLLM API 서버가 실행 중인지, 포트와 방화벽이 올바른지 확인하세요.",
			requestURL,
			0,
			true,
		)
	case strings.Contains(lower, "no such host"):
		return newVLLMCallError(
			"dns_error",
			"vLLM 호스트 이름을 찾지 못했습니다.",
			detail,
			"봇별 vLLM URL의 도메인 이름과 DNS 설정을 확인하세요.",
			requestURL,
			0,
			false,
		)
	default:
		return newVLLMCallError(
			"network_error",
			"vLLM 서버에 연결하지 못했습니다.",
			detail,
			"vLLM URL, 네트워크 경로, 방화벽, 프록시 설정을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}
}

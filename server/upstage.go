package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type upstageServiceConfig struct {
	BaseURL       string
	ParsedBaseURL *url.URL
	AuthMode      string
	AuthToken     string
}

type upstageConnectionStatus struct {
	OK         bool   `json:"ok"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	ErrorCode  string `json:"error_code,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Hint       string `json:"hint,omitempty"`
	Retryable  bool   `json:"retryable"`
}

type upstageParseContent struct {
	HTML     string `json:"html"`
	Markdown string `json:"markdown"`
	Text     string `json:"text"`
}

type upstageUsage struct {
	Pages int `json:"pages"`
}

type upstageParseResponse struct {
	API     string              `json:"api"`
	Content upstageParseContent `json:"content"`
	Model   string              `json:"model"`
	Usage   upstageUsage        `json:"usage"`
}

type upstageDocumentResult struct {
	Attachment botAttachment
	Response   upstageParseResponse
}

type upstageCallError struct {
	Code       string
	Summary    string
	Detail     string
	Hint       string
	RequestURL string
	StatusCode int
	Retryable  bool
}

func (e *upstageCallError) Error() string {
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
	if e.RequestURL != "" {
		lines = append(lines, "요청 URL: "+e.RequestURL)
	}

	return strings.Join(lines, "\n")
}

func (e *upstageCallError) toConnectionStatus() *upstageConnectionStatus {
	if e == nil {
		return &upstageConnectionStatus{}
	}

	return &upstageConnectionStatus{
		OK:         false,
		URL:        e.RequestURL,
		StatusCode: e.StatusCode,
		Message:    e.Summary,
		ErrorCode:  e.Code,
		Detail:     e.Detail,
		Hint:       e.Hint,
		Retryable:  e.Retryable,
	}
}

func normalizeUpstageEndpointURL(raw string) (string, *url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultUpstageEndpointURL
	}

	parsedURL, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("invalid Upstage endpoint URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", nil, fmt.Errorf("Upstage endpoint URL must include scheme and host")
	}

	if strings.EqualFold(parsedURL.Hostname(), "console.upstage.ai") {
		parsedURL, err = url.Parse(defaultUpstageEndpointURL)
		if err != nil {
			return "", nil, err
		}
		return parsedURL.String(), parsedURL, nil
	}

	if strings.EqualFold(parsedURL.Hostname(), "api.upstage.ai") {
		path := strings.TrimRight(parsedURL.Path, "/")
		switch path {
		case "", "/":
			parsedURL.Path = "/v1/document-digitization"
		case "/v1":
			parsedURL.Path = "/v1/document-digitization"
		}
	}

	return strings.TrimRight(parsedURL.String(), "/"), parsedURL, nil
}

func (cfg *runtimeConfiguration) serviceConfigForBot(bot BotDefinition) (upstageServiceConfig, error) {
	baseURL := strings.TrimSpace(bot.BaseURL)
	if baseURL == "" {
		baseURL = cfg.ServiceBaseURL
	}
	normalizedURL, parsedURL, err := normalizeUpstageEndpointURL(baseURL)
	if err != nil {
		return upstageServiceConfig{}, err
	}
	if !hostAllowed(parsedURL.Hostname(), cfg.AllowHosts) {
		return upstageServiceConfig{}, fmt.Errorf("Upstage host %q is not allowed by configuration", parsedURL.Hostname())
	}

	authMode := normalizeAuthMode(bot.AuthMode)
	if strings.TrimSpace(bot.AuthMode) == "" {
		authMode = cfg.AuthMode
	}
	authToken := strings.TrimSpace(bot.AuthToken)
	if authToken == "" {
		authToken = cfg.AuthToken
	}

	return upstageServiceConfig{
		BaseURL:       normalizedURL,
		ParsedBaseURL: parsedURL,
		AuthMode:      authMode,
		AuthToken:     authToken,
	}, nil
}

func (p *Plugin) invokeUpstageDocumentParse(
	ctx context.Context,
	service upstageServiceConfig,
	bot BotDefinition,
	attachment botAttachment,
	correlationID string,
) (upstageDocumentResult, int, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"model":                  defaultIfEmpty(strings.TrimSpace(bot.Model), defaultUpstageModel),
		"mode":                   normalizeUpstageMode(bot.Mode),
		"ocr":                    normalizeUpstageOCRMode(bot.OCR),
		"coordinates":            strconv.FormatBool(bot.useCoordinates()),
		"chart_recognition":      strconv.FormatBool(bot.useChartRecognition()),
		"merge_multipage_tables": strconv.FormatBool(bot.useMergeMultipageTables()),
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to write %s field: %w", key, err)
		}
	}

	if len(bot.OutputFormats) > 0 {
		rawOutputFormats, err := json.Marshal(bot.OutputFormats)
		if err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to encode output formats: %w", err)
		}
		if err := writer.WriteField("output_formats", string(rawOutputFormats)); err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to write output_formats field: %w", err)
		}
	}
	if len(bot.Base64Encoding) > 0 {
		rawBase64Encoding, err := json.Marshal(bot.Base64Encoding)
		if err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to encode base64_encoding: %w", err)
		}
		if err := writer.WriteField("base64_encoding", string(rawBase64Encoding)); err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to write base64_encoding field: %w", err)
		}
	}

	part, err := writer.CreateFormFile("document", sanitizeUploadFilename(attachment.Name))
	if err != nil {
		return upstageDocumentResult{}, 0, fmt.Errorf("failed to create document form field: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(attachment.Content)); err != nil {
		return upstageDocumentResult{}, 0, fmt.Errorf("failed to write document content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return upstageDocumentResult{}, 0, fmt.Errorf("failed to finalize multipart body: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.BaseURL, &body)
	if err != nil {
		return upstageDocumentResult{}, 0, fmt.Errorf("failed to build Upstage request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Correlation-ID", correlationID)
	applyAuthHeader(request, service)

	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return upstageDocumentResult{}, 0, classifyUpstageRequestError(service.BaseURL, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		return upstageDocumentResult{}, response.StatusCode, newUpstageCallError(
			"response_read_failed",
			"Upstage 응답 본문을 읽는 중 오류가 발생했습니다.",
			err.Error(),
			"Upstage 응답 크기 제한과 네트워크 상태를 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			true,
		)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return upstageDocumentResult{}, response.StatusCode, classifyUpstageHTTPError(service.BaseURL, response.StatusCode, response.Header, responseBody)
	}

	var parsed upstageParseResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return upstageDocumentResult{}, response.StatusCode, newUpstageCallError(
			"decode_failed",
			"Upstage 응답 JSON을 해석하지 못했습니다.",
			err.Error(),
			"엔드포인트 URL과 응답 형식을 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			false,
		)
	}
	if strings.TrimSpace(parsed.Model) == "" {
		parsed.Model = bot.Model
	}

	return upstageDocumentResult{
		Attachment: attachment,
		Response:   parsed,
	}, response.StatusCode, nil
}

func choosePreferredUpstageContent(response upstageParseResponse, preferred []string) (string, string) {
	contentByFormat := map[string]string{
		"markdown": strings.TrimSpace(response.Content.Markdown),
		"text":     strings.TrimSpace(response.Content.Text),
		"html":     strings.TrimSpace(response.Content.HTML),
	}

	for _, format := range preferred {
		format = strings.ToLower(strings.TrimSpace(format))
		if contentByFormat[format] != "" {
			return format, contentByFormat[format]
		}
	}
	for _, format := range []string{"markdown", "text", "html"} {
		if contentByFormat[format] != "" {
			return format, contentByFormat[format]
		}
	}

	pretty, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", ""
	}
	return "json", string(pretty)
}

func (p *Plugin) testUpstageConnection(ctx context.Context, cfg *runtimeConfiguration) (*upstageConnectionStatus, error) {
	serviceConfig, err := cfg.serviceConfigForBot(BotDefinition{})
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", defaultUpstageModel); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceConfig.BaseURL, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create Upstage connection test request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	applyAuthHeader(request, serviceConfig)

	client := &http.Client{Timeout: minDuration(cfg.DefaultTimeout, 10*time.Second)}
	response, err := client.Do(request)
	if err != nil {
		return classifyUpstageRequestError(serviceConfig.BaseURL, err).toConnectionStatus(), nil
	}
	defer response.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(response.Body, 32*1024))
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnsupportedMediaType || response.StatusCode == http.StatusUnprocessableEntity {
		return &upstageConnectionStatus{
			OK:         true,
			URL:        serviceConfig.BaseURL,
			StatusCode: response.StatusCode,
			Message:    "엔드포인트 연결과 인증은 확인되었습니다. 테스트 요청은 문서 파일이 없어 예상대로 거부되었습니다.",
		}, nil
	}
	if response.StatusCode >= http.StatusBadRequest {
		return classifyUpstageHTTPError(serviceConfig.BaseURL, response.StatusCode, response.Header, bodyBytes).toConnectionStatus(), nil
	}

	return &upstageConnectionStatus{
		OK:         true,
		URL:        serviceConfig.BaseURL,
		StatusCode: response.StatusCode,
		Message:    defaultIfEmpty(strings.TrimSpace(string(bodyBytes)), "연결에 성공했습니다."),
	}, nil
}

func applyAuthHeader(request *http.Request, service upstageServiceConfig) {
	if service.AuthToken == "" {
		return
	}
	if service.AuthMode == "x-api-key" {
		request.Header.Set("x-api-key", service.AuthToken)
		return
	}
	request.Header.Set("Authorization", "Bearer "+service.AuthToken)
}

func summarizeResponseBody(body []byte) string {
	text := extractTextFromBody(body)
	if text != "" {
		return truncateString(text, 280)
	}
	return truncateString(strings.TrimSpace(string(body)), 280)
}

func extractTextFromBody(body []byte) string {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body))
	}

	text := extractTextFromValue(payload)
	if text != "" {
		return text
	}

	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return string(pretty)
}

func extractTextFromValue(value any) string {
	candidates := make([]string, 0, 8)
	collectTextCandidates(value, &candidates)

	best := ""
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if len(candidate) > len(best) {
			best = candidate
		}
	}

	return best
}

func collectTextCandidates(value any, candidates *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			lowerKey := strings.ToLower(key)
			if isLikelyTextKey(lowerKey) {
				switch nestedValue := nested.(type) {
				case string:
					*candidates = append(*candidates, nestedValue)
				case map[string]any, []any:
					collectTextCandidates(nestedValue, candidates)
				}
				continue
			}
			collectTextCandidates(nested, candidates)
		}
	case []any:
		for _, item := range typed {
			collectTextCandidates(item, candidates)
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			*candidates = append(*candidates, typed)
		}
	}
}

func isLikelyTextKey(key string) bool {
	return strings.Contains(key, "text") ||
		strings.Contains(key, "message") ||
		strings.Contains(key, "output") ||
		strings.Contains(key, "result") ||
		strings.Contains(key, "content") ||
		strings.Contains(key, "response") ||
		strings.Contains(key, "detail") ||
		strings.Contains(key, "error")
}

func truncateString(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	if maxLength <= 3 {
		return value[:maxLength]
	}
	return value[:maxLength-3] + "..."
}

func minDuration(values ...time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func newUpstageCallError(code, summary, detail, hint, requestURL string, statusCode int, retryable bool) *upstageCallError {
	return &upstageCallError{
		Code:       code,
		Summary:    strings.TrimSpace(summary),
		Detail:     strings.TrimSpace(detail),
		Hint:       strings.TrimSpace(hint),
		RequestURL: strings.TrimSpace(requestURL),
		StatusCode: statusCode,
		Retryable:  retryable,
	}
}

func classifyUpstageHTTPError(requestURL string, statusCode int, headers http.Header, body []byte) *upstageCallError {
	bodySummary := summarizeResponseBody(body)
	requestID := firstHeaderValue(headers, "X-Request-Id", "X-Request-ID", "X-Correlation-ID")
	if requestID != "" {
		bodySummary = strings.TrimSpace(bodySummary + " (Upstage request id: " + requestID + ")")
	}

	switch statusCode {
	case http.StatusBadRequest:
		return newUpstageCallError(
			"bad_request",
			"Upstage가 요청을 거부했습니다.",
			defaultIfEmpty(bodySummary, "입력 문서 또는 파라미터 형식이 Upstage 요구사항과 맞지 않습니다."),
			"문서 파일 형식, output_formats, mode, ocr 값을 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusUnauthorized, http.StatusForbidden:
		return newUpstageCallError(
			"auth_failed",
			"Upstage 인증에 실패했습니다.",
			defaultIfEmpty(bodySummary, "API 키가 유효하지 않거나 권한이 없습니다."),
			"System Console의 인증 토큰과 헤더 방식을 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusNotFound:
		return newUpstageCallError(
			"not_found",
			"Upstage API 엔드포인트를 찾지 못했습니다.",
			defaultIfEmpty(bodySummary, "document-digitization 경로가 올바르지 않습니다."),
			"기본 URL에 전체 Upstage endpoint를 입력했는지 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusTooManyRequests:
		return newUpstageCallError(
			"rate_limited",
			"Upstage 호출 한도에 걸렸습니다.",
			defaultIfEmpty(bodySummary, "잠시 후 다시 시도해야 합니다."),
			"요청 빈도를 줄이거나 잠시 후 다시 시도하세요.",
			requestURL,
			statusCode,
			true,
		)
	case http.StatusRequestEntityTooLarge:
		return newUpstageCallError(
			"file_too_large",
			"업로드한 문서 파일이 너무 큽니다.",
			defaultIfEmpty(bodySummary, "Upstage가 파일 크기 제한을 초과한 요청을 거부했습니다."),
			"더 작은 파일로 나누거나 비동기 API 사용을 검토하세요.",
			requestURL,
			statusCode,
			false,
		)
	case http.StatusUnsupportedMediaType:
		return newUpstageCallError(
			"unsupported_media_type",
			"Upstage가 이 문서 형식을 지원하지 않습니다.",
			defaultIfEmpty(bodySummary, "지원되지 않는 파일 형식입니다."),
			"PDF, 이미지, DOCX 등 Upstage가 지원하는 형식인지 확인하세요.",
			requestURL,
			statusCode,
			false,
		)
	default:
		if statusCode >= http.StatusInternalServerError {
			return newUpstageCallError(
				"server_error",
				"Upstage 서버 내부 오류가 발생했습니다.",
				defaultIfEmpty(bodySummary, "Upstage 서버가 5xx 오류를 반환했습니다."),
				"잠시 후 다시 시도하고, 반복되면 Upstage 상태와 서버 로그를 확인하세요.",
				requestURL,
				statusCode,
				true,
			)
		}
		return newUpstageCallError(
			"unexpected_status",
			fmt.Sprintf("Upstage가 예상하지 못한 HTTP 상태 %d 를 반환했습니다.", statusCode),
			bodySummary,
			"응답 본문과 Upstage 설정을 함께 확인하세요.",
			requestURL,
			statusCode,
			statusCode >= 500,
		)
	}
}

func classifyUpstageRequestError(requestURL string, err error) *upstageCallError {
	detail := strings.TrimSpace(err.Error())

	var timeoutError interface{ Timeout() bool }
	if errors.As(err, &timeoutError) && timeoutError.Timeout() {
		return newUpstageCallError(
			"network_timeout",
			"Upstage 서버 연결이 시간 초과되었습니다.",
			detail,
			"Upstage 서버 상태와 네트워크 지연, 플러그인 타임아웃 설정을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newUpstageCallError(
			"network_timeout",
			"Upstage 서버 연결이 시간 초과되었습니다.",
			detail,
			"Upstage 서버 상태와 플러그인 타임아웃 값을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return newUpstageCallError(
			"dns_error",
			"Upstage 호스트 이름을 찾지 못했습니다.",
			detail,
			"기본 URL의 도메인 이름과 DNS 설정을 확인하세요.",
			requestURL,
			0,
			false,
		)
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return newUpstageCallError(
			"tls_hostname_error",
			"TLS 인증서의 호스트 이름이 Upstage URL과 일치하지 않습니다.",
			detail,
			"인증서의 SAN/CN과 기본 URL 호스트가 일치하는지 확인하세요.",
			requestURL,
			0,
			false,
		)
	}

	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return newUpstageCallError(
			"tls_unknown_authority",
			"Upstage TLS 인증서를 신뢰할 수 없습니다.",
			detail,
			"사설 인증서를 사용 중이면 Mattermost 서버가 해당 루트 인증서를 신뢰하도록 구성하세요.",
			requestURL,
			0,
			false,
		)
	}

	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "connection refused"):
		return newUpstageCallError(
			"connection_refused",
			"Upstage 서버가 연결을 거부했습니다.",
			detail,
			"Upstage API 서버가 실행 중인지, 포트와 방화벽이 올바른지 확인하세요.",
			requestURL,
			0,
			true,
		)
	case strings.Contains(lower, "no such host"):
		return newUpstageCallError(
			"dns_error",
			"Upstage 호스트 이름을 찾지 못했습니다.",
			detail,
			"기본 URL의 도메인 이름과 DNS 설정을 확인하세요.",
			requestURL,
			0,
			false,
		)
	case strings.Contains(lower, "certificate"), strings.Contains(lower, "tls"):
		return newUpstageCallError(
			"tls_error",
			"Upstage TLS 연결을 설정하지 못했습니다.",
			detail,
			"HTTPS 인증서 체인과 프록시 TLS 구성을 확인하세요.",
			requestURL,
			0,
			false,
		)
	default:
		return newUpstageCallError(
			"network_error",
			"Upstage 서버에 연결하지 못했습니다.",
			detail,
			"기본 URL, 네트워크 경로, 방화벽, 프록시 설정을 확인하세요.",
			requestURL,
			0,
			true,
		)
	}
}

func firstHeaderValue(headers http.Header, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

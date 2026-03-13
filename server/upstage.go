package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type upstageServiceConfig struct {
	BaseURL       string
	ParsedBaseURL *url.URL
	AuthMode      string
	AuthToken     string
	Timeout       time.Duration
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

type upstageParseElement struct {
	Category string              `json:"category"`
	Content  upstageParseContent `json:"content"`
	Page     int                 `json:"page"`
}

type upstageParseResponse struct {
	API      string                `json:"api"`
	Content  upstageParseContent   `json:"content"`
	Elements []upstageParseElement `json:"elements"`
	Model    string                `json:"model"`
	Usage    upstageUsage          `json:"usage"`
}

type upstageDocumentResult struct {
	Attachment    botAttachment
	Response      upstageParseResponse
	RequestDebugs []upstageRequestDebug
}

type upstageCallError struct {
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

type upstageRequestDebug struct {
	URL         string                 `json:"url"`
	AuthMode    string                 `json:"auth_mode"`
	Fields      map[string]string      `json:"fields"`
	Attachment  upstageAttachmentDebug `json:"attachment"`
	Correlation string                 `json:"correlation_id,omitempty"`
	Attempt     string                 `json:"attempt,omitempty"`
}

type upstageAttachmentDebug struct {
	Name      string `json:"name"`
	MIMEType  string `json:"mime_type"`
	Extension string `json:"extension,omitempty"`
	Size      int64  `json:"size"`
}

type upstageResponseDebug struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Hint       string `json:"hint,omitempty"`
	Body       string `json:"body,omitempty"`
}

var upstageHTMLTagPattern = regexp.MustCompile(`(?s)<[^>]+>`)

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
		Timeout:       cfg.DefaultTimeout,
	}, nil
}

func (p *Plugin) invokeUpstageDocumentParse(
	ctx context.Context,
	service upstageServiceConfig,
	bot BotDefinition,
	attachment botAttachment,
	correlationID string,
) (upstageDocumentResult, int, time.Duration, error) {
	fields, err := buildUpstageFormFields(bot)
	if err != nil {
		return upstageDocumentResult{}, 0, 0, err
	}
	initialDebug := buildUpstageRequestDebug(service, fields, attachment, correlationID, "")

	startedAt := time.Now()
	result, statusCode, err := p.performUpstageDocumentParseRequest(ctx, service, bot, attachment, fields, initialDebug)
	elapsed := time.Since(startedAt)
	if err == nil {
		result.RequestDebugs = append(result.RequestDebugs, initialDebug)
		return result, statusCode, elapsed, nil
	}
	if statusCode != http.StatusBadRequest || strings.TrimSpace(fields["output_formats"]) == "" {
		return result, statusCode, elapsed, err
	}

	retryFields := cloneUpstageFields(fields)
	delete(retryFields, "output_formats")
	retryDebug := buildUpstageRequestDebug(service, retryFields, attachment, correlationID, "retry_without_output_formats")

	retryResult, retryStatus, retryErr := p.performUpstageDocumentParseRequest(ctx, service, bot, attachment, retryFields, retryDebug)
	if retryErr == nil {
		retryResult.RequestDebugs = append(retryResult.RequestDebugs, initialDebug, retryDebug)
		p.API.LogWarn("Upstage request succeeded after retrying without output_formats", "correlation_id", correlationID, "bot_id", bot.ID, "filename", sanitizeUploadFilename(attachment.Name))
		return retryResult, retryStatus, time.Since(startedAt), nil
	}

	return retryResult, retryStatus, time.Since(startedAt), attachUpstageAttemptDebug(retryErr, []upstageRequestDebug{initialDebug, retryDebug})
}

func (p *Plugin) performUpstageDocumentParseRequest(
	ctx context.Context,
	service upstageServiceConfig,
	bot BotDefinition,
	attachment botAttachment,
	fields map[string]string,
	requestDebug upstageRequestDebug,
) (upstageDocumentResult, int, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return upstageDocumentResult{}, 0, fmt.Errorf("failed to write %s field: %w", key, err)
		}
	}

	part, err := createDocumentPart(writer, attachment)
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
	request.Header.Set("X-Correlation-ID", strings.TrimSpace(requestDebug.Correlation))
	applyAuthHeader(request, service)

	client := &http.Client{Timeout: resolveUpstageRequestTimeout(service.Timeout)}
	response, err := client.Do(request)
	if err != nil {
		return upstageDocumentResult{}, 0, attachUpstageDebug(
			classifyUpstageRequestError(service.BaseURL, err),
			requestDebug,
			upstageResponseDebug{},
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		callErr := newUpstageCallError(
			"response_read_failed",
			"Upstage 응답 본문을 읽는 중 오류가 발생했습니다.",
			err.Error(),
			"Upstage 응답 크기 제한과 네트워크 상태를 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			true,
		)
		return upstageDocumentResult{}, response.StatusCode, callErr.withDebug(
			requestDebug,
			buildUpstageResponseDebug(response.StatusCode, response.Header, nil, callErr),
		)
	}
	if response.StatusCode >= http.StatusBadRequest {
		callErr := classifyUpstageHTTPError(service.BaseURL, response.StatusCode, response.Header, responseBody)
		return upstageDocumentResult{}, response.StatusCode, attachUpstageDebug(
			callErr,
			requestDebug,
			buildUpstageResponseDebug(response.StatusCode, response.Header, responseBody, callErr),
		)
	}

	var parsed upstageParseResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		callErr := newUpstageCallError(
			"decode_failed",
			"Upstage 응답 JSON을 해석하지 못했습니다.",
			err.Error(),
			"엔드포인트 URL과 응답 형식을 확인하세요.",
			service.BaseURL,
			response.StatusCode,
			false,
		)
		return upstageDocumentResult{}, response.StatusCode, callErr.withDebug(
			requestDebug,
			buildUpstageResponseDebug(response.StatusCode, response.Header, responseBody, callErr),
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

func buildUpstageFormFields(bot BotDefinition) (map[string]string, error) {
	fields := map[string]string{
		"model": defaultIfEmpty(strings.TrimSpace(bot.Model), defaultUpstageModel),
	}

	if formatted, err := formatUpstageListField(bot.OutputFormats); err != nil {
		return nil, fmt.Errorf("failed to encode output_formats: %w", err)
	} else if formatted != "" {
		fields["output_formats"] = formatted
	}

	if formatted, err := formatUpstageListField(bot.Base64Encoding); err != nil {
		return nil, fmt.Errorf("failed to encode base64_encoding: %w", err)
	} else if formatted != "" {
		fields["base64_encoding"] = formatted
	}

	if mode := normalizeUpstageMode(bot.Mode); mode != "standard" {
		fields["mode"] = mode
	}
	if ocr := normalizeUpstageOCRMode(bot.OCR); ocr != "auto" {
		fields["ocr"] = ocr
	}
	if !bot.useCoordinates() {
		fields["coordinates"] = strconv.FormatBool(false)
	}
	if !bot.useChartRecognition() {
		fields["chart_recognition"] = strconv.FormatBool(false)
	}
	if bot.useMergeMultipageTables() {
		fields["merge_multipage_tables"] = strconv.FormatBool(true)
	}

	return fields, nil
}

func formatUpstageListField(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}

	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func createDocumentPart(writer *multipart.Writer, attachment botAttachment) (io.Writer, error) {
	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition", fmt.Sprintf(`form-data; name="document"; filename="%s"`, escapeMultipartValue(sanitizeUploadFilename(attachment.Name))))
	headers.Set("Content-Type", defaultIfEmpty(strings.TrimSpace(attachment.MIMEType), "application/octet-stream"))
	return writer.CreatePart(headers)
}

func escapeMultipartValue(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
	return replacer.Replace(value)
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

func renderableUpstageElementContent(element upstageParseElement, format string) string {
	switch format {
	case "markdown":
		if value := strings.TrimSpace(element.Content.Markdown); value != "" {
			return value
		}
		if value := strings.TrimSpace(element.Content.Text); value != "" {
			return value
		}
		if value := strings.TrimSpace(element.Content.HTML); value != "" {
			return convertUpstageHTMLFragmentToMessage(element.Category, value)
		}
	case "text":
		if value := strings.TrimSpace(element.Content.Text); value != "" {
			return value
		}
		if value := strings.TrimSpace(element.Content.Markdown); value != "" {
			return value
		}
		if value := strings.TrimSpace(element.Content.HTML); value != "" {
			return convertUpstageHTMLFragmentToMessage(element.Category, value)
		}
	case "html":
		if value := strings.TrimSpace(element.Content.HTML); value != "" {
			return convertUpstageHTMLFragmentToMessage(element.Category, value)
		}
		if value := strings.TrimSpace(element.Content.Markdown); value != "" {
			return value
		}
		if value := strings.TrimSpace(element.Content.Text); value != "" {
			return value
		}
	}

	return ""
}

func convertUpstageHTMLFragmentToMessage(category, fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}

	if strings.Contains(strings.ToLower(fragment), "<table") {
		return strings.TrimSpace(normalizeUpstageHTMLLineBreaks(fragment))
	}

	text := normalizeUpstageHTMLLineBreaks(fragment)
	text = upstageHTMLTagPattern.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	text = normalizeUpstagePlainText(text)
	if text == "" {
		return ""
	}

	switch normalizeUpstageCategory(category) {
	case "heading1":
		return "# " + text
	case "heading2":
		return "## " + text
	case "heading3":
		return "### " + text
	case "heading4":
		return "#### " + text
	case "heading5":
		return "##### " + text
	case "heading6":
		return "###### " + text
	case "caption":
		return "_" + text + "_"
	default:
		return text
	}
}

func normalizeUpstageHTMLLineBreaks(value string) string {
	replacements := []string{
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n",
		"</div>", "\n",
		"</section>", "\n",
		"</article>", "\n",
		"</header>", "\n",
		"</footer>", "\n",
		"</figure>", "\n",
		"</figcaption>", "\n",
		"</caption>", "\n",
		"</li>", "\n",
		"</h1>", "\n",
		"</h2>", "\n",
		"</h3>", "\n",
		"</h4>", "\n",
		"</h5>", "\n",
		"</h6>", "\n",
	}
	replacer := strings.NewReplacer(replacements...)
	return replacer.Replace(value)
}

func normalizeUpstagePlainText(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	normalized := make([]string, 0, len(lines))
	previousBlank := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if previousBlank {
				continue
			}
			normalized = append(normalized, "")
			previousBlank = true
			continue
		}

		normalized = append(normalized, line)
		previousBlank = false
	}

	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

func normalizeUpstageCategory(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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

func resolveUpstageRequestTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return time.Duration(defaultTimeoutSeconds) * time.Second
	}
	return value
}

func cloneUpstageFields(fields map[string]string) map[string]string {
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func buildUpstageRequestDebug(service upstageServiceConfig, fields map[string]string, attachment botAttachment, correlationID, attempt string) upstageRequestDebug {
	copiedFields := make(map[string]string, len(fields))
	for key, value := range fields {
		copiedFields[key] = value
	}

	extension := ""
	if index := strings.LastIndex(strings.TrimSpace(attachment.Name), "."); index >= 0 {
		extension = strings.ToLower(strings.TrimSpace(attachment.Name[index:]))
	}

	return upstageRequestDebug{
		URL:      strings.TrimSpace(service.BaseURL),
		AuthMode: strings.TrimSpace(service.AuthMode),
		Fields:   copiedFields,
		Attachment: upstageAttachmentDebug{
			Name:      sanitizeUploadFilename(attachment.Name),
			MIMEType:  strings.TrimSpace(attachment.MIMEType),
			Extension: extension,
			Size:      int64(len(attachment.Content)),
		},
		Correlation: strings.TrimSpace(correlationID),
		Attempt:     strings.TrimSpace(attempt),
	}
}

func buildUpstageResponseDebug(statusCode int, headers http.Header, body []byte, callErr *upstageCallError) upstageResponseDebug {
	responseDebug := upstageResponseDebug{
		StatusCode: statusCode,
		RequestID:  firstHeaderValue(headers, "X-Request-Id", "X-Request-ID", "X-Correlation-ID"),
		Body:       formatUpstageDebugBody(body),
	}
	if callErr != nil {
		responseDebug.ErrorCode = strings.TrimSpace(callErr.Code)
		responseDebug.Summary = strings.TrimSpace(callErr.Summary)
		responseDebug.Detail = strings.TrimSpace(callErr.Detail)
		responseDebug.Hint = strings.TrimSpace(callErr.Hint)
	}
	return responseDebug
}

func formatUpstageDebugBody(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}

	var payload any
	if err := json.Unmarshal(trimmed, &payload); err == nil {
		pretty, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr == nil {
			return truncateString(string(pretty), 16*1024)
		}
	}

	return truncateString(string(trimmed), 16*1024)
}

func attachUpstageDebug(err error, requestDebug upstageRequestDebug, responseDebug upstageResponseDebug) error {
	if err == nil {
		return nil
	}

	var callErr *upstageCallError
	if errors.As(err, &callErr) {
		return callErr.withDebug(requestDebug, responseDebug)
	}
	return err
}

func attachUpstageAttemptDebug(err error, requestDebugs []upstageRequestDebug) error {
	if err == nil {
		return nil
	}

	var callErr *upstageCallError
	if !errors.As(err, &callErr) {
		return err
	}

	copyErr := *callErr
	copyErr.InputDebug = marshalUpstageRequestDebugs(requestDebugs)
	return &copyErr
}

func (e *upstageCallError) withDebug(requestDebug upstageRequestDebug, responseDebug upstageResponseDebug) *upstageCallError {
	if e == nil {
		return nil
	}

	copyErr := *e
	copyErr.InputDebug = marshalUpstageDebugPayload(requestDebug)
	copyErr.OutputDebug = marshalUpstageDebugPayload(responseDebug)
	return &copyErr
}

func marshalUpstageDebugPayload(payload any) string {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	return string(raw)
}

func marshalUpstageRequestDebugs(requestDebugs []upstageRequestDebug) string {
	switch len(requestDebugs) {
	case 0:
		return ""
	case 1:
		return marshalUpstageDebugPayload(requestDebugs[0])
	default:
		return marshalUpstageDebugPayload(requestDebugs)
	}
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

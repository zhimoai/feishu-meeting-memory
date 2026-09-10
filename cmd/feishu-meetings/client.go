package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type apiError struct {
	Status  int
	Code    any
	Message string
	Path    string
	Details any
}

func (e *apiError) Error() string {
	parts := make([]string, 0, 2)
	if e.Code != nil {
		parts = append(parts, fmt.Sprintf("code=%v", e.Code))
	}
	if e.Status != 0 {
		parts = append(parts, fmt.Sprintf("http=%d", e.Status))
	}
	if len(parts) == 0 {
		return e.Message
	}
	return strings.Join(parts, ", ") + ": " + e.Message
}

func numericCode(value any) int {
	switch code := value.(type) {
	case float64:
		return int(code)
	case json.Number:
		parsed, _ := strconv.Atoi(code.String())
		return parsed
	case string:
		parsed, _ := strconv.Atoi(code)
		return parsed
	case int:
		return code
	default:
		return 0
	}
}

func (e *apiError) Retryable() bool {
	code := numericCode(e.Code)
	return code == processingCode || e.Status == 429 || e.Status >= 500
}

func (e *apiError) PermissionError() bool {
	code := numericCode(e.Code)
	return code == noMinutePermissionCode || code == missingScopeCode || code == missingUserScopeCode || e.Status == 401 || e.Status == 403
}

func (e *apiError) Map() map[string]any {
	result := map[string]any{
		"type":             "feishu_api_error",
		"message":          e.Message,
		"path":             e.Path,
		"retryable":        e.Retryable(),
		"permission_error": e.PermissionError(),
	}
	if e.Status != 0 {
		result["http_status"] = e.Status
	}
	if e.Code != nil {
		result["code"] = e.Code
	}
	if e.Details != nil {
		result["details"] = e.Details
	}
	return result
}

type client struct {
	baseURL               string
	token                 string
	tokenSource           string
	appID                 string
	appSecret             string
	userOpenID            string
	configPath            string
	configLoaded          bool
	tokenExpiresAt        string
	refreshTokenAvailable bool
	tokenAutoRefreshed    bool
	oauthScope            string
	httpClient            *http.Client
}

func safeAPIBase(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", configurationError("FEISHU_API_BASE 无效")
	}
	if parsed.Scheme == "https" {
		return value, nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return value, nil
	}
	return "", configurationError("FEISHU_API_BASE 必须是 HTTPS；只有本机测试地址允许 HTTP")
}

func newClientFromEnv() (*client, error) {
	return newClientFromEnvWithRefresh(true)
}

func newClientFromEnvWithRefresh(allowRefresh bool) (*client, error) {
	settings, err := loadSettings()
	if err != nil {
		return nil, err
	}
	baseValue := settings.APIBase
	if baseValue == "" {
		baseValue = defaultAPIBase
	}
	baseURL, err := safeAPIBase(baseValue)
	if err != nil {
		return nil, err
	}
	timeout := settings.HTTPTimeoutSeconds
	if timeout < 1 || timeout > 300 {
		return nil, configurationError("FEISHU_HTTP_TIMEOUT 必须是 1 到 300 的整数秒")
	}
	httpClient := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	autoRefreshed := false
	if allowRefresh {
		settings, autoRefreshed, err = refreshUserAccessTokenIfNeeded(settings, httpClient)
		if err != nil {
			return nil, err
		}
	}
	result := &client{
		baseURL: baseURL, appID: settings.AppID, appSecret: settings.AppSecret,
		userOpenID: settings.UserOpenID, configPath: settings.ConfigPath, configLoaded: settings.ConfigLoaded,
		tokenExpiresAt: settings.UserTokenExpiresAt, refreshTokenAvailable: settings.RefreshToken != "",
		tokenAutoRefreshed: autoRefreshed, oauthScope: settings.OAuthScope, httpClient: httpClient,
	}
	switch {
	case settings.UserAccessToken != "":
		result.token = settings.UserAccessToken
		result.tokenSource = "user_access_token"
	case settings.TenantAccessToken != "":
		result.token = settings.TenantAccessToken
		result.tokenSource = "tenant_access_token"
	case settings.AccessToken != "":
		result.token = settings.AccessToken
		result.tokenSource = "explicit_access_token"
	case result.appID != "" && result.appSecret != "":
		result.tokenSource = "app_credentials"
	case result.appID != "" || result.appSecret != "":
		return nil, configurationError("FEISHU_APP_ID 与 FEISHU_APP_SECRET 必须同时配置")
	default:
		return nil, configurationError("请在 %s 配置 app_id/app_secret，或提供用户访问令牌", settings.ConfigPath)
	}
	return result, nil
}

func apiMessage(payload map[string]any, fallback string) string {
	for _, key := range []string{"msg", "message", "error_description", "error"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return fallback
}

func decodeJSON(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, validationError("飞书返回了无法解析的 JSON: %v", err)
	}
	return payload, nil
}

func checkAPIPayload(payload map[string]any, status int, path string) error {
	code, exists := payload["code"]
	if !exists || numericCode(code) == 0 {
		return nil
	}
	details := payload["error"]
	if details == nil {
		details = payload["data"]
	}
	return &apiError{Status: status, Code: code, Message: apiMessage(payload, "飞书 API 调用失败"), Path: path, Details: details}
}

func (c *client) ensureAccessToken() (string, error) {
	if c.token != "" {
		return c.token, nil
	}
	if c.appID == "" || c.appSecret == "" {
		return "", configurationError("没有可用的访问令牌或完整应用凭据")
	}
	path := "/open-apis/auth/v3/tenant_access_token/internal"
	body, _ := json.Marshal(map[string]string{"app_id": c.appID, "app_secret": c.appSecret})
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", validationError("无法创建鉴权请求: %v", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("User-Agent", userAgent)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", &apiError{Message: "网络连接失败: " + err.Error(), Path: path}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", &apiError{Status: response.StatusCode, Message: "读取鉴权响应失败: " + err.Error(), Path: path}
	}
	payload, err := decodeJSON(raw)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &apiError{Status: response.StatusCode, Code: payload["code"], Message: apiMessage(payload, response.Status), Path: path, Details: payload["error"]}
	}
	if err := checkAPIPayload(payload, response.StatusCode, path); err != nil {
		return "", err
	}
	token, _ := payload["tenant_access_token"].(string)
	if token == "" {
		return "", &apiError{Status: response.StatusCode, Message: "取 token 成功但响应中没有 tenant_access_token", Path: path}
	}
	c.token = token
	return token, nil
}

func (c *client) request(method, path string, query url.Values, body any) ([]byte, string, int, error) {
	token, err := c.ensureAccessToken()
	if err != nil {
		return nil, "", 0, err
	}
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, "", 0, validationError("请求 JSON 编码失败: %v", marshalErr)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		return nil, "", 0, validationError("无法创建 API 请求: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json, text/plain, application/octet-stream")
	request.Header.Set("User-Agent", userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, "", 0, &apiError{Message: "网络请求超时", Path: path}
		}
		return nil, "", 0, &apiError{Message: "网络连接失败: " + err.Error(), Path: path}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, "", response.StatusCode, &apiError{Status: response.StatusCode, Message: "读取 API 响应失败: " + err.Error(), Path: path}
	}
	contentType := response.Header.Get("Content-Type")
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if strings.Contains(strings.ToLower(contentType), "json") || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			if payload, parseErr := decodeJSON(raw); parseErr == nil {
				return nil, contentType, response.StatusCode, &apiError{Status: response.StatusCode, Code: payload["code"], Message: apiMessage(payload, response.Status), Path: path, Details: payload["error"]}
			}
		}
		return nil, contentType, response.StatusCode, &apiError{Status: response.StatusCode, Message: response.Status, Path: path}
	}
	return raw, contentType, response.StatusCode, nil
}

func (c *client) requestJSON(method, path string, query url.Values, body any) (map[string]any, error) {
	raw, _, status, err := c.request(method, path, query, body)
	if err != nil {
		return nil, err
	}
	payload, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	if err := checkAPIPayload(payload, status, path); err != nil {
		return nil, err
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data, nil
	}
	return payload, nil
}

func (c *client) requestBytes(method, path string, query url.Values) ([]byte, error) {
	raw, contentType, status, err := c.request(method, path, query, nil)
	if err != nil {
		return nil, err
	}
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "json") {
		return raw, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if contentType == "" && !bytes.HasPrefix(trimmed, []byte("{")) {
		return raw, nil
	}
	payload, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	if err := checkAPIPayload(payload, status, path); err != nil {
		return nil, err
	}
	data := payload["data"]
	if text, ok := data.(string); ok {
		return []byte(text), nil
	}
	if object, ok := data.(map[string]any); ok {
		for _, key := range []string{"transcript", "content", "text"} {
			if text, ok := object[key].(string); ok {
				return []byte(text), nil
			}
		}
	}
	return nil, &apiError{Status: status, Message: "接口未返回可保存的文本内容", Path: path, Details: data}
}

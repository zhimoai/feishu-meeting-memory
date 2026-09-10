package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOAuthBase        = "https://accounts.feishu.cn"
	defaultOAuthRedirectURI = "http://127.0.0.1:8080/callback"
	defaultOAuthScope       = "space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read"
	fullOAuthScope          = "minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve"
	tokenRefreshSkew        = 5 * time.Minute
)

type oauthCallback struct {
	code string
	err  error
}

type oauthTokenResult struct {
	accessToken         string
	expiresIn           int64
	refreshToken        string
	refreshTokenExpires int64
	scope               string
}

func randomURLSafe(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", validationError("无法生成 OAuth 随机值: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func oauthAuthorizationURL(base, appID, redirectURI, scope, state, challenge string) (string, error) {
	baseURL, err := safeAPIBase(base)
	if err != nil {
		return "", configurationError("oauth_base 无效: %v", err)
	}
	query := url.Values{
		"client_id":     {appID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {scope},
		"state":         {state},
		"prompt":        {"consent"},
	}
	if challenge != "" {
		query.Set("code_challenge", challenge)
		query.Set("code_challenge_method", "S256")
	}
	return baseURL + "/open-apis/authen/v1/authorize?" + query.Encode(), nil
}

func validateLoopbackRedirect(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return nil, configurationError("oauth_redirect_uri 必须是本机 HTTP 回调地址")
	}
	ip := net.ParseIP(parsed.Hostname())
	if parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, configurationError("oauth_redirect_uri 只允许 localhost、127.0.0.1 或 ::1")
	}
	if parsed.Port() == "" {
		return nil, configurationError("oauth_redirect_uri 必须包含固定端口")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, configurationError("oauth_redirect_uri 不能包含查询参数或片段")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed, nil
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return validationError("无法自动打开浏览器: %v", err)
	}
	return nil
}

func int64Field(payload map[string]any, key string) int64 {
	switch value := payload[key].(type) {
	case json.Number:
		parsed, _ := strconv.ParseInt(value.String(), 10, 64)
		return parsed
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

func exchangeOAuthCode(httpClient *http.Client, oauthBase, appID, appSecret, code, redirectURI, verifier, scope string) (oauthTokenResult, error) {
	baseURL, err := safeAPIBase(oauthBase)
	if err != nil {
		return oauthTokenResult{}, configurationError("oauth_base 无效: %v", err)
	}
	path := "/oauth/v3/token"
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {appID},
		"client_secret": {appSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	if verifier != "" {
		form.Set("code_verifier", verifier)
	}
	if scope != "" {
		form.Set("scope", scope)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResult{}, validationError("无法创建 OAuth 换取令牌请求: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := httpClient.Do(request)
	if err != nil {
		return oauthTokenResult{}, &apiError{Message: "OAuth 网络连接失败: " + err.Error(), Path: path}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return oauthTokenResult{}, &apiError{Status: response.StatusCode, Message: "读取 OAuth 响应失败: " + err.Error(), Path: path}
	}
	payload, err := decodeJSON(raw)
	if err != nil {
		return oauthTokenResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || numericCode(payload["code"]) != 0 {
		return oauthTokenResult{}, &apiError{
			Status: response.StatusCode, Code: payload["code"],
			Message: apiMessage(payload, response.Status), Path: path, Details: payload["error"],
		}
	}
	accessToken, _ := payload["access_token"].(string)
	if accessToken == "" {
		return oauthTokenResult{}, &apiError{Status: response.StatusCode, Message: "OAuth 成功响应缺少 access_token", Path: path}
	}
	refreshToken, _ := payload["refresh_token"].(string)
	grantedScope, _ := payload["scope"].(string)
	return oauthTokenResult{
		accessToken: accessToken, expiresIn: int64Field(payload, "expires_in"),
		refreshToken: refreshToken, refreshTokenExpires: int64Field(payload, "refresh_token_expires_in"),
		scope: grantedScope,
	}, nil
}

func refreshOAuthToken(httpClient *http.Client, oauthBase, appID, appSecret, refreshToken string) (oauthTokenResult, error) {
	baseURL, err := safeAPIBase(oauthBase)
	if err != nil {
		return oauthTokenResult{}, configurationError("oauth_base 无效: %v", err)
	}
	path := "/oauth/v3/token"
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {appID},
		"client_secret": {appSecret},
		"refresh_token": {refreshToken},
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResult{}, validationError("无法创建 OAuth 刷新请求: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	response, err := httpClient.Do(request)
	if err != nil {
		return oauthTokenResult{}, &apiError{Message: "OAuth 刷新网络连接失败: " + err.Error(), Path: path}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return oauthTokenResult{}, &apiError{Status: response.StatusCode, Message: "读取 OAuth 刷新响应失败: " + err.Error(), Path: path}
	}
	payload, err := decodeJSON(raw)
	if err != nil {
		return oauthTokenResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || numericCode(payload["code"]) != 0 {
		return oauthTokenResult{}, &apiError{
			Status: response.StatusCode, Code: payload["code"],
			Message: apiMessage(payload, response.Status), Path: path, Details: payload["error"],
		}
	}
	accessToken, _ := payload["access_token"].(string)
	if accessToken == "" {
		return oauthTokenResult{}, &apiError{Status: response.StatusCode, Message: "OAuth 刷新响应缺少 access_token", Path: path}
	}
	rotatedRefreshToken, _ := payload["refresh_token"].(string)
	grantedScope, _ := payload["scope"].(string)
	return oauthTokenResult{
		accessToken: accessToken, expiresIn: int64Field(payload, "expires_in"),
		refreshToken: rotatedRefreshToken, refreshTokenExpires: int64Field(payload, "refresh_token_expires_in"),
		scope: grantedScope,
	}, nil
}

func appendOAuthScopes(scope string, additions ...string) string {
	seen := map[string]bool{}
	ordered := make([]string, 0)
	for _, group := range append([]string{scope}, additions...) {
		for _, item := range strings.Fields(group) {
			if !seen[item] {
				seen[item] = true
				ordered = append(ordered, item)
			}
		}
	}
	return strings.Join(ordered, " ")
}

func removeOAuthScope(scope, unwanted string) string {
	kept := make([]string, 0)
	for _, item := range strings.Fields(scope) {
		if item != unwanted {
			kept = append(kept, item)
		}
	}
	return strings.Join(kept, " ")
}

func refreshUserAccessTokenIfNeeded(current settings, httpClient *http.Client) (settings, bool, error) {
	if current.UserAccessToken == "" || current.RefreshToken == "" || current.UserTokenExpiresAt == "" {
		return current, false, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, current.UserTokenExpiresAt)
	if err != nil {
		return current, false, configurationError("user_access_token_expires_at 不是有效的 RFC3339 时间；请重新运行 oauth-login")
	}
	if time.Until(expiresAt) > tokenRefreshSkew {
		return current, false, nil
	}
	if current.RefreshExpiresAt != "" {
		refreshExpiresAt, parseErr := time.Parse(time.RFC3339, current.RefreshExpiresAt)
		if parseErr != nil {
			return current, false, configurationError("refresh_token_expires_at 不是有效的 RFC3339 时间；请重新运行 oauth-login")
		}
		if !time.Now().Before(refreshExpiresAt) {
			return current, false, configurationError("个人授权已过期，请重新运行 oauth-login")
		}
	}
	if current.AppID == "" || current.AppSecret == "" {
		return current, false, configurationError("个人令牌即将或已经过期，但本机没有刷新所需的 app_id/app_secret；请重新授权")
	}
	oauthBase := current.OAuthBase
	if oauthBase == "" {
		oauthBase = defaultOAuthBase
	}
	token, err := refreshOAuthToken(httpClient, oauthBase, current.AppID, current.AppSecret, current.RefreshToken)
	if err != nil {
		return current, false, err
	}
	config, _, err := readFileConfig(current.ConfigPath)
	if err != nil {
		return current, false, err
	}
	now := time.Now().UTC()
	config.UserAccessToken = token.accessToken
	config.UserAccessTokenExpiresAt = now.Add(time.Duration(token.expiresIn) * time.Second).Format(time.RFC3339)
	if token.refreshToken != "" {
		config.RefreshToken = token.refreshToken
		current.RefreshToken = token.refreshToken
	}
	if token.refreshTokenExpires > 0 {
		config.RefreshTokenExpiresAt = now.Add(time.Duration(token.refreshTokenExpires) * time.Second).Format(time.RFC3339)
		current.RefreshExpiresAt = config.RefreshTokenExpiresAt
	}
	if token.scope != "" {
		config.OAuthScope = token.scope
		current.OAuthScope = token.scope
	}
	if err := writeFileConfig(current.ConfigPath, config); err != nil {
		return current, false, err
	}
	current.UserAccessToken = config.UserAccessToken
	current.UserTokenExpiresAt = config.UserAccessTokenExpiresAt
	return current, true, nil
}

func commandOAuthLogin(args []string) (any, error) {
	flags := newFlagSet("oauth-login")
	redirectFlag := flags.String("redirect-uri", "", "本机 OAuth 回调地址")
	scopeFlag := flags.String("scope", "", "空格分隔的飞书权限")
	offline := flags.Bool("offline", false, "兼容参数；个人模式默认请求 offline_access")
	noRefresh := flags.Bool("no-refresh", false, "不请求 refresh_token；仅用于临时测试")
	full := flags.Bool("full", false, "同时申请妙记详情、AI 产物、逐字稿与 Note 只读权限")
	pkce := flags.Bool("pkce", false, "启用 PKCE；本地自建应用默认使用 App Secret 与 state 校验")
	noBrowser := flags.Bool("no-browser", false, "不自动打开浏览器，仅打印授权地址")
	waitSeconds := flags.Int("wait-seconds", 300, "等待浏览器授权的秒数")
	if err := flags.Parse(args); err != nil {
		return nil, validationError("%v", err)
	}
	if flags.NArg() != 0 {
		return nil, validationError("oauth-login 不接受位置参数")
	}
	if *waitSeconds < 30 || *waitSeconds > 900 {
		return nil, validationError("--wait-seconds 必须在 30 到 900 之间")
	}

	settings, err := loadSettings()
	if err != nil {
		return nil, err
	}
	if settings.AppID == "" || settings.AppSecret == "" {
		return nil, configurationError("未找到应用凭据：请把 Skill 根目录的 config.example.json 复制为 config.json，并填写 app_id 与 app_secret（当前读取路径：%s）", settings.ConfigPath)
	}
	oauthBase := strings.TrimSpace(settings.OAuthBase)
	if oauthBase == "" {
		oauthBase = defaultOAuthBase
	}
	redirectURI := strings.TrimSpace(*redirectFlag)
	if redirectURI == "" {
		redirectURI = strings.TrimSpace(settings.OAuthRedirectURI)
	}
	if redirectURI == "" {
		redirectURI = defaultOAuthRedirectURI
	}
	parsedRedirect, err := validateLoopbackRedirect(redirectURI)
	if err != nil {
		return nil, err
	}
	scope := strings.TrimSpace(*scopeFlag)
	if scope == "" {
		scope = defaultOAuthScope
	}
	if *full {
		scope = appendOAuthScopes(scope, fullOAuthScope)
	}
	if *noRefresh {
		scope = removeOAuthScope(scope, "offline_access")
	} else if *offline || !*noRefresh {
		scope = appendOAuthScopes(scope, "offline_access")
	}

	state, err := randomURLSafe(24)
	if err != nil {
		return nil, err
	}
	verifier := ""
	challenge := ""
	if *pkce {
		verifier, err = randomURLSafe(48)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(verifier))
		challenge = base64.RawURLEncoding.EncodeToString(digest[:])
	}
	authorizeURL, err := oauthAuthorizationURL(oauthBase, settings.AppID, redirectURI, scope, state, challenge)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", parsedRedirect.Host)
	if err != nil {
		return nil, configurationError("无法监听 OAuth 回调 %s: %v", redirectURI, err)
	}
	callbackChannel := make(chan oauthCallback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(parsedRedirect.Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		query := r.URL.Query()
		result := oauthCallback{}
		switch {
		case query.Get("state") != state:
			result.err = validationError("OAuth state 校验失败，请重新登录")
		case query.Get("error") != "":
			result.err = validationError("用户未授权: %s", query.Get("error"))
		case query.Get("code") == "":
			result.err = validationError("OAuth 回调缺少授权码")
		default:
			result.code = query.Get("code")
		}
		select {
		case callbackChannel <- result:
		default:
		}
		if result.err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "<h2>飞书授权未完成</h2><p>请关闭此页面，并回到终端查看原因。</p>")
			return
		}
		_, _ = io.WriteString(w, "<h2>飞书授权成功</h2><p>可以关闭此页面，回到 Codex 继续查询会议。</p>")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	fmt.Fprintln(os.Stderr, "请在浏览器完成飞书授权；若浏览器未自动打开，请访问：")
	fmt.Fprintln(os.Stderr, authorizeURL)
	if !*noBrowser {
		if err := openBrowser(authorizeURL); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}

	var callback oauthCallback
	select {
	case callback = <-callbackChannel:
		if callback.err != nil {
			return nil, callback.err
		}
	case <-time.After(time.Duration(*waitSeconds) * time.Second):
		return nil, validationError("等待飞书授权超时；请重新运行 oauth-login")
	}

	httpClient := &http.Client{Timeout: time.Duration(settings.HTTPTimeoutSeconds) * time.Second}
	token, err := exchangeOAuthCode(httpClient, oauthBase, settings.AppID, settings.AppSecret, callback.code, redirectURI, verifier, scope)
	if err != nil {
		return nil, err
	}
	config, _, err := readFileConfig(settings.ConfigPath)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	config.UserAccessToken = token.accessToken
	config.UserAccessTokenExpiresAt = now.Add(time.Duration(token.expiresIn) * time.Second).Format(time.RFC3339)
	config.RefreshToken = token.refreshToken
	if token.refreshToken != "" && token.refreshTokenExpires > 0 {
		config.RefreshTokenExpiresAt = now.Add(time.Duration(token.refreshTokenExpires) * time.Second).Format(time.RFC3339)
	} else {
		config.RefreshTokenExpiresAt = ""
	}
	config.OAuthScope = token.scope
	if config.OAuthScope == "" {
		config.OAuthScope = scope
	}
	config.OAuthRedirectURI = redirectURI
	config.OAuthBase = oauthBase
	if err := writeFileConfig(settings.ConfigPath, config); err != nil {
		return nil, err
	}
	return map[string]any{
		"authorized": true, "config_file": settings.ConfigPath,
		"token_expires_at":    config.UserAccessTokenExpiresAt,
		"refresh_token_saved": token.refreshToken != "", "scope": config.OAuthScope,
		"secrets_printed": false,
	}, nil
}

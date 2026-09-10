package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxConfigBytes = 64 * 1024

type fileConfig struct {
	AppID                    string `json:"app_id,omitempty"`
	AppSecret                string `json:"app_secret,omitempty"`
	UserAccessToken          string `json:"user_access_token,omitempty"`
	UserAccessTokenExpiresAt string `json:"user_access_token_expires_at,omitempty"`
	RefreshToken             string `json:"refresh_token,omitempty"`
	RefreshTokenExpiresAt    string `json:"refresh_token_expires_at,omitempty"`
	OAuthScope               string `json:"oauth_scope,omitempty"`
	OAuthRedirectURI         string `json:"oauth_redirect_uri,omitempty"`
	OAuthBase                string `json:"oauth_base,omitempty"`
	TenantAccessToken        string `json:"tenant_access_token,omitempty"`
	AccessToken              string `json:"access_token,omitempty"`
	UserOpenID               string `json:"user_open_id,omitempty"`
	APIBase                  string `json:"api_base,omitempty"`
	HTTPTimeoutSeconds       int    `json:"http_timeout_seconds,omitempty"`
}

type settings struct {
	ConfigPath         string
	ConfigLoaded       bool
	AppID              string
	AppSecret          string
	UserAccessToken    string
	UserTokenExpiresAt string
	RefreshToken       string
	RefreshExpiresAt   string
	OAuthScope         string
	OAuthRedirectURI   string
	OAuthBase          string
	TenantAccessToken  string
	AccessToken        string
	UserOpenID         string
	APIBase            string
	HTTPTimeoutSeconds int
}

func defaultConfigPath() (string, error) {
	executable, executableErr := os.Executable()
	workingDirectory, workingErr := os.Getwd()
	if executableErr == nil {
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}
		if path, ok := skillConfigPath(executable); ok {
			return path, nil
		}
	}
	if workingErr != nil {
		return "", configurationError("无法确定 Skill 配置文件路径: %v", workingErr)
	}
	return filepath.Join(workingDirectory, "config.json"), nil
}

func skillConfigPath(executable string) (string, bool) {
	platformDirectory := filepath.Dir(executable)
	binDirectory := filepath.Dir(platformDirectory)
	if filepath.Base(binDirectory) != "bin" {
		return "", false
	}
	return filepath.Join(filepath.Dir(binDirectory), "config.json"), true
}

func configPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("FEISHU_CONFIG_FILE")); explicit != "" {
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", configurationError("FEISHU_CONFIG_FILE 无效: %v", err)
		}
		return absolute, nil
	}
	return defaultConfigPath()
}

func readFileConfig(path string) (fileConfig, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileConfig{}, false, nil
	}
	if err != nil {
		return fileConfig{}, false, configurationError("无法读取配置文件 %s: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fileConfig{}, false, configurationError("无法检查配置文件 %s: %v", path, err)
	}
	if info.Size() > maxConfigBytes {
		return fileConfig{}, false, configurationError("配置文件超过 64 KiB 安全上限: %s", path)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return fileConfig{}, false, configurationError("无法读取配置文件 %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var config fileConfig
	if err := decoder.Decode(&config); err != nil {
		return fileConfig{}, false, configurationError("配置文件 JSON 无效 %s: %v", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fileConfig{}, false, configurationError("配置文件只能包含一个 JSON 对象: %s", path)
	}
	return config, true, nil
}

func envOrConfig(key, configValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return strings.TrimSpace(configValue)
}

func loadSettings() (settings, error) {
	path, err := configPath()
	if err != nil {
		return settings{}, err
	}
	config, loaded, err := readFileConfig(path)
	if err != nil {
		return settings{}, err
	}
	timeout := config.HTTPTimeoutSeconds
	if raw := strings.TrimSpace(os.Getenv("FEISHU_HTTP_TIMEOUT")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return settings{}, configurationError("FEISHU_HTTP_TIMEOUT 必须是整数秒")
		}
		timeout = parsed
	}
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	}
	userTokenFromEnvironment := strings.TrimSpace(os.Getenv("FEISHU_USER_ACCESS_TOKEN")) != ""
	userAccessToken := envOrConfig("FEISHU_USER_ACCESS_TOKEN", config.UserAccessToken)
	userTokenExpiresAt := config.UserAccessTokenExpiresAt
	refreshToken := config.RefreshToken
	refreshExpiresAt := config.RefreshTokenExpiresAt
	if userTokenFromEnvironment {
		userTokenExpiresAt = ""
		refreshToken = ""
		refreshExpiresAt = ""
	}
	return settings{
		ConfigPath:         path,
		ConfigLoaded:       loaded,
		AppID:              envOrConfig("FEISHU_APP_ID", config.AppID),
		AppSecret:          envOrConfig("FEISHU_APP_SECRET", config.AppSecret),
		UserAccessToken:    userAccessToken,
		UserTokenExpiresAt: strings.TrimSpace(userTokenExpiresAt),
		RefreshToken:       strings.TrimSpace(refreshToken),
		RefreshExpiresAt:   strings.TrimSpace(refreshExpiresAt),
		OAuthScope:         strings.TrimSpace(config.OAuthScope),
		OAuthRedirectURI:   envOrConfig("FEISHU_OAUTH_REDIRECT_URI", config.OAuthRedirectURI),
		OAuthBase:          envOrConfig("FEISHU_OAUTH_BASE", config.OAuthBase),
		TenantAccessToken:  envOrConfig("FEISHU_TENANT_ACCESS_TOKEN", config.TenantAccessToken),
		AccessToken:        envOrConfig("FEISHU_ACCESS_TOKEN", config.AccessToken),
		UserOpenID:         envOrConfig("FEISHU_USER_OPEN_ID", config.UserOpenID),
		APIBase:            envOrConfig("FEISHU_API_BASE", config.APIBase),
		HTTPTimeoutSeconds: timeout,
	}, nil
}

func writeFileConfig(path string, config fileConfig) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return configurationError("无法创建配置目录 %s: %v", directory, err)
	}
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return configurationError("无法编码配置文件: %v", err)
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return configurationError("无法写入配置文件 %s: %v", path, err)
	}
	if _, err = file.Write(raw); err != nil {
		_ = file.Close()
		return configurationError("无法写入配置文件 %s: %v", path, err)
	}
	if err = file.Close(); err != nil {
		return configurationError("无法关闭配置文件 %s: %v", path, err)
	}
	if err = os.Chmod(path, 0o600); err != nil && os.PathSeparator != '\\' {
		return configurationError("无法设置配置文件权限 %s: %v", path, err)
	}
	return nil
}

func configuredUserOpenID() (string, error) {
	settings, err := loadSettings()
	if err != nil {
		return "", err
	}
	return settings.UserOpenID, nil
}

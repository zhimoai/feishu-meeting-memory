package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

var (
	minuteURLPattern  = regexp.MustCompile(`/minutes/([A-Za-z0-9_-]+)`)
	resourcePattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,300}$`)
	openIDPattern     = regexp.MustCompile(`^ou_[A-Za-z0-9_-]+$`)
	contactHighlight  = regexp.MustCompile(`(?i)</?h>`)
	meetingDateSuffix = regexp.MustCompile(`\s+\d{4}年\d{1,2}月\d{1,2}日\s*$`)
)

func sanitizeContactSearchItem(item any) any {
	object, ok := item.(map[string]any)
	if !ok {
		return item
	}
	result := map[string]any{"open_id": object["id"], "content_is_untrusted": true}
	metadata, _ := object["meta_data"].(map[string]any)
	if metadata != nil {
		result["localized_names"] = metadata["i18n_names"]
		result["is_registered"] = metadata["is_registered"]
		result["is_cross_tenant"] = metadata["is_cross_tenant"]
	}
	if display, ok := object["display_info"].(string); ok {
		cleaned := html.UnescapeString(contactHighlight.ReplaceAllString(display, ""))
		lines := strings.Split(cleaned, "\n")
		for index := range lines {
			lines[index] = strings.TrimSpace(lines[index])
		}
		result["display_lines"] = lines
		if len(lines) > 1 && lines[1] != "" {
			result["department_hint"] = lines[1]
		}
	}
	return result
}

type timeWindow struct {
	Start *time.Time
	End   *time.Time
}

func parseDateTime(value string, endOfDay bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, validationError("时间不能为空")
	}
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, value); matched {
		parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
		if err != nil {
			return time.Time{}, validationError("无效时间 %q", value)
		}
		if endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Second)
		}
		return parsed, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		var parsed time.Time
		var err error
		if layout == time.RFC3339 {
			parsed, err = time.Parse(layout, value)
		} else {
			parsed, err = time.ParseInLocation(layout, value, time.Local)
		}
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, validationError("无效时间 %q；请使用 YYYY-MM-DD 或 ISO 8601", value)
}

func ptrTime(value time.Time) *time.Time { return &value }

func buildSearchWindows(start, end *time.Time, now time.Time) ([]timeWindow, error) {
	if start != nil && end != nil && start.After(*end) {
		return nil, validationError("开始时间不能晚于结束时间")
	}
	if start != nil && end == nil && !start.After(now) {
		end = ptrTime(now)
	}
	if start == nil || end == nil {
		return []timeWindow{{Start: start, End: end}}, nil
	}
	const maxWindow = 30 * 24 * time.Hour
	if end.Sub(*start) <= maxWindow {
		return []timeWindow{{Start: start, End: end}}, nil
	}
	result := make([]timeWindow, 0)
	cursorEnd := *end
	for !cursorEnd.Before(*start) {
		cursorStart := cursorEnd.Add(-maxWindow)
		if cursorStart.Before(*start) {
			cursorStart = *start
		}
		startCopy, endCopy := cursorStart, cursorEnd
		result = append(result, timeWindow{Start: &startCopy, End: &endCopy})
		if cursorStart.Equal(*start) {
			break
		}
		cursorEnd = cursorStart.Add(-time.Second)
	}
	return result, nil
}

func timeString(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

func parseMinuteToken(value string) (string, error) {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		match := minuteURLPattern.FindStringSubmatch(parsed.Path)
		if len(match) != 2 {
			return "", validationError("URL 中没有找到 /minutes/<minute_token>")
		}
		value = match[1]
	}
	if !resourcePattern.MatchString(value) || len(value) < 3 || len(value) > 200 {
		return "", validationError("无效的 minute_token")
	}
	return value, nil
}

func parseResourceName(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if !resourcePattern.MatchString(value) {
		return "", validationError("无效的 %s", label)
	}
	return value, nil
}

func tokenFromMinuteURL(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	token, err := parseMinuteToken(text)
	if err != nil {
		return ""
	}
	return token
}

func flattenCSV(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				result = append(result, part)
			}
		}
	}
	return result
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func parseArtifacts(value string) (map[string]bool, error) {
	aliases := map[string]string{"todo": "todos", "chapter": "chapters", "keyword": "keywords"}
	allowed := map[string]bool{"summary": true, "todos": true, "chapters": true, "keywords": true, "transcript": true}
	result := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if alias := aliases[part]; alias != "" {
			part = alias
		}
		if part == "all" {
			for name := range allowed {
				result[name] = true
			}
			continue
		}
		if !allowed[part] {
			return nil, validationError("未知 artifacts：%s", part)
		}
		result[part] = true
	}
	return result, nil
}

func defaultArtifactPath(kind, token, name string) (string, error) {
	path := filepath.Join(os.TempDir(), "feishu-meeting-memory", kind, token, name)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", validationError("无法解析临时文件路径: %v", err)
	}
	return absolute, nil
}

func saveBytes(raw []byte, output string, overwrite, explicit bool) (string, error) {
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", validationError("无法解析输出路径: %v", err)
	}
	if _, err := os.Stat(absolute); err == nil && explicit && !overwrite {
		return "", validationError("输出文件已存在：%s；如需覆盖请传 --overwrite", absolute)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", validationError("无法创建输出目录: %v", err)
	}
	if err := os.WriteFile(absolute, raw, 0o600); err != nil {
		return "", validationError("无法保存输出文件: %v", err)
	}
	return absolute, nil
}

func previewText(raw []byte, limit int) (string, bool) {
	text := string(raw)
	runes := []rune(text)
	if len(runes) <= limit {
		return text, false
	}
	return string(runes[:limit]), true
}

type documentRef struct {
	Kind  string
	Token string
}

func parseDocumentRef(value string) (documentRef, error) {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		patterns := []struct {
			Pattern *regexp.Regexp
			Kind    string
		}{
			{regexp.MustCompile(`/docx/([A-Za-z0-9_-]+)`), "docx"},
			{regexp.MustCompile(`/wiki/([A-Za-z0-9_-]+)`), "wiki"},
			{regexp.MustCompile(`/(?:docs?|document)/([A-Za-z0-9_-]+)`), "doc"},
		}
		for _, candidate := range patterns {
			match := candidate.Pattern.FindStringSubmatch(parsed.Path)
			if len(match) == 2 {
				return documentRef{Kind: candidate.Kind, Token: match[1]}, nil
			}
		}
		return documentRef{}, validationError("无法从 URL 识别 Docx、Wiki 或旧版 Doc token")
	}
	token, err := parseResourceName(value, "document token")
	if err != nil {
		return documentRef{}, err
	}
	return documentRef{Kind: "docx", Token: token}, nil
}

func mapString(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func sliceAny(value any) []any {
	result, _ := value.([]any)
	return result
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return ""
}

func sortedKeys(input map[string]bool) []string {
	result := make([]string, 0, len(input))
	for key := range input {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func binaryNameForCurrentPlatform() string {
	name := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		return name + "/feishu-meetings.exe"
	}
	return name + "/feishu-meetings"
}

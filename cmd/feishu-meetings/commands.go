package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func newFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func parseFlagSet(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return validationError("请运行 feishu-meetings --help 查看命令说明")
		}
		return validationError("%s", err)
	}
	if len(set.Args()) > 0 {
		return validationError("无法识别的参数：%s", strings.Join(set.Args(), " "))
	}
	return nil
}

func commandPermissions(args []string) (any, error) {
	set := newFlagSet("permissions")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}
	return map[string]any{
		"profile": "personal_full_readonly",
		"required_oauth_scopes": strings.Fields(appendOAuthScopes(
			defaultOAuthScope, fullOAuthScope, "offline_access",
		)),
		"optional_oauth_scopes": map[string]any{
			"people_search":           []string{"contact:user:search"},
			"video_meeting_recording": []string{"vc:meeting.meetingevent:read", "vc:record:readonly"},
		},
		"redirect_uri":             defaultOAuthRedirectURI,
		"permissions_are_readonly": true,
		"publish_and_reauthorize_required_after_scope_change": true,
		"app_type_notice": "妙记接口可能仅支持企业自建应用；个人云盘 Docx 是默认主链路",
	}, nil
}

func missingOAuthScopes(granted string, required []string) []string {
	grantedSet := map[string]bool{}
	for _, scope := range strings.Fields(granted) {
		grantedSet[scope] = true
	}
	missing := make([]string, 0)
	for _, scope := range required {
		if !grantedSet[scope] {
			missing = append(missing, scope)
		}
	}
	return missing
}

func commandDoctor(args []string) (any, error) {
	set := newFlagSet("doctor")
	offline := set.Bool("offline", false, "只检查本地配置")
	probe := set.Bool("probe", false, "探测妙记与云文档搜索权限")
	minuteRaw := set.String("minute", "", "用于探测详情权限的测试妙记 token/URL")
	documentRaw := set.String("doc", "", "用于探测正文权限的测试文档 token/URL")
	probeTranscript := set.Bool("transcript", false, "同时探测测试妙记的逐字稿导出权限")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}
	if *offline && (*probe || strings.TrimSpace(*minuteRaw) != "" || strings.TrimSpace(*documentRaw) != "" || *probeTranscript) {
		return nil, validationError("--offline 不能与权限探测参数同时使用")
	}
	if *probeTranscript && strings.TrimSpace(*minuteRaw) == "" {
		return nil, validationError("--transcript 需要同时提供 --minute")
	}
	api, err := newClientFromEnvWithRefresh(!*offline)
	if err != nil {
		return nil, err
	}
	var authOK any
	if !*offline {
		if _, err := api.ensureAccessToken(); err != nil {
			return nil, err
		}
		authOK = true
	}
	probes := map[string]any{}
	probeCall := func(name string, call func() error) {
		if err := call(); err != nil {
			probes[name] = map[string]any{"ok": false, "error": errorPayload(err)}
			return
		}
		probes[name] = map[string]any{"ok": true}
	}
	if *probe {
		now := time.Now()
		start := now.Add(-24 * time.Hour)
		if api.tokenSource != "user_access_token" {
			probes["minutes_search"] = map[string]any{
				"ok": false, "skipped": true, "reason": "requires_user_access_token",
			}
		} else {
			probeCall("minutes_search", func() error {
				_, err := searchPage(api, "", nil, nil, timeWindow{Start: &start, End: &now}, 1, "")
				return err
			})
		}
		probeCall("cloud_documents_search", func() error {
			_, err := api.requestJSON(http.MethodPost, "/open-apis/search/v2/doc_wiki/search", nil, map[string]any{
				"query": "", "page_size": 1,
				"doc_filter":  map[string]any{"doc_types": []string{"DOCX"}, "only_title": true},
				"wiki_filter": map[string]any{"doc_types": []string{"WIKI"}, "only_title": true},
			})
			return err
		})
	}
	if strings.TrimSpace(*minuteRaw) != "" {
		token, parseErr := parseMinuteToken(*minuteRaw)
		if parseErr != nil {
			return nil, parseErr
		}
		path := "/open-apis/minutes/v1/minutes/" + url.PathEscape(token)
		probeCall("minute_basic", func() error {
			_, err := api.requestJSON(http.MethodGet, path, nil, nil)
			return err
		})
		probeCall("minute_artifacts", func() error {
			_, err := api.requestJSON(http.MethodGet, path+"/artifacts", nil, nil)
			return err
		})
		if *probeTranscript {
			probeCall("minute_transcript", func() error {
				_, err := api.requestBytes(http.MethodGet, path+"/transcript", url.Values{
					"need_speaker": []string{"false"}, "need_timestamp": []string{"false"}, "file_format": []string{"txt"},
				})
				return err
			})
		}
	}
	if strings.TrimSpace(*documentRaw) != "" {
		reference, parseErr := documentRefFrom(*documentRaw)
		if parseErr != nil {
			return nil, parseErr
		}
		probeCall("document_content", func() error {
			_, _, err := fetchDocumentContent(api, reference.Kind, reference.Token)
			return err
		})
	}
	deploymentScopes := strings.Fields(appendOAuthScopes(defaultOAuthScope, fullOAuthScope, "offline_access"))
	var missingDeploymentScopes any
	var authorizationProfileComplete any
	if api.oauthScope != "" {
		missing := missingOAuthScopes(api.oauthScope, deploymentScopes)
		missingDeploymentScopes = missing
		authorizationProfileComplete = len(missing) == 0
	}
	return map[string]any{
		"ok":                             true,
		"network_checked":                !*offline,
		"auth_ok":                        authOK,
		"token_source":                   api.tokenSource,
		"api_base":                       api.baseURL,
		"config_file":                    api.configPath,
		"config_loaded":                  api.configLoaded,
		"user_open_id_configured":        api.userOpenID != "",
		"user_token_expires_at":          emptyAsNil(api.tokenExpiresAt),
		"refresh_token_saved":            api.refreshTokenAvailable,
		"token_auto_refreshed":           api.tokenAutoRefreshed,
		"oauth_scope":                    emptyAsNil(api.oauthScope),
		"authorization_profile_complete": authorizationProfileComplete,
		"missing_deployment_scopes":      missingDeploymentScopes,
		"secrets_printed":                false,
		"runtime":                        "standalone_go_binary",
		"platform_binary":                binaryPathHint(),
		"identity_capabilities": map[string]any{
			"minutes_search":     api.tokenSource == "user_access_token",
			"direct_minute_read": true,
			"cloud_doc_search":   true,
		},
		"probes":                  probes,
		"probe_content_discarded": len(probes) > 0,
		"required_scopes": map[string]any{
			"authorization_refresh": []string{"offline_access"},
			"user_search":           []string{"contact:user:search"},
			"search":                []string{"minutes:minutes.search:read"},
			"docs_search":           []string{"search:docs:read"},
			"drive_list":            []string{"space:document:retrieve", "drive:drive:readonly"},
			"minute_basic":          []string{"minutes:minutes.basic:read"},
			"artifacts":             []string{"minutes:minutes.artifacts:read"},
			"transcript":            []string{"minutes:minutes.transcript:export"},
			"note":                  []string{"vc:note:read"},
			"docx":                  []string{"docx:document:readonly"},
			"wiki":                  []string{"wiki:node:retrieve"},
			"meeting_recording":     []string{"vc:meeting.meetingevent:read", "vc:record:readonly"},
		},
		"deployment_profile": deploymentScopes,
		"note":               "妙记搜索必须使用 user_access_token；tenant/app 身份只能从已知 token 开始读取获授权的妙记。scope 与单条妙记/文档 ACL 仍必须同时满足。",
	}, nil
}

type driveMeetingAccumulator struct {
	title          string
	latestCreated  int64
	latestModified int64
	hasSummary     bool
	resources      []any
}

func unixTimeField(item map[string]any, key string) int64 {
	raw := stringValue(item[key])
	value, _ := strconv.ParseInt(raw, 10, 64)
	return value
}

func meetingDocumentName(name string) (string, string, bool) {
	name = strings.TrimSpace(name)
	prefixes := []struct {
		prefix string
		kind   string
	}{
		{"智能纪要：", "summary"}, {"智能纪要:", "summary"},
		{"文字记录：", "transcript"}, {"文字记录:", "transcript"},
		{"我的笔记：", "notes"}, {"我的笔记:", "notes"},
	}
	for _, candidate := range prefixes {
		if !strings.HasPrefix(name, candidate.prefix) {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(name, candidate.prefix))
		title = meetingDateSuffix.ReplaceAllString(title, "")
		if title == "" {
			return "", "", false
		}
		return title, candidate.kind, true
	}
	return "", "", false
}

func formatUnixTime(value int64) any {
	if value <= 0 {
		return nil
	}
	return time.Unix(value, 0).In(time.Local).Format(time.RFC3339)
}

func commandDriveMeetings(args []string) (any, error) {
	return commandDriveMeetingsWithDefault(args, 0)
}

func commandPersonalMeetings(args []string) (any, error) {
	return commandDriveMeetingsWithDefault(args, 30)
}

func commandDriveMeetingsWithDefault(args []string, defaultDays int) (any, error) {
	set := newFlagSet("drive-meetings")
	dateRaw := set.String("date", "", "指定单日 YYYY-MM-DD")
	days := set.Int("days", defaultDays, "最近自然日数量")
	queryRaw := set.String("query", "", "按会议标题筛选")
	folderToken := set.String("folder-token", "", "指定文件夹 token；默认个人云盘根目录")
	pageSize := set.Int("page-size", 200, "文件夹分页大小")
	limit := set.Int("limit", 100, "会议结果上限")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}
	if *pageSize < 1 || *pageSize > 200 {
		return nil, validationError("--page-size 必须在 1 到 200 之间")
	}
	if *limit < 1 || *limit > 5000 {
		return nil, validationError("--limit 必须在 1 到 5000 之间")
	}
	if *days < 0 || *days > 366 {
		return nil, validationError("--days 必须在 0 到 366 之间")
	}
	if strings.TrimSpace(*dateRaw) != "" && *days > 0 {
		return nil, validationError("--date 与 --days 不能同时使用")
	}
	queryText := strings.TrimSpace(*queryRaw)
	if utf8.RuneCountInString(queryText) > 100 {
		return nil, validationError("--query 最多 100 个字符")
	}
	*folderToken = strings.TrimSpace(*folderToken)
	if *folderToken != "" && !resourcePattern.MatchString(*folderToken) {
		return nil, validationError("无效的 --folder-token")
	}
	dateText := strings.TrimSpace(*dateRaw)
	var start time.Time
	var end time.Time
	if dateText != "" {
		if len(dateText) != len("2006-01-02") {
			return nil, validationError("--date 必须使用 YYYY-MM-DD")
		}
		var err error
		start, err = parseDateTime(dateText, false)
		if err != nil {
			return nil, err
		}
		end = start.Add(24 * time.Hour)
	} else {
		if *days == 0 {
			*days = 1
		}
		now := time.Now().In(time.Local)
		end = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local)
		start = end.AddDate(0, 0, -*days)
		dateText = start.Format("2006-01-02")
	}

	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	if api.tokenSource != "user_access_token" {
		return nil, configurationError("个人云盘“我的文件夹”需要 user_access_token；当前身份是 %s", api.tokenSource)
	}

	groups := map[string]*driveMeetingAccumulator{}
	pageToken := ""
	filesSeen := 0
	pagesFetched := 0
	for page := 1; page <= maxDocSearchPages; page++ {
		query := url.Values{
			"page_size": []string{strconv.Itoa(*pageSize)},
			"order_by":  []string{"CreatedTime"},
			"direction": []string{"DESC"},
		}
		if *folderToken != "" {
			query.Set("folder_token", *folderToken)
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		data, requestErr := api.requestJSON(http.MethodGet, "/open-apis/drive/v1/files", query, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		pagesFetched++
		files := sliceAny(data["files"])
		filesSeen += len(files)
		pageOlderThanRange := false
		for _, raw := range files {
			item := mapString(raw)
			created := unixTimeField(item, "created_time")
			if created > 0 && created < start.Unix() {
				pageOlderThanRange = true
			}
			if created < start.Unix() || created >= end.Unix() {
				continue
			}
			title, kind, ok := meetingDocumentName(stringValue(item["name"]))
			if !ok {
				continue
			}
			if queryText != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(queryText)) {
				continue
			}
			key := strings.ToLower(title) + "|" + time.Unix(created, 0).In(time.Local).Format("2006-01-02")
			group := groups[key]
			if group == nil {
				group = &driveMeetingAccumulator{title: title, resources: make([]any, 0, 3)}
				groups[key] = group
			}
			modified := unixTimeField(item, "modified_time")
			if created > group.latestCreated {
				group.latestCreated = created
			}
			if modified > group.latestModified {
				group.latestModified = modified
			}
			if kind == "summary" {
				group.hasSummary = true
			}
			group.resources = append(group.resources, map[string]any{
				"kind": kind, "name": item["name"], "type": item["type"],
				"token": item["token"], "url": item["url"],
				"created_time": formatUnixTime(created), "modified_time": formatUnixTime(modified),
			})
		}
		hasMore, _ := data["has_more"].(bool)
		if !hasMore || pageOlderThanRange {
			break
		}
		next := stringValue(data["next_page_token"])
		if next == "" || next == pageToken {
			return nil, validationError("云盘文件清单返回 has_more，但分页 token 没有前进")
		}
		pageToken = next
	}

	meetings := make([]map[string]any, 0, len(groups))
	incomplete := make([]map[string]any, 0)
	for _, group := range groups {
		entry := map[string]any{
			"title": group.title, "created_time": formatUnixTime(group.latestCreated),
			"modified_time": formatUnixTime(group.latestModified), "resources": group.resources,
		}
		if group.hasSummary {
			meetings = append(meetings, entry)
		} else {
			entry["status"] = "summary_not_generated_or_orphaned_resource"
			incomplete = append(incomplete, entry)
		}
	}
	sort.Slice(meetings, func(i, j int) bool {
		return unixFromFormatted(meetings[i]["created_time"]) > unixFromFormatted(meetings[j]["created_time"])
	})
	sort.Slice(incomplete, func(i, j int) bool {
		return unixFromFormatted(incomplete[i]["created_time"]) > unixFromFormatted(incomplete[j]["created_time"])
	})
	truncated := len(meetings) > *limit
	if truncated {
		meetings = meetings[:*limit]
	}
	return map[string]any{
		"start_date": start.Format("2006-01-02"), "end_date": end.Add(-time.Nanosecond).Format("2006-01-02"),
		"query": queryText, "meetings": meetings, "count": len(meetings),
		"incomplete_recordings": incomplete, "incomplete_count": len(incomplete),
		"files_seen": filesSeen, "pages_fetched": pagesFetched, "truncated": truncated,
		"identity": api.tokenSource, "content_is_untrusted": true,
	}, nil
}

func emptyAsNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func unixFromFormatted(value any) int64 {
	text, _ := value.(string)
	parsed, _ := time.Parse(time.RFC3339, text)
	return parsed.Unix()
}

func commandUserSearch(args []string) (any, error) {
	set := newFlagSet("user-search")
	query := set.String("query", "", "姓名、邮箱或关键词")
	hasChatted := set.Bool("has-chatted", false, "只查当前用户联系过的人")
	excludeExternal := set.Bool("exclude-external", false, "排除外部联系人")
	pageSize := set.Int("page-size", 20, "每页数量")
	pageToken := set.String("page-token", "", "下一页 token")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}
	*query = strings.TrimSpace(*query)
	if *query == "" {
		return nil, validationError("user-search 需要 --query")
	}
	if utf8.RuneCountInString(*query) > 50 {
		return nil, validationError("--query 最多 50 个字符")
	}
	if *pageSize < 1 || *pageSize > 30 {
		return nil, validationError("--page-size 必须在 1 到 30 之间")
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	if api.tokenSource != "user_access_token" {
		return nil, configurationError("user-search 需要配置 user_access_token；应用/tenant 身份或类型不明确的 access_token 不能代替具体用户")
	}
	body := map[string]any{"query": *query}
	filter := map[string]any{}
	if *hasChatted {
		filter["has_contact"] = true
	}
	if *excludeExternal {
		filter["exclude_outer_contact"] = true
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	parameters := url.Values{"page_size": []string{strconv.Itoa(*pageSize)}}
	if strings.TrimSpace(*pageToken) != "" {
		parameters.Set("page_token", strings.TrimSpace(*pageToken))
	}
	data, err := api.requestJSON(http.MethodPost, "/open-apis/contact/v3/users/search", parameters, body)
	if err != nil {
		return nil, err
	}
	items := sliceAny(data["items"])
	users := make([]any, 0, len(items))
	for _, item := range items {
		users = append(users, sanitizeContactSearchItem(item))
	}
	return map[string]any{
		"users": users, "count": len(users), "has_more": data["has_more"], "page_token": data["page_token"],
		"notice": data["notice"], "identity": api.tokenSource, "personal_contact_fields_omitted": true,
	}, nil
}

func binaryPathHint() string {
	return "bin/" + binaryNameForCurrentPlatform()
}

func searchItemToken(item any) string {
	object, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	if token, ok := object["token"].(string); ok && token != "" {
		return token
	}
	if token, ok := object["minute_token"].(string); ok && token != "" {
		return token
	}
	if metadata, ok := object["meta_data"].(map[string]any); ok {
		return tokenFromMinuteURL(metadata["app_link"])
	}
	return ""
}

func sanitizeSearchItem(item any) any {
	object, ok := item.(map[string]any)
	if !ok {
		return item
	}
	cleaned := make(map[string]any, len(object))
	for key, value := range object {
		cleaned[key] = value
	}
	if metadata, ok := cleaned["meta_data"].(map[string]any); ok {
		metaCopy := make(map[string]any, len(metadata))
		for key, value := range metadata {
			if key != "avatar" {
				metaCopy[key] = value
			}
		}
		cleaned["meta_data"] = metaCopy
	}
	return cleaned
}

func searchPage(
	api *client,
	query string,
	owners, participants []string,
	window timeWindow,
	pageSize int,
	pageToken string,
) (map[string]any, error) {
	body := map[string]any{}
	if query != "" {
		body["query"] = query
	}
	filterBody := map[string]any{}
	if len(owners) > 0 {
		filterBody["owner_ids"] = owners
	}
	if len(participants) > 0 {
		filterBody["participant_ids"] = participants
	}
	if window.Start != nil || window.End != nil {
		createTime := map[string]string{}
		if window.Start != nil {
			createTime["start_time"] = window.Start.Format(time.RFC3339)
		}
		if window.End != nil {
			createTime["end_time"] = window.End.Format(time.RFC3339)
		}
		filterBody["create_time"] = createTime
	}
	if len(filterBody) > 0 {
		body["filter"] = filterBody
	}
	parameters := url.Values{"page_size": []string{strconv.Itoa(pageSize)}}
	if pageToken != "" {
		parameters.Set("page_token", pageToken)
	}
	return api.requestJSON(http.MethodPost, "/open-apis/minutes/v1/minutes/search", parameters, body)
}

type searchPlan struct {
	Owners       []string
	Participants []string
	Label        string
}

func searchMinutes(
	api *client,
	query string,
	owners, participants []string,
	mineOpenID string,
	windows []timeWindow,
	pageSize, limit int,
) ([]any, bool, []any, error) {
	plans := []searchPlan{{Owners: owners, Participants: participants, Label: "filters"}}
	if mineOpenID != "" {
		plans = []searchPlan{
			{Owners: []string{mineOpenID}, Label: "owned_by_me"},
			{Participants: []string{mineOpenID}, Label: "participated_by_me"},
		}
	}
	results := make([]any, 0)
	searched := make([]any, 0)
	seen := map[string]bool{}
	truncated := false
	for planIndex, plan := range plans {
		for windowIndex, window := range windows {
			searched = append(searched, map[string]any{
				"plan":  plan.Label,
				"start": timeString(window.Start),
				"end":   timeString(window.End),
			})
			pageToken := ""
			for page := 1; page <= maxSearchPages; page++ {
				data, err := searchPage(api, query, plan.Owners, plan.Participants, window, pageSize, pageToken)
				if err != nil {
					return nil, false, searched, err
				}
				items, _ := data["items"].([]any)
				for itemIndex, item := range items {
					token := searchItemToken(item)
					key := token
					if key == "" {
						encoded, _ := jsonMarshalStable(item)
						key = encoded
					}
					if seen[key] {
						continue
					}
					seen[key] = true
					results = append(results, sanitizeSearchItem(item))
					if len(results) >= limit {
						moreItems := itemIndex < len(items)-1
						morePages, _ := data["has_more"].(bool)
						moreWindows := windowIndex < len(windows)-1
						morePlans := planIndex < len(plans)-1
						truncated = moreItems || morePages || moreWindows || morePlans
						return results, truncated, searched, nil
					}
				}
				hasMore, _ := data["has_more"].(bool)
				if !hasMore {
					break
				}
				next, _ := data["page_token"].(string)
				if next == "" || next == pageToken {
					return nil, false, searched, validationError("飞书返回 has_more，但分页 token 没有前进")
				}
				pageToken = next
				if page == maxSearchPages {
					return nil, false, searched, validationError("妙记搜索分页超过安全上限，已停止以避免无限循环")
				}
			}
		}
	}
	return results, truncated, searched, nil
}

func jsonMarshalStable(value any) (string, error) {
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func commandSearch(args []string) (any, error) {
	set := newFlagSet("search")
	query := set.String("query", "", "关键词")
	days := set.Int("days", 0, "最近 N 天")
	startRaw := set.String("start", "", "开始时间")
	endRaw := set.String("end", "", "结束时间")
	var ownerValues, participantValues stringList
	set.Var(&ownerValues, "owner-id", "所有者 open_id")
	set.Var(&participantValues, "participant-id", "参与者 open_id")
	mine := set.Bool("mine", false, "查询我拥有或参与的妙记")
	userOpenID := set.String("user-open-id", "", "用于 --mine 的 open_id")
	pageSize := set.Int("page-size", 30, "每页数量")
	limit := set.Int("limit", defaultSearchResultLimit, "结果上限")
	all := set.Bool("all", false, "最多取 1000 条")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}
	*query = strings.TrimSpace(*query)
	if utf8.RuneCountInString(*query) > 50 {
		return nil, validationError("--query 最多 50 个字符")
	}
	if *pageSize < 1 || *pageSize > 30 {
		return nil, validationError("--page-size 必须在 1 到 30 之间")
	}
	owners, participants := flattenCSV(ownerValues), flattenCSV(participantValues)
	if *mine && (len(owners) > 0 || len(participants) > 0) {
		return nil, validationError("--mine 不能与 --owner-id/--participant-id 同时使用")
	}

	now := time.Now()
	var start, end *time.Time
	if *days != 0 {
		if *startRaw != "" || *endRaw != "" {
			return nil, validationError("--days 不能与 --start/--end 同时使用")
		}
		if *days < 1 || *days > 3650 {
			return nil, validationError("--days 必须在 1 到 3650 之间")
		}
		startValue, endValue := now.Add(-time.Duration(*days)*24*time.Hour), now
		start, end = &startValue, &endValue
	} else {
		if *startRaw != "" {
			value, err := parseDateTime(*startRaw, false)
			if err != nil {
				return nil, err
			}
			start = &value
		}
		if *endRaw != "" {
			value, err := parseDateTime(*endRaw, true)
			if err != nil {
				return nil, err
			}
			end = &value
		}
	}
	if *query == "" && len(owners) == 0 && len(participants) == 0 && !*mine && start == nil && end == nil {
		startValue, endValue := now.Add(-30*24*time.Hour), now
		start, end = &startValue, &endValue
	}
	windows, err := buildSearchWindows(start, end, now)
	if err != nil {
		return nil, err
	}

	mineOpenID := ""
	if *mine {
		mineOpenID = strings.TrimSpace(*userOpenID)
		if mineOpenID == "" {
			mineOpenID, err = configuredUserOpenID()
			if err != nil {
				return nil, err
			}
		}
		if mineOpenID == "" {
			return nil, configurationError("--mine 需要 --user-open-id 或 FEISHU_USER_OPEN_ID")
		}
		if !openIDPattern.MatchString(mineOpenID) {
			return nil, validationError("个人 open_id 应以 ou_ 开头")
		}
	}
	if *all {
		*limit = 1000
	}
	if *limit < 1 || *limit > 5000 {
		return nil, validationError("--limit 必须在 1 到 5000 之间")
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	if api.tokenSource != "user_access_token" {
		return nil, configurationError("search 是用户身份接口，需要在配置文件填写 user_access_token；app_id/app_secret 只能用于已知 minute_token 的详情与 AI 产物读取")
	}
	items, truncated, searched, err := searchMinutes(api, *query, owners, participants, mineOpenID, windows, *pageSize, *limit)
	if err != nil {
		return nil, err
	}
	interpretation := "current_identity_visible_minutes"
	if mineOpenID != "" {
		interpretation = "mine_union"
	}
	return map[string]any{
		"items":                items,
		"count":                len(items),
		"truncated":            truncated,
		"identity":             api.tokenSource,
		"scope_interpretation": interpretation,
		"searched_windows":     searched,
	}, nil
}

func cloneFilter(source map[string]any) map[string]any {
	copy := make(map[string]any, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func docSearchItemKey(item any) string {
	object, ok := item.(map[string]any)
	if ok {
		for _, key := range []string{"url", "token", "doc_token", "wiki_token", "document_id"} {
			if value, ok := object[key].(string); ok && value != "" {
				return key + ":" + value
			}
		}
	}
	encoded, _ := jsonMarshalStable(item)
	return encoded
}

func searchDocuments(api *client, baseBody map[string]any, pageSize, limit int) ([]any, bool, int, error) {
	results := make([]any, 0)
	seen := map[string]bool{}
	pageToken := ""
	pages := 0
	for page := 1; page <= maxDocSearchPages; page++ {
		body := cloneFilter(baseBody)
		body["page_size"] = pageSize
		if pageToken != "" {
			body["page_token"] = pageToken
		}
		data, err := api.requestJSON(http.MethodPost, "/open-apis/search/v2/doc_wiki/search", nil, body)
		if err != nil {
			return nil, false, pages, err
		}
		pages++
		items, _ := data["res_units"].([]any)
		for index, item := range items {
			key := docSearchItemKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, item)
			if len(results) >= limit {
				hasMore, _ := data["has_more"].(bool)
				return results, index < len(items)-1 || hasMore, pages, nil
			}
		}
		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			return results, false, pages, nil
		}
		next, _ := data["page_token"].(string)
		if next == "" || next == pageToken {
			return nil, false, pages, validationError("云文档搜索返回 has_more，但分页 token 没有前进")
		}
		pageToken = next
	}
	return nil, false, pages, validationError("云文档搜索分页超过安全上限，已停止以避免无限循环")
}

func commandDocSearch(args []string) (any, error) {
	set := newFlagSet("doc-search")
	query := set.String("query", "", "关键词")
	days := set.Int("days", 0, "最近 N 天创建的文档")
	startRaw := set.String("start", "", "文档创建开始时间")
	endRaw := set.String("end", "", "文档创建结束时间")
	var creatorValues, originalCreatorValues, folderValues, spaceValues stringList
	set.Var(&creatorValues, "creator-id", "当前归属人 open_id")
	set.Var(&originalCreatorValues, "original-creator-id", "最初创建者 open_id")
	set.Var(&folderValues, "folder-token", "限定云盘文件夹")
	set.Var(&spaceValues, "space-id", "限定知识库空间")
	mine := set.Bool("mine", false, "只查我当前归属的文档")
	userOpenID := set.String("user-open-id", "", "用于 --mine 的 open_id")
	docTypesRaw := set.String("doc-types", "docx,wiki", "文档类型，逗号分隔")
	onlyTitle := set.Bool("only-title", false, "只搜索标题")
	sortRaw := set.String("sort", "create_time", "default/edit_time/edit_time_asc/open_time/create_time")
	pageSize := set.Int("page-size", 20, "每页数量")
	limit := set.Int("limit", defaultSearchResultLimit, "结果上限")
	all := set.Bool("all", false, "最多取 1000 条")
	if err := parseFlagSet(set, args); err != nil {
		return nil, err
	}

	*query = strings.TrimSpace(*query)
	if utf8.RuneCountInString(*query) > 30 {
		return nil, validationError("--query 最多 30 个字符")
	}
	if *pageSize < 1 || *pageSize > 20 {
		return nil, validationError("--page-size 必须在 1 到 20 之间")
	}
	if *all {
		*limit = 1000
	}
	if *limit < 1 || *limit > 5000 {
		return nil, validationError("--limit 必须在 1 到 5000 之间")
	}

	creators := flattenCSV(creatorValues)
	originalCreators := flattenCSV(originalCreatorValues)
	folders := flattenCSV(folderValues)
	spaces := flattenCSV(spaceValues)
	if *mine && len(creators) > 0 {
		return nil, validationError("--mine 不能与 --creator-id 同时使用")
	}
	if len(folders) > 0 && len(spaces) > 0 {
		return nil, validationError("--folder-token 不能与 --space-id 同时使用")
	}
	for _, id := range append(append([]string{}, creators...), originalCreators...) {
		if !openIDPattern.MatchString(id) {
			return nil, validationError("文档人员 open_id 应以 ou_ 开头：%s", id)
		}
	}
	for _, token := range append(append([]string{}, folders...), spaces...) {
		if !resourcePattern.MatchString(token) {
			return nil, validationError("无效的文件夹 token 或知识库 space_id：%s", token)
		}
	}
	if *mine {
		mineOpenID := strings.TrimSpace(*userOpenID)
		if mineOpenID == "" {
			configuredOpenID, configErr := configuredUserOpenID()
			if configErr != nil {
				return nil, configErr
			}
			mineOpenID = configuredOpenID
		}
		if mineOpenID == "" {
			return nil, configurationError("--mine 需要 --user-open-id 或 FEISHU_USER_OPEN_ID")
		}
		if !openIDPattern.MatchString(mineOpenID) {
			return nil, validationError("个人 open_id 应以 ou_ 开头")
		}
		creators = []string{mineOpenID}
	}

	allowedTypes := map[string]bool{
		"DOC": true, "SHEET": true, "BITABLE": true, "MINDNOTE": true, "FILE": true,
		"WIKI": true, "DOCX": true, "FOLDER": true, "CATALOG": true, "SLIDES": true, "SHORTCUT": true,
	}
	docTypes := flattenCSV(stringList{*docTypesRaw})
	if len(docTypes) == 0 {
		return nil, validationError("--doc-types 至少需要一种文档类型")
	}
	for index, value := range docTypes {
		docTypes[index] = strings.ToUpper(value)
		if !allowedTypes[docTypes[index]] {
			return nil, validationError("--doc-types 包含不支持的类型 %q", value)
		}
	}
	allowedSorts := map[string]string{
		"default": "DEFAULT_TYPE", "edit_time": "EDIT_TIME", "edit_time_asc": "EDIT_TIME_ASC",
		"open_time": "OPEN_TIME", "create_time": "CREATE_TIME",
	}
	sortType, ok := allowedSorts[strings.ToLower(strings.TrimSpace(*sortRaw))]
	if !ok {
		return nil, validationError("--sort 只能是 default、edit_time、edit_time_asc、open_time 或 create_time")
	}

	now := time.Now()
	var start, end *time.Time
	if *days != 0 {
		if *startRaw != "" || *endRaw != "" {
			return nil, validationError("--days 不能与 --start/--end 同时使用")
		}
		if *days < 1 || *days > 3650 {
			return nil, validationError("--days 必须在 1 到 3650 之间")
		}
		startValue, endValue := now.Add(-time.Duration(*days)*24*time.Hour), now
		start, end = &startValue, &endValue
	} else {
		if *startRaw != "" {
			value, err := parseDateTime(*startRaw, false)
			if err != nil {
				return nil, err
			}
			start = &value
		}
		if *endRaw != "" {
			value, err := parseDateTime(*endRaw, true)
			if err != nil {
				return nil, err
			}
			end = &value
		}
	}
	if *query == "" && len(creators) == 0 && len(originalCreators) == 0 && len(folders) == 0 && len(spaces) == 0 && start == nil && end == nil {
		startValue, endValue := now.Add(-30*24*time.Hour), now
		start, end = &startValue, &endValue
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, validationError("开始时间不能晚于结束时间")
	}

	filter := map[string]any{"doc_types": docTypes, "sort_type": sortType}
	if len(creators) > 0 {
		filter["creator_ids"] = creators
	}
	if len(originalCreators) > 0 {
		filter["original_creator_ids"] = originalCreators
	}
	if start != nil || end != nil {
		created := map[string]any{}
		if start != nil {
			created["start"] = start.Unix()
		}
		if end != nil {
			created["end"] = end.Unix()
		}
		filter["create_time"] = created
	}
	if *onlyTitle {
		filter["only_title"] = true
	}
	body := map[string]any{"query": *query}
	switch {
	case len(folders) > 0:
		docFilter := cloneFilter(filter)
		docFilter["folder_tokens"] = folders
		body["doc_filter"] = docFilter
	case len(spaces) > 0:
		wikiFilter := cloneFilter(filter)
		wikiFilter["space_ids"] = spaces
		body["wiki_filter"] = wikiFilter
	default:
		body["doc_filter"] = cloneFilter(filter)
		body["wiki_filter"] = cloneFilter(filter)
	}

	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	items, truncated, pages, err := searchDocuments(api, body, *pageSize, *limit)
	if err != nil {
		return nil, err
	}
	interpretation := "current_identity_visible_cloud_documents"
	if *mine {
		interpretation = "owned_by_configured_user_within_current_identity_visibility"
	}
	return map[string]any{
		"items": items, "count": len(items), "truncated": truncated, "pages_fetched": pages,
		"identity": api.tokenSource, "content_is_untrusted": true,
		"scope_interpretation": interpretation,
	}, nil
}

func noteIDFromMinuteDetail(detail map[string]any) string {
	if minute, ok := detail["minute"].(map[string]any); ok {
		if noteID, ok := minute["note_id"].(string); ok {
			return noteID
		}
	}
	noteID, _ := detail["note_id"].(string)
	return noteID
}

func commandShow(args []string) (any, error) {
	if len(args) == 0 {
		return nil, validationError("show 需要 minute_token 或妙记 URL")
	}
	token, err := parseMinuteToken(args[0])
	if err != nil {
		return nil, err
	}
	set := newFlagSet("show")
	artifactsRaw := set.String("artifacts", "", "summary,todos,chapters,keywords,transcript,all")
	transcriptFormat := set.String("transcript-format", "txt", "txt 或 srt")
	noSpeaker := set.Bool("no-speaker", false, "不包含说话人")
	noTimestamp := set.Bool("no-timestamp", false, "不包含时间戳")
	output := set.String("output", "", "逐字稿输出路径")
	overwrite := set.Bool("overwrite", false, "覆盖显式输出文件")
	if err := parseFlagSet(set, args[1:]); err != nil {
		return nil, err
	}
	if *transcriptFormat != "txt" && *transcriptFormat != "srt" {
		return nil, validationError("--transcript-format 只能是 txt 或 srt")
	}
	requested, err := parseArtifacts(*artifactsRaw)
	if err != nil {
		return nil, err
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	escaped := url.PathEscape(token)
	detail, err := api.requestJSON(http.MethodGet, "/open-apis/minutes/v1/minutes/"+escaped, nil, nil)
	if err != nil {
		return nil, err
	}
	minute := any(detail)
	if value, ok := detail["minute"]; ok {
		minute = value
	}
	resultArtifacts := map[string]any{}
	warnings := make([]any, 0)
	partialErrors := make([]any, 0)
	artifactData := map[string]any{}
	var artifactFetchErr error
	if len(requested) > 0 {
		fetched, fetchErr := api.requestJSON(http.MethodGet, "/open-apis/minutes/v1/minutes/"+escaped+"/artifacts", nil, nil)
		if fetchErr != nil {
			artifactFetchErr = fetchErr
		} else {
			artifactData = fetched
		}
	}
	mappings := map[string]string{
		"summary": "summary", "todos": "minute_todos", "chapters": "minute_chapters", "keywords": "keywords",
	}
	for name, apiName := range mappings {
		if !requested[name] {
			continue
		}
		value := artifactData[apiName]
		if value == nil {
			if name == "summary" {
				value = ""
			} else {
				value = []any{}
			}
		}
		resultArtifacts[name] = value
	}
	if artifactFetchErr != nil && (requested["summary"] || requested["todos"] || requested["chapters"] || requested["keywords"]) {
		partialErrors = append(partialErrors, map[string]any{"artifact": "artifacts", "error": errorPayload(artifactFetchErr)})
	}
	if requested["transcript"] {
		parameters := url.Values{
			"need_speaker":   []string{strconv.FormatBool(!*noSpeaker)},
			"need_timestamp": []string{strconv.FormatBool(!*noTimestamp)},
			"file_format":    []string{*transcriptFormat},
		}
		transcript, transcriptErr := api.requestBytes(http.MethodGet, "/open-apis/minutes/v1/minutes/"+escaped+"/transcript", parameters)
		source := "transcript_export_api"
		if transcriptErr != nil {
			if fallback, ok := artifactData["transcript"].(string); ok && fallback != "" {
				transcript = []byte(fallback)
				source = "artifacts_fallback"
				warnings = append(warnings, map[string]any{
					"message": "文字记录导出接口失败，已退化到 artifacts 中的 transcript；格式可能没有完整说话人或时间戳。",
					"cause":   errorPayload(transcriptErr),
				})
			} else {
				partialErrors = append(partialErrors, map[string]any{"artifact": "transcript", "error": errorPayload(transcriptErr)})
				if artifactFetchErr != nil && len(partialErrors) == 1 {
					partialErrors = append(partialErrors, map[string]any{"artifact": "transcript_fallback", "error": errorPayload(artifactFetchErr)})
				}
			}
		}
		if transcript != nil {
			explicit := strings.TrimSpace(*output) != ""
			outputPath := strings.TrimSpace(*output)
			if !explicit {
				outputPath, err = defaultArtifactPath("minutes", token, "transcript."+*transcriptFormat)
				if err != nil {
					return nil, err
				}
			}
			saved, err := saveBytes(transcript, outputPath, *overwrite, explicit)
			if err != nil {
				return nil, err
			}
			preview, previewTruncated := previewBytes(transcript, 2000)
			resultArtifacts["transcript"] = map[string]any{
				"source": source, "file": saved, "size_bytes": len(transcript),
				"preview": preview, "preview_truncated": previewTruncated,
			}
		}
	}
	return map[string]any{
		"minute_token": token,
		"identity":     api.tokenSource,
		"minute":       minute,
		"note_id":      noteIDFromMinuteDetail(detail),
		"artifacts":    resultArtifacts,
		"warnings":     warnings,
		"errors":       partialErrors,
	}, nil
}

func evidenceDocument(
	api *client,
	bundleDir, role, token string,
	overwrite, explicit bool,
) (map[string]any, error) {
	content, resolved, err := fetchDocumentContent(api, "docx", token)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(bundleDir, role+"-"+token+".txt")
	saved, err := saveBytes([]byte(content), path, overwrite, explicit)
	if err != nil {
		return nil, err
	}
	preview, truncated := previewBytes([]byte(content), 2000)
	return map[string]any{
		"role": role, "token": token, "document": resolved, "file": saved,
		"size_bytes": len([]byte(content)), "preview": preview, "preview_truncated": truncated,
		"content_is_untrusted": true,
	}, nil
}

func commandEvidence(args []string) (any, error) {
	if len(args) == 0 {
		return nil, validationError("evidence 需要 minute_token 或妙记 URL")
	}
	token, err := parseMinuteToken(args[0])
	if err != nil {
		return nil, err
	}
	set := newFlagSet("evidence")
	outputDirRaw := set.String("output-dir", "", "证据包目录")
	overwrite := set.Bool("overwrite", false, "覆盖显式目录中的同名文件")
	transcriptFormat := set.String("transcript-format", "txt", "txt 或 srt")
	includeNoteDoc := set.Bool("note-doc", true, "包含智能纪要主文档；可传 --note-doc=false")
	includeVerbatim := set.Bool("verbatim-doc", false, "包含 Note 的逐字稿文档")
	includeShared := set.Bool("shared-docs", false, "包含 Note 引用的共享文档，最多 20 个")
	if err := parseFlagSet(set, args[1:]); err != nil {
		return nil, err
	}
	if *transcriptFormat != "txt" && *transcriptFormat != "srt" {
		return nil, validationError("--transcript-format 只能是 txt 或 srt")
	}

	explicitDir := strings.TrimSpace(*outputDirRaw) != ""
	bundleDir := strings.TrimSpace(*outputDirRaw)
	if explicitDir {
		bundleDir, err = filepath.Abs(bundleDir)
		if err != nil {
			return nil, validationError("无法解析证据包目录: %v", err)
		}
		if !*overwrite {
			entries, readErr := os.ReadDir(bundleDir)
			if readErr == nil && len(entries) > 0 {
				return nil, validationError("证据包目录不是空目录：%s；如需覆盖同名文件请传 --overwrite", bundleDir)
			}
			if readErr != nil && !os.IsNotExist(readErr) {
				return nil, validationError("无法检查证据包目录: %v", readErr)
			}
		}
	} else {
		manifestPath, pathErr := defaultArtifactPath("evidence", token, "manifest.json")
		if pathErr != nil {
			return nil, pathErr
		}
		bundleDir = filepath.Dir(manifestPath)
	}
	allowOverwrite := *overwrite || !explicitDir
	transcriptPath := filepath.Join(bundleDir, "transcript."+*transcriptFormat)
	showArgs := []string{
		token, "--artifacts", "all", "--transcript-format", *transcriptFormat,
		"--output", transcriptPath,
	}
	if allowOverwrite {
		showArgs = append(showArgs, "--overwrite")
	}
	showResultRaw, err := commandShow(showArgs)
	if err != nil {
		return nil, err
	}
	showResult, _ := showResultRaw.(map[string]any)

	bundle := map[string]any{
		"minute_token": token, "bundle_dir": bundleDir, "meeting": showResult,
		"note": nil, "documents": []any{}, "errors": []any{}, "content_is_untrusted": true,
	}
	errorsList := make([]any, 0)
	documents := make([]any, 0)
	noteID, _ := showResult["note_id"].(string)
	if noteID != "" && (*includeNoteDoc || *includeVerbatim || *includeShared) {
		api, clientErr := newClientFromEnv()
		if clientErr != nil {
			return nil, clientErr
		}
		note, noteErr := fetchNoteDetail(api, noteID)
		if noteErr != nil {
			errorsList = append(errorsList, map[string]any{"resource": "note", "error": errorPayload(noteErr)})
		} else {
			bundle["note"] = note
			type docTarget struct{ role, token string }
			targets := make([]docTarget, 0)
			if *includeNoteDoc {
				if value, _ := note["note_doc_token"].(string); value != "" {
					targets = append(targets, docTarget{"note", value})
				}
			}
			if *includeVerbatim {
				if value, _ := note["verbatim_doc_token"].(string); value != "" {
					targets = append(targets, docTarget{"verbatim", value})
				}
			}
			if *includeShared {
				sharedTokens, _ := note["shared_doc_tokens"].([]string)
				for index, value := range sharedTokens {
					if index >= 20 {
						errorsList = append(errorsList, map[string]any{"resource": "shared_docs", "warning": "共享文档超过 20 个，仅读取前 20 个"})
						break
					}
					if value != "" {
						targets = append(targets, docTarget{fmt.Sprintf("shared-%02d", index+1), value})
					}
				}
			}
			seen := map[string]bool{}
			for _, target := range targets {
				if seen[target.token] {
					continue
				}
				seen[target.token] = true
				document, docErr := evidenceDocument(api, bundleDir, target.role, target.token, allowOverwrite, explicitDir)
				if docErr != nil {
					errorsList = append(errorsList, map[string]any{"resource": target.role, "token": target.token, "error": errorPayload(docErr)})
					continue
				}
				documents = append(documents, document)
			}
		}
	}
	bundle["documents"] = documents
	bundle["errors"] = errorsList
	manifestRaw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, validationError("无法生成证据包清单: %v", err)
	}
	manifestPath := filepath.Join(bundleDir, "manifest.json")
	savedManifest, err := saveBytes(manifestRaw, manifestPath, allowOverwrite, explicitDir)
	if err != nil {
		return nil, err
	}
	bundle["manifest_file"] = savedManifest
	return bundle, nil
}

func previewBytes(raw []byte, maxRunes int) (string, bool) {
	text := string(raw)
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text, false
	}
	return string(runes[:maxRunes]), true
}

func fetchNoteDetail(api *client, noteID string) (map[string]any, error) {
	path := "/open-apis/vc/v1/notes/" + url.PathEscape(noteID)
	data, err := api.requestJSON(http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	note, ok := data["note"].(map[string]any)
	if !ok {
		return nil, &apiError{Message: "Note 详情响应中没有 note 对象", Path: path}
	}
	displayRaw := note["note_display_type"]
	if displayRaw == nil {
		displayRaw = note["display_type"]
	}
	display := "unknown"
	switch numericCode(displayRaw) {
	case 1:
		display = "normal"
	case 2:
		display = "unified"
	}
	noteDoc, verbatimDoc := "", ""
	for _, raw := range sliceAny(note["artifacts"]) {
		artifact, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		docToken, _ := artifact["doc_token"].(string)
		switch numericCode(artifact["artifact_type"]) {
		case 1:
			noteDoc = docToken
		case 2:
			verbatimDoc = docToken
		}
	}
	sharedDocs := make([]string, 0)
	for _, raw := range sliceAny(note["references"]) {
		reference, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if token, ok := reference["doc_token"].(string); ok && token != "" {
			sharedDocs = append(sharedDocs, token)
		}
	}
	return map[string]any{
		"note_id": noteID, "note_display_type": display,
		"creator_id": note["creator_id"], "create_time": note["create_time"],
		"note_doc_token": noteDoc, "verbatim_doc_token": verbatimDoc,
		"shared_doc_tokens": sharedDocs, "raw": note,
	}, nil
}

func commandNote(args []string) (any, error) {
	if len(args) != 1 {
		return nil, validationError("note 需要且只接受一个 note_id")
	}
	noteID, err := parseResourceName(args[0], "note_id")
	if err != nil {
		return nil, err
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	detail, err := fetchNoteDetail(api, noteID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"identity": api.tokenSource, "note": detail}, nil
}

func commandNoteTranscript(args []string) (any, error) {
	if len(args) == 0 {
		return nil, validationError("note-transcript 需要 note_id")
	}
	noteID, err := parseResourceName(args[0], "note_id")
	if err != nil {
		return nil, err
	}
	set := newFlagSet("note-transcript")
	transcriptFormat := set.String("format", "markdown", "markdown 或 plain_text")
	locale := set.String("locale", "zh_cn", "zh_cn/en_us/ja_jp")
	output := set.String("output", "", "输出路径")
	overwrite := set.Bool("overwrite", false, "覆盖显式输出")
	if err := parseFlagSet(set, args[1:]); err != nil {
		return nil, err
	}
	if *transcriptFormat != "markdown" && *transcriptFormat != "plain_text" {
		return nil, validationError("--format 只能是 markdown 或 plain_text")
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	detail, err := fetchNoteDetail(api, noteID)
	if err != nil {
		return nil, err
	}
	if detail["note_display_type"] != "unified" {
		if verbatim, _ := detail["verbatim_doc_token"].(string); verbatim != "" {
			return nil, validationError("该 Note 不是 unified 类型；请改用 doc %s", verbatim)
		}
		return nil, validationError("该 Note 不是 unified 类型；请先检查 note 详情中的逐字稿入口")
	}
	path := "/open-apis/vc/v1/notes/" + url.PathEscape(noteID) + "/unified_note_transcript"
	cursor := ""
	seen := map[string]bool{}
	var content strings.Builder
	completed := false
	for page := 1; page <= maxNoteTranscriptPages; page++ {
		parameters := url.Values{
			"format":    []string{*transcriptFormat},
			"locale":    []string{*locale},
			"page_size": []string{"200"},
		}
		if cursor != "" {
			parameters.Set("cursor_id", cursor)
		}
		data, err := api.requestJSON(http.MethodGet, path, parameters, nil)
		if err != nil {
			return nil, err
		}
		if transcript, ok := data["transcript"].(map[string]any); ok {
			if chunk, ok := transcript[*transcriptFormat].(string); ok {
				content.WriteString(chunk)
			}
		}
		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			completed = true
			break
		}
		next := strings.TrimSpace(fmt.Sprint(data["next_cursor_id"]))
		if next == "" || next == "<nil>" || next == cursor || seen[next] {
			return nil, validationError("unified transcript 第 %d 页游标没有前进", page)
		}
		if cursor != "" {
			seen[cursor] = true
		}
		cursor = next
		time.Sleep(100 * time.Millisecond)
	}
	if !completed {
		return nil, validationError("unified transcript 分页超过安全上限")
	}
	if content.Len() == 0 {
		return nil, validationError("unified transcript 为空，未保存文件")
	}
	extension := "md"
	if *transcriptFormat == "plain_text" {
		extension = "txt"
	}
	explicit := strings.TrimSpace(*output) != ""
	outputPath := strings.TrimSpace(*output)
	if !explicit {
		outputPath, err = defaultArtifactPath("notes", noteID, "unified_transcript."+extension)
		if err != nil {
			return nil, err
		}
	}
	raw := []byte(content.String())
	saved, err := saveBytes(raw, outputPath, *overwrite, explicit)
	if err != nil {
		return nil, err
	}
	preview, truncated := previewBytes(raw, 2000)
	return map[string]any{
		"identity": api.tokenSource, "note_id": noteID, "format": *transcriptFormat,
		"file": saved, "size_bytes": len(raw), "preview": preview, "preview_truncated": truncated,
	}, nil
}

func fetchDocumentContent(api *client, kind, token string) (string, map[string]any, error) {
	resolved := map[string]any{"input_type": kind, "input_token": token}
	if kind == "wiki" {
		data, err := api.requestJSON(http.MethodGet, "/open-apis/wiki/v2/spaces/get_node", url.Values{"token": []string{token}}, nil)
		if err != nil {
			return "", nil, err
		}
		node, ok := data["node"].(map[string]any)
		if !ok {
			return "", nil, &apiError{Message: "Wiki 响应中没有 node 对象", Path: "/open-apis/wiki/v2/spaces/get_node"}
		}
		objectToken, tokenOK := node["obj_token"].(string)
		objectType, typeOK := node["obj_type"].(string)
		if !tokenOK || !typeOK {
			return "", nil, &apiError{Message: "Wiki node 缺少 obj_token 或 obj_type", Path: "/open-apis/wiki/v2/spaces/get_node"}
		}
		kind, token = objectType, objectToken
		resolved["wiki_node"] = node
	}
	var path string
	switch kind {
	case "docx":
		path = "/open-apis/docx/v1/documents/" + url.PathEscape(token) + "/raw_content"
	case "doc":
		path = "/open-apis/doc/v2/" + url.PathEscape(token) + "/raw_content"
	default:
		return "", nil, validationError("Wiki 底层类型 %q 不是本技能可读取的 Docx/Doc", kind)
	}
	data, err := api.requestJSON(http.MethodGet, path, nil, nil)
	if err != nil {
		return "", nil, err
	}
	content, ok := data["content"].(string)
	if !ok {
		return "", nil, &apiError{Message: "文档响应中没有纯文本 content", Path: path, Details: data}
	}
	resolved["resolved_type"] = kind
	resolved["resolved_token"] = token
	return content, resolved, nil
}

func decodedLink(raw string) string {
	value := strings.TrimSpace(raw)
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = decoded
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return ""
	}
	return value
}

func collectRichLinks(value any, links *[]any, seen map[string]bool) {
	switch current := value.(type) {
	case []any:
		for _, item := range current {
			collectRichLinks(item, links, seen)
		}
	case map[string]any:
		label := ""
		for _, key := range []string{"content", "title"} {
			if text, ok := current[key].(string); ok && strings.TrimSpace(text) != "" {
				label = strings.TrimSpace(text)
				break
			}
		}
		if raw, ok := current["url"].(string); ok {
			if target := decodedLink(raw); target != "" && !seen[target] {
				seen[target] = true
				entry := map[string]any{"url": target}
				if label != "" {
					entry["text"] = label
				}
				*links = append(*links, entry)
			}
		}
		for _, child := range current {
			collectRichLinks(child, links, seen)
		}
	}
}

func fetchDocxLinks(api *client, token string) ([]any, error) {
	links := make([]any, 0)
	seenLinks := map[string]bool{}
	pageToken := ""
	seenPages := map[string]bool{}
	for page := 1; page <= 100; page++ {
		parameters := url.Values{
			"page_size":            []string{"500"},
			"document_revision_id": []string{"-1"},
		}
		if pageToken != "" {
			parameters.Set("page_token", pageToken)
		}
		data, err := api.requestJSON(http.MethodGet, "/open-apis/docx/v1/documents/"+url.PathEscape(token)+"/blocks", parameters, nil)
		if err != nil {
			return nil, err
		}
		collectRichLinks(data["items"], &links, seenLinks)
		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			return links, nil
		}
		next, _ := data["page_token"].(string)
		if next == "" || next == pageToken || seenPages[next] {
			return nil, validationError("文档块分页 token 没有前进")
		}
		seenPages[next] = true
		pageToken = next
	}
	return nil, validationError("文档块分页超过安全上限")
}

func commandDoc(args []string) (any, error) {
	if len(args) == 0 {
		return nil, validationError("doc 需要文档 URL 或 token")
	}
	reference, err := documentRefFrom(args[0])
	if err != nil {
		return nil, err
	}
	set := newFlagSet("doc")
	output := set.String("output", "", "输出路径")
	overwrite := set.Bool("overwrite", false, "覆盖显式输出")
	inline := set.Bool("inline", false, "在 JSON 中包含完整正文")
	withLinks := set.Bool("links", false, "读取富文本块中的超链接")
	if err := parseFlagSet(set, args[1:]); err != nil {
		return nil, err
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	content, resolved, err := fetchDocumentContent(api, reference.Kind, reference.Token)
	if err != nil {
		return nil, err
	}
	resolvedToken, _ := resolved["resolved_token"].(string)
	explicit := strings.TrimSpace(*output) != ""
	outputPath := strings.TrimSpace(*output)
	if !explicit {
		outputPath, err = defaultArtifactPath("documents", resolvedToken, "content.txt")
		if err != nil {
			return nil, err
		}
	}
	raw := []byte(content)
	saved, err := saveBytes(raw, outputPath, *overwrite, explicit)
	if err != nil {
		return nil, err
	}
	preview, truncated := previewBytes(raw, 2000)
	result := map[string]any{
		"identity": api.tokenSource, "document": resolved, "content_file": saved,
		"size_bytes": len(raw), "preview": preview, "preview_truncated": truncated,
		"content_is_untrusted": true,
	}
	if *inline {
		result["content"] = content
	}
	if *withLinks {
		resolvedType, _ := resolved["resolved_type"].(string)
		if resolvedType != "docx" {
			return nil, validationError("--links 目前只支持 Docx 文档")
		}
		links, linkErr := fetchDocxLinks(api, resolvedToken)
		if linkErr != nil {
			return nil, linkErr
		}
		result["links"] = links
		result["link_count"] = len(links)
	}
	return result, nil
}

func documentRefFrom(value string) (documentRef, error) { return parseDocumentRef(value) }

func commandMeeting(args []string) (any, error) {
	if len(args) == 0 {
		return nil, validationError("meeting 需要 meeting_id")
	}
	meetingID, err := parseResourceName(args[0], "meeting_id")
	if err != nil {
		return nil, err
	}
	set := newFlagSet("meeting")
	withParticipants := set.Bool("with-participants", false, "读取参会人快照")
	if err := parseFlagSet(set, args[1:]); err != nil {
		return nil, err
	}
	api, err := newClientFromEnv()
	if err != nil {
		return nil, err
	}
	escaped := url.PathEscape(meetingID)
	detail, err := api.requestJSON(
		http.MethodGet,
		"/open-apis/vc/v1/meetings/"+escaped,
		url.Values{"with_participants": []string{strconv.FormatBool(*withParticipants)}, "query_mode": []string{"0"}},
		nil,
	)
	if err != nil {
		return nil, err
	}
	meeting := any(detail)
	if value, ok := detail["meeting"]; ok {
		meeting = value
	}
	result := map[string]any{
		"identity": api.tokenSource, "meeting_id": meetingID, "meeting": meeting,
		"recording": nil, "minute_token": "",
	}
	recordingData, recordingErr := api.requestJSON(http.MethodGet, "/open-apis/vc/v1/meetings/"+escaped+"/recording", nil, nil)
	if recordingErr != nil {
		result["recording_error"] = errorPayload(recordingErr)
		return result, nil
	}
	recording := any(recordingData)
	if value, ok := recordingData["recording"]; ok {
		recording = value
	}
	result["recording"] = recording
	if object, ok := recording.(map[string]any); ok {
		result["minute_token"] = tokenFromMinuteURL(object["url"])
	}
	return result, nil
}

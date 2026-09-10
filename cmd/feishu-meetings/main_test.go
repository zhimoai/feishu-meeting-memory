package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeTestJSON(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func setTestEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("FEISHU_CONFIG_FILE", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("FEISHU_API_BASE", serverURL)
	t.Setenv("FEISHU_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "test-token")
	t.Setenv("FEISHU_TENANT_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
}

func TestConfigFileLoadsAndEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	payload := `{"app_id":"cli_from_file","app_secret":"secret-from-file-123","user_open_id":"ou_from_file","api_base":"https://open.feishu.cn","http_timeout_seconds":45}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_CONFIG_FILE", path)
	for _, key := range []string{
		"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_USER_ACCESS_TOKEN", "FEISHU_TENANT_ACCESS_TOKEN",
		"FEISHU_ACCESS_TOKEN", "FEISHU_USER_OPEN_ID", "FEISHU_API_BASE", "FEISHU_HTTP_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
	api, err := newClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !api.configLoaded || api.configPath != path || api.appID != "cli_from_file" || api.userOpenID != "ou_from_file" {
		t.Fatalf("api=%+v", api)
	}
	if api.httpClient.Timeout != 45*time.Second {
		t.Fatalf("timeout=%v", api.httpClient.Timeout)
	}
	t.Setenv("FEISHU_APP_ID", "cli_from_env")
	t.Setenv("FEISHU_APP_SECRET", "secret-from-env-456")
	api, err = newClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if api.appID != "cli_from_env" || api.appSecret != "secret-from-env-456" {
		t.Fatalf("environment did not override file")
	}
}

func TestSkillConfigPathUsesSkillRoot(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "windows-amd64", "feishu-meetings.exe")
	path, ok := skillConfigPath(executable)
	if !ok {
		t.Fatal("expected packaged executable layout to be recognized")
	}
	if path != filepath.Join(root, "config.json") {
		t.Fatalf("path=%q", path)
	}
	if _, ok := skillConfigPath(filepath.Join(root, "feishu-meetings.exe")); ok {
		t.Fatal("unexpected skill root inference outside bin/<platform>")
	}
}

func TestParseMinuteTokenFromURL(t *testing.T) {
	token, err := parseMinuteToken("https://team.feishu.cn/minutes/obcnABC123?from=share")
	if err != nil {
		t.Fatal(err)
	}
	if token != "obcnABC123" {
		t.Fatalf("token=%q", token)
	}
}

func TestCollectRichLinksDecodesAndDeduplicates(t *testing.T) {
	value := []any{
		map[string]any{"content": "妙记", "url": "https%3A%2F%2Fexample.feishu.cn%2Fminutes%2FobcnABC123"},
		map[string]any{"url": "https%3A%2F%2Fexample.feishu.cn%2Fminutes%2FobcnABC123"},
		map[string]any{"url": "javascript%3Aalert%281%29"},
	}
	links := make([]any, 0)
	collectRichLinks(value, &links, map[string]bool{})
	if len(links) != 1 {
		t.Fatalf("links=%v", links)
	}
	entry := links[0].(map[string]any)
	if entry["url"] != "https://example.feishu.cn/minutes/obcnABC123" || entry["text"] != "妙记" {
		t.Fatalf("entry=%v", entry)
	}
}

func TestMissingScopeCodeIsPermissionError(t *testing.T) {
	err := &apiError{Status: 400, Code: json.Number("99991672"), Message: "missing scope"}
	if !err.PermissionError() {
		t.Fatal("missing-scope code should be classified as a permission error")
	}
}

func TestMissingUserScopeCodeIsPermissionError(t *testing.T) {
	err := &apiError{Status: 400, Code: json.Number("99991679"), Message: "missing user scope"}
	if !err.PermissionError() {
		t.Fatal("missing-user-scope code should be classified as a permission error")
	}
}

func TestPermissionsIncludesFullReadonlyAndRefreshScopes(t *testing.T) {
	result, err := commandPermissions(nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes := result.(map[string]any)["required_oauth_scopes"].([]string)
	joined := " " + strings.Join(scopes, " ") + " "
	for _, scope := range []string{
		"offline_access", "space:document:retrieve", "docx:document:readonly",
		"minutes:minutes.search:read", "minutes:minutes.transcript:export",
	} {
		if !strings.Contains(joined, " "+scope+" ") {
			t.Fatalf("missing scope %s in %v", scope, scopes)
		}
	}
}

func TestMissingOAuthScopes(t *testing.T) {
	missing := missingOAuthScopes("one three", []string{"one", "two", "three"})
	if len(missing) != 1 || missing[0] != "two" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestBuildSearchWindowsNewestFirst(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	windows, err := buildSearchWindows(&start, &end, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) < 4 || !windows[0].End.Equal(end) || !windows[len(windows)-1].Start.Equal(start) {
		t.Fatalf("unexpected windows: %#v", windows)
	}
	for index, window := range windows {
		if window.End.Sub(*window.Start) > 30*24*time.Hour {
			t.Fatalf("window %d too large", index)
		}
		if index > 0 && !windows[index-1].Start.Add(-time.Second).Equal(*window.End) {
			t.Fatalf("gap between windows %d and %d", index-1, index)
		}
	}
}

func TestDoctorFetchesTokenWithoutExposingSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "very-secret") {
			t.Fatalf("secret not sent to auth endpoint")
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "tenant_access_token": "t-token", "expire": 7200})
	}))
	defer server.Close()
	t.Setenv("FEISHU_API_BASE", server.URL)
	t.Setenv("FEISHU_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_TENANT_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_APP_ID", "cli_test")
	t.Setenv("FEISHU_APP_SECRET", "very-secret")
	result, err := commandDoctor(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "very-secret") || strings.Contains(string(encoded), "t-token") {
		t.Fatalf("doctor exposed a secret: %s", encoded)
	}
}

func TestDoctorProbeReportsEachSearchCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/minutes/v1/minutes/search":
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"items": []any{}, "has_more": false}})
		case "/open-apis/search/v2/doc_wiki/search":
			writeTestJSON(t, w, 403, map[string]any{"code": 99991672, "msg": "missing scope"})
		default:
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	result, err := commandDoctor([]string{"--probe"})
	if err != nil {
		t.Fatal(err)
	}
	probes := result.(map[string]any)["probes"].(map[string]any)
	if probes["minutes_search"].(map[string]any)["ok"] != true {
		t.Fatalf("minutes=%v", probes["minutes_search"])
	}
	if probes["cloud_documents_search"].(map[string]any)["ok"] != false {
		t.Fatalf("docs=%v", probes["cloud_documents_search"])
	}
}

func TestSearchRejectsApplicationIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"app_id":"cli_test","app_secret":"secret-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_CONFIG_FILE", path)
	for _, key := range []string{
		"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_USER_ACCESS_TOKEN", "FEISHU_TENANT_ACCESS_TOKEN", "FEISHU_ACCESS_TOKEN",
	} {
		t.Setenv(key, "")
	}
	_, err := commandSearch([]string{"--days", "1"})
	if err == nil || !strings.Contains(err.Error(), "user_access_token") {
		t.Fatalf("expected user identity error, got %v", err)
	}
}

func TestUserSearchReturnsMinimalIdentityHints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/contact/v3/users/search" || r.URL.Query().Get("page_size") != "10" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		filter := body["filter"].(map[string]any)
		if filter["exclude_outer_contact"] != true {
			t.Fatalf("filter=%v", filter)
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{
				"id": "ou_zhangsan", "display_info": "<h>张三</h>\n研发部",
				"meta_data": map[string]any{
					"i18n_names": map[string]any{"zh_cn": "张三"}, "mail_address": "private@example.com",
					"is_registered": true, "is_cross_tenant": false,
				},
			}},
			"has_more": false,
		}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	t.Setenv("FEISHU_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "user-token")
	result, err := commandUserSearch([]string{"--query", "张三", "--exclude-external", "--page-size", "10"})
	if err != nil {
		t.Fatal(err)
	}
	users := result.(map[string]any)["users"].([]any)
	user := users[0].(map[string]any)
	if user["open_id"] != "ou_zhangsan" || user["department_hint"] != "研发部" {
		t.Fatalf("user=%v", user)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "private@example.com") {
		t.Fatalf("private contact field leaked: %s", encoded)
	}
}

func TestMineSearchUnionsAndDeduplicates(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		filter := body["filter"].(map[string]any)
		items := []any{
			map[string]any{"token": "shared", "meta_data": map[string]any{"avatar": "remove"}},
		}
		if filter["owner_ids"] != nil {
			items = append([]any{map[string]any{"token": "owned"}}, items...)
		} else {
			items = append(items, map[string]any{"token": "participated"})
		}
		mu.Lock()
		calls++
		mu.Unlock()
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"items": items, "has_more": false}})
	}))
	defer server.Close()
	api := &client{baseURL: server.URL, token: "test", tokenSource: "test", httpClient: server.Client()}
	items, truncated, _, err := searchMinutes(api, "", nil, nil, "ou_me", []timeWindow{{}}, 30, 20)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(items) != 3 || calls != 2 {
		t.Fatalf("items=%v truncated=%v calls=%d", items, truncated, calls)
	}
	firstMeta := items[1].(map[string]any)["meta_data"].(map[string]any)
	if _, exists := firstMeta["avatar"]; exists {
		t.Fatalf("avatar was not removed")
	}
}

func TestSearchFollowsPageToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextPage := r.URL.Query().Get("page_token") == "next"
		token := "first"
		if nextPage {
			token = "second"
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
			"items":      []any{map[string]any{"token": token}},
			"has_more":   !nextPage,
			"page_token": map[bool]string{true: "", false: "next"}[nextPage],
		}})
	}))
	defer server.Close()
	api := &client{baseURL: server.URL, token: "test", tokenSource: "test", httpClient: server.Client()}
	items, truncated, _, err := searchMinutes(api, "周会", nil, nil, "", []timeWindow{{}}, 30, 20)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(items) != 2 || searchItemToken(items[0]) != "first" || searchItemToken(items[1]) != "second" {
		t.Fatalf("items=%v truncated=%v", items, truncated)
	}
}

func TestDocSearchBuildsFiltersAndPaginates(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/search/v2/doc_wiki/search" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		if body["page_token"] == "next" {
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
				"res_units": []any{map[string]any{"url": "https://example/docx/second", "title": "第二场"}}, "has_more": false,
			}})
			return
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
			"res_units": []any{map[string]any{"url": "https://example/docx/first", "title": "第一场"}},
			"has_more":  true, "page_token": "next",
		}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	result, err := commandDocSearch([]string{
		"--query", "周会", "--start", "2026-09-01", "--end", "2026-09-09",
		"--mine", "--user-open-id", "ou_me", "--doc-types", "docx,wiki", "--only-title",
	})
	if err != nil {
		t.Fatal(err)
	}
	object := result.(map[string]any)
	if object["count"] != 2 || object["pages_fetched"] != 2 {
		t.Fatalf("result=%v", result)
	}
	if len(bodies) != 2 || bodies[1]["page_token"] != "next" {
		t.Fatalf("bodies=%v", bodies)
	}
	filter := bodies[0]["doc_filter"].(map[string]any)
	if filter["sort_type"] != "CREATE_TIME" || filter["only_title"] != true {
		t.Fatalf("filter=%v", filter)
	}
	creators := filter["creator_ids"].([]any)
	if len(creators) != 1 || creators[0] != "ou_me" {
		t.Fatalf("creator_ids=%v", creators)
	}
	created := filter["create_time"].(map[string]any)
	if numericCode(created["start"]) == 0 || numericCode(created["end"]) == 0 {
		t.Fatalf("create_time=%v", created)
	}
	docTypes := filter["doc_types"].([]any)
	if len(docTypes) != 2 || docTypes[0] != "DOCX" || docTypes[1] != "WIKI" {
		t.Fatalf("doc_types=%v", docTypes)
	}
	if _, ok := bodies[0]["wiki_filter"]; !ok {
		t.Fatal("wiki_filter missing")
	}
}

func TestDocSearchMineRequiresOpenID(t *testing.T) {
	setTestEnv(t, "http://127.0.0.1:1")
	t.Setenv("FEISHU_USER_OPEN_ID", "")
	_, err := commandDocSearch([]string{"--mine"})
	if err == nil || !strings.Contains(err.Error(), "FEISHU_USER_OPEN_ID") {
		t.Fatalf("err=%v", err)
	}
}

func TestShowExportsTranscript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"summary": "摘要"}})
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			if r.URL.Query().Get("need_speaker") != "true" || r.URL.Query().Get("need_timestamp") != "true" {
				t.Fatalf("missing transcript options")
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("[00:01] 张三：开始"))
		default:
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"minute": map[string]any{"title": "周会", "note_id": "note1"}}})
		}
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	output := filepath.Join(t.TempDir(), "transcript.txt")
	result, err := commandShow([]string{"obctest", "--artifacts", "summary,transcript", "--output", output})
	if err != nil {
		t.Fatal(err)
	}
	object := result.(map[string]any)
	if object["note_id"] != "note1" {
		t.Fatalf("note_id=%v", object["note_id"])
	}
	content, err := os.ReadFile(output)
	if err != nil || !strings.Contains(string(content), "张三") {
		t.Fatalf("transcript=%q err=%v", content, err)
	}
}

func TestShowFallsBackToArtifactTranscript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"transcript": "备用逐字稿"}})
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			writeTestJSON(t, w, 403, map[string]any{"code": 99991672, "msg": "missing scope"})
		default:
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"minute": map[string]any{"title": "测试"}}})
		}
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	output := filepath.Join(t.TempDir(), "fallback.txt")
	result, err := commandShow([]string{"obctest", "--artifacts", "transcript", "--output", output})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := result.(map[string]any)["artifacts"].(map[string]any)
	transcript := artifacts["transcript"].(map[string]any)
	if transcript["source"] != "artifacts_fallback" {
		t.Fatalf("source=%v", transcript["source"])
	}
	content, _ := os.ReadFile(output)
	if string(content) != "备用逐字稿" {
		t.Fatalf("content=%q", content)
	}
}

func TestShowTranscriptDoesNotReportUnusedArtifactFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			writeTestJSON(t, w, 403, map[string]any{"code": 99991672, "msg": "missing artifacts scope"})
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("逐字稿可用"))
		default:
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"minute": map[string]any{"title": "测试"}}})
		}
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	output := filepath.Join(t.TempDir(), "transcript.txt")
	result, err := commandShow([]string{"obctest", "--artifacts", "transcript", "--output", output})
	if err != nil {
		t.Fatal(err)
	}
	errorsList := result.(map[string]any)["errors"].([]any)
	if len(errorsList) != 0 {
		t.Fatalf("unexpected errors=%v", errorsList)
	}
}

func TestEvidenceBuildsManifestAndNoteDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
				"summary": "摘要", "minute_todos": []any{map[string]any{"task": "跟进"}},
			}})
		case strings.HasSuffix(r.URL.Path, "/transcript"):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("[00:01] 张三：开始"))
		case strings.Contains(r.URL.Path, "/vc/v1/notes/"):
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"note": map[string]any{
				"note_display_type": 1,
				"artifacts":         []any{map[string]any{"artifact_type": 1, "doc_token": "note_doc"}},
				"references":        []any{},
			}}})
		case strings.Contains(r.URL.Path, "/docx/v1/documents/note_doc/raw_content"):
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"content": "智能纪要正文"}})
		default:
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
				"minute": map[string]any{"title": "周会", "note_id": "note1"},
			}})
		}
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	directory := t.TempDir()
	result, err := commandEvidence([]string{"obctest", "--output-dir", directory, "--overwrite"})
	if err != nil {
		t.Fatal(err)
	}
	bundle := result.(map[string]any)
	if _, err := os.Stat(bundle["manifest_file"].(string)); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "transcript.txt")); err != nil {
		t.Fatalf("transcript: %v", err)
	}
	documents := bundle["documents"].([]any)
	if len(documents) != 1 {
		t.Fatalf("documents=%v", documents)
	}
	content, err := os.ReadFile(documents[0].(map[string]any)["file"].(string))
	if err != nil || string(content) != "智能纪要正文" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestWikiDocumentResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/spaces/get_node") {
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_type": "docx", "obj_token": "docx123"}}})
			return
		}
		if !strings.Contains(r.URL.Path, "/docx/v1/documents/docx123/raw_content") {
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"content": "关联资料正文"}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	output := filepath.Join(t.TempDir(), "doc.txt")
	result, err := commandDoc([]string{"https://team.feishu.cn/wiki/wiki123", "--output", output})
	if err != nil {
		t.Fatal(err)
	}
	document := result.(map[string]any)["document"].(map[string]any)
	if document["resolved_token"] != "docx123" {
		t.Fatalf("resolved=%v", document)
	}
	content, _ := os.ReadFile(output)
	if string(content) != "关联资料正文" {
		t.Fatalf("content=%q", content)
	}
}

func TestNoteRelationships(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"note": map[string]any{
			"note_display_type": 1,
			"artifacts": []any{
				map[string]any{"artifact_type": 1, "doc_token": "main_doc"},
				map[string]any{"artifact_type": 2, "doc_token": "verbatim_doc"},
			},
			"references": []any{map[string]any{"doc_token": "shared_doc"}},
		}}})
	}))
	defer server.Close()
	api := &client{baseURL: server.URL, token: "test", tokenSource: "test", httpClient: server.Client()}
	detail, err := fetchNoteDetail(api, "note123")
	if err != nil {
		t.Fatal(err)
	}
	if detail["note_display_type"] != "normal" || detail["note_doc_token"] != "main_doc" || detail["verbatim_doc_token"] != "verbatim_doc" {
		t.Fatalf("detail=%v", detail)
	}
}

func TestUnifiedNoteTranscriptPaginatesAndSaves(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/unified_note_transcript") {
			if r.URL.Query().Get("cursor_id") == "2" {
				writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
					"transcript": map[string]any{"markdown": "第二页"}, "has_more": false,
				}})
				return
			}
			writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
				"transcript": map[string]any{"markdown": "第一页"}, "has_more": true, "next_cursor_id": "2",
			}})
			return
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{"note": map[string]any{
			"note_display_type": 2, "artifacts": []any{}, "references": []any{},
		}}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	output := filepath.Join(t.TempDir(), "unified.md")
	result, err := commandNoteTranscript([]string{"note123", "--output", output})
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := os.ReadFile(output)
	if readErr != nil || string(content) != "第一页第二页" {
		t.Fatalf("content=%q err=%v result=%v", content, readErr, result)
	}
}

func TestOAuthAuthorizationURLUsesPKCEAndScope(t *testing.T) {
	target, err := oauthAuthorizationURL(
		"https://accounts.feishu.cn", "cli_test", "http://127.0.0.1:8080/callback",
		"minutes:minutes.search:read", "state-value", "challenge-value",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/open-apis/authen/v1/authorize" || query.Get("client_id") != "cli_test" {
		t.Fatalf("target=%s", target)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != "challenge-value" {
		t.Fatalf("pkce query=%v", query)
	}
	if query.Get("scope") != "minutes:minutes.search:read" || query.Get("state") != "state-value" {
		t.Fatalf("oauth query=%v", query)
	}
}

func TestExchangeOAuthCodeUsesV3FormAndParsesTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v3/token" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("request=%s content-type=%s", r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_secret") != "secret-value" || r.Form.Get("code_verifier") != "verifier-value" {
			t.Fatalf("form=%v", r.Form)
		}
		writeTestJSON(t, w, 200, map[string]any{
			"code": 0, "access_token": "user-token", "expires_in": 7200,
			"refresh_token": "refresh-token", "refresh_token_expires_in": 604800,
			"scope": "minutes:minutes.search:read offline_access",
		})
	}))
	defer server.Close()
	token, err := exchangeOAuthCode(
		server.Client(), server.URL, "cli_test", "secret-value", "code-value",
		"http://127.0.0.1:8080/callback", "verifier-value", "minutes:minutes.search:read offline_access",
	)
	if err != nil {
		t.Fatal(err)
	}
	if token.accessToken != "user-token" || token.expiresIn != 7200 || token.refreshTokenExpires != 604800 {
		t.Fatalf("token=%+v", token)
	}
}

func TestRefreshOAuthTokenUsesV3FormAndRotatesTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v3/token" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("request=%s content-type=%s", r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("form=%v", r.Form)
		}
		writeTestJSON(t, w, 200, map[string]any{
			"code": 0, "access_token": "new-user-token", "expires_in": 7200,
			"refresh_token": "new-refresh", "refresh_token_expires_in": 604800,
		})
	}))
	defer server.Close()
	token, err := refreshOAuthToken(server.Client(), server.URL, "cli_test", "secret-value", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if token.accessToken != "new-user-token" || token.refreshToken != "new-refresh" {
		t.Fatalf("token=%+v", token)
	}
}

func TestClientAutoRefreshesExpiredPersonalToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v3/token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		writeTestJSON(t, w, 200, map[string]any{
			"code": 0, "access_token": "fresh-user-token", "expires_in": 7200,
			"refresh_token": "rotated-refresh", "refresh_token_expires_in": 604800,
		})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	payload := map[string]any{
		"app_id": "cli_test", "app_secret": "secret-value", "api_base": server.URL, "oauth_base": server.URL,
		"user_access_token": "expired-token", "user_access_token_expires_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"refresh_token": "old-refresh", "refresh_token_expires_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_CONFIG_FILE", path)
	for _, key := range []string{
		"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_USER_ACCESS_TOKEN", "FEISHU_TENANT_ACCESS_TOKEN",
		"FEISHU_ACCESS_TOKEN", "FEISHU_API_BASE", "FEISHU_OAUTH_BASE",
	} {
		t.Setenv(key, "")
	}
	api, err := newClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if api.token != "fresh-user-token" || !api.tokenAutoRefreshed || !api.refreshTokenAvailable {
		t.Fatalf("api=%+v", api)
	}
	config, _, err := readFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.UserAccessToken != "fresh-user-token" || config.RefreshToken != "rotated-refresh" {
		t.Fatalf("config=%+v", config)
	}
}

func TestOAuthRedirectMustBeLoopback(t *testing.T) {
	if _, err := validateLoopbackRedirect("https://example.com/callback"); err == nil {
		t.Fatal("expected non-loopback redirect to be rejected")
	}
	if _, err := validateLoopbackRedirect("http://127.0.0.1:8080/callback"); err != nil {
		t.Fatal(err)
	}
}

func TestDriveMeetingsGroupsTodayDocuments(t *testing.T) {
	date := "2026-09-09"
	created := time.Date(2026, 9, 9, 14, 35, 0, 0, time.Local).Unix()
	yesterday := time.Date(2026, 9, 8, 20, 0, 0, 0, time.Local).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/drive/v1/files" || r.URL.Query().Get("order_by") != "CreatedTime" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
			"files": []any{
				map[string]any{"name": "智能纪要：抽奖活动相关功能方案讨论 2026年9月9日", "token": "summary1", "type": "docx", "url": "https://example.feishu.cn/docx/summary1", "created_time": created, "modified_time": created + 60},
				map[string]any{"name": "文字记录：抽奖活动相关功能方案讨论 2026年9月9日", "token": "transcript1", "type": "docx", "url": "https://example.feishu.cn/docx/transcript1", "created_time": created - 120, "modified_time": created},
				map[string]any{"name": "我的笔记：抽奖活动相关功能方案讨论 2026年9月9日", "token": "note1", "type": "docx", "url": "https://example.feishu.cn/docx/note1", "created_time": created - 120, "modified_time": created},
				map[string]any{"name": "智能纪要：昨天的会议 2026年9月8日", "token": "old", "type": "docx", "created_time": yesterday},
				map[string]any{"name": "文字记录：soundcore Work_09-09 10:40 2026年9月9日", "token": "short", "type": "docx", "created_time": created},
				map[string]any{"name": "普通文档", "token": "other", "type": "docx", "created_time": created},
			}, "has_more": false,
		}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	result, err := commandDriveMeetings([]string{"--date", date})
	if err != nil {
		t.Fatal(err)
	}
	meetings := result.(map[string]any)["meetings"].([]map[string]any)
	if len(meetings) != 1 || meetings[0]["title"] != "抽奖活动相关功能方案讨论" {
		t.Fatalf("meetings=%v", meetings)
	}
	if len(meetings[0]["resources"].([]any)) != 3 {
		t.Fatalf("resources=%v", meetings[0]["resources"])
	}
	if result.(map[string]any)["incomplete_count"] != 1 {
		t.Fatalf("incomplete=%v", result.(map[string]any)["incomplete_recordings"])
	}
}

func TestDriveMeetingsRejectsApplicationIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"app_id":"cli_test","app_secret":"secret-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEISHU_CONFIG_FILE", path)
	for _, key := range []string{"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_USER_ACCESS_TOKEN", "FEISHU_TENANT_ACCESS_TOKEN", "FEISHU_ACCESS_TOKEN"} {
		t.Setenv(key, "")
	}
	_, err := commandDriveMeetings([]string{"--date", "2026-09-09"})
	if err == nil || !strings.Contains(err.Error(), "user_access_token") {
		t.Fatalf("expected user identity error, got %v", err)
	}
}

func TestPersonalMeetingsDefaultsToRecentThirtyDaysAndFiltersTitle(t *testing.T) {
	now := time.Now().In(time.Local)
	recent := now.AddDate(0, 0, -5).Unix()
	old := now.AddDate(0, 0, -40).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(t, w, 200, map[string]any{"code": 0, "data": map[string]any{
			"files": []any{
				map[string]any{"name": "智能纪要：新客户方案讨论 2026年9月4日", "token": "recent", "type": "docx", "url": "https://example/docx/recent", "created_time": recent},
				map[string]any{"name": "智能纪要：内部周会 2026年9月4日", "token": "internal", "type": "docx", "url": "https://example/docx/internal", "created_time": recent},
				map[string]any{"name": "智能纪要：旧客户方案讨论 2026年7月1日", "token": "old", "type": "docx", "url": "https://example/docx/old", "created_time": old},
			}, "has_more": false,
		}})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)
	result, err := commandPersonalMeetings([]string{"--query", "客户"})
	if err != nil {
		t.Fatal(err)
	}
	meetings := result.(map[string]any)["meetings"].([]map[string]any)
	if len(meetings) != 1 || meetings[0]["title"] != "新客户方案讨论" {
		t.Fatalf("meetings=%v", meetings)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	defaultAPIBase           = "https://open.feishu.cn"
	defaultTimeoutSeconds    = 30
	maxSearchPages           = 200
	maxDocSearchPages        = 100
	maxNoteTranscriptPages   = 500
	processingCode           = 2091003
	noMinutePermissionCode   = 2091005
	missingScopeCode         = 99991672
	missingUserScopeCode     = 99991679
	userAgent                = "feishu-meeting-memory/1.0"
	defaultSearchResultLimit = 20
)

type cliError struct {
	Kind    string
	Message string
}

func (e *cliError) Error() string { return e.Message }

func validationError(format string, args ...any) error {
	return &cliError{Kind: "validation_error", Message: fmt.Sprintf(format, args...)}
}

func configurationError(format string, args ...any) error {
	return &cliError{Kind: "missing_configuration", Message: fmt.Sprintf(format, args...)}
}

func emitJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "用法: feishu-meetings <command> [arguments]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "只读命令:")
	fmt.Fprintln(w, "  doctor                         检查配置与鉴权")
	fmt.Fprintln(w, "  permissions                    输出完整个人只读部署权限清单")
	fmt.Fprintln(w, "  oauth-login [flags]            浏览器授权用户身份并写入本机配置")
	fmt.Fprintln(w, "  meetings [flags]               查询个人云盘最近 30 天的录音会议")
	fmt.Fprintln(w, "  user-search [flags]             按姓名查找人员 open_id（用户身份）")
	fmt.Fprintln(w, "  search [flags]                 搜索飞书妙记")
	fmt.Fprintln(w, "  doc-search [flags]             搜索云文档与知识库页面")
	fmt.Fprintln(w, "  drive-meetings [flags]         从个人云盘文件清单发现录音豆会议")
	fmt.Fprintln(w, "  show <minute_token|URL> [flags] 读取妙记与指定产物")
	fmt.Fprintln(w, "  evidence <minute_token|URL> [flags] 生成单场会议证据包")
	fmt.Fprintln(w, "  note <note_id>                 读取智能纪要关联文档")
	fmt.Fprintln(w, "  note-transcript <note_id> [flags] 读取 unified 逐字稿")
	fmt.Fprintln(w, "  doc <URL|token> [flags]        读取 Docx/Wiki/旧版 Doc")
	fmt.Fprintln(w, "  meeting <meeting_id> [flags]   读取视频会议与录制关系")
}

func run(args []string) (any, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(os.Stdout)
		return nil, nil
	}
	switch args[0] {
	case "doctor":
		return commandDoctor(args[1:])
	case "permissions":
		return commandPermissions(args[1:])
	case "oauth-login":
		return commandOAuthLogin(args[1:])
	case "meetings":
		return commandPersonalMeetings(args[1:])
	case "user-search":
		return commandUserSearch(args[1:])
	case "search":
		return commandSearch(args[1:])
	case "doc-search":
		return commandDocSearch(args[1:])
	case "drive-meetings":
		return commandDriveMeetings(args[1:])
	case "show":
		return commandShow(args[1:])
	case "evidence":
		return commandEvidence(args[1:])
	case "note":
		return commandNote(args[1:])
	case "note-transcript":
		return commandNoteTranscript(args[1:])
	case "doc":
		return commandDoc(args[1:])
	case "meeting":
		return commandMeeting(args[1:])
	default:
		return nil, validationError("未知命令 %q；运行 --help 查看用法", args[0])
	}
}

func exitCodeFor(err error) int {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.PermissionError() {
			return 5
		}
		return 4
	}
	var localErr *cliError
	if errors.As(err, &localErr) {
		if localErr.Kind == "missing_configuration" {
			return 3
		}
		return 2
	}
	return 1
}

func errorPayload(err error) map[string]any {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.Map()
	}
	var localErr *cliError
	if errors.As(err, &localErr) {
		return map[string]any{"type": localErr.Kind, "message": localErr.Message}
	}
	return map[string]any{"type": "internal_error", "message": err.Error()}
}

func main() {
	payload, err := run(os.Args[1:])
	if err != nil {
		emitJSON(os.Stderr, map[string]any{"ok": false, "error": errorPayload(err)})
		os.Exit(exitCodeFor(err))
	}
	if payload != nil {
		emitJSON(os.Stdout, map[string]any{"ok": true, "data": payload})
	}
}

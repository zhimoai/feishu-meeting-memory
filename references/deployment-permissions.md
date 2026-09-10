# 完整部署权限清单

这是个人授权版 `feishu-meeting-memory` 的权限来源文件。部署人员应先运行 `feishu-meetings permissions` 查看当前程序内置清单，再与本文核对。

## 完整个人只读权限

完整会议发现、智能纪要、妙记 AI 产物和原始转写稿需要以下 OAuth scopes：

| 权限 | 用途 | 是否必需 |
|---|---|---|
| `offline_access` | 保存 refresh token，自动续期个人授权 | 必需 |
| `space:document:retrieve` | 枚举当前用户个人云盘中的录音豆文档 | 必需 |
| `docx:document:readonly` | 读取智能纪要、文字记录和笔记正文 | 必需 |
| `search:docs:read` | 搜索当前用户可见的云文档和知识库 | 必需 |
| `minutes:minutes.search:read` | 搜索当前用户可见妙记并取得 minute token | 必需 |
| `minutes:minutes.basic:read` | 读取妙记标题、时间、时长和关联 Note | 必需 |
| `minutes:minutes.artifacts:read` | 读取总结、待办、章节和关键词 | 必需 |
| `minutes:minutes.transcript:export` | 导出带说话人和时间戳的原始转写稿 | 必需 |
| `vc:note:read` | 解析妙记关联的智能纪要与共享资料关系 | 必需 |
| `wiki:node:retrieve` | 把 Wiki 链接解析到底层 Docx | 必需 |

以上都是读取或授权续期能力，不包含修改文档、下载原始音视频、发送消息、创建任务或管理协作者。

完整权限字符串：

```text
space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access
```

## 可选权限

| 权限 | 何时需要 |
|---|---|
| `contact:user:search` | 用户按姓名查找某人的会议，需要把姓名解析为 open_id 时 |
| `vc:meeting.meetingevent:read` | 输入飞书视频会议 meeting_id 并读取会议元信息时 |
| `vc:record:readonly` | 从视频会议定位录制和妙记时 |

录音豆本地录制通常没有 `meeting_id`，所以后两项不属于默认部署权限。

## 应用后台配置

1. 在“权限管理”开通上述完整个人只读权限。
2. 在“安全设置 → 重定向 URL”添加 `http://127.0.0.1:8080/callback`。
3. 把授权用户加入应用可用范围。
4. 创建并发布新版本，完成管理员审批。
5. 每位用户运行 `oauth-login --full`，使用自己的飞书账号授权。

新增权限只发布应用版本还不够；已经授权过的用户必须重新 OAuth，新的 user scope 才会进入个人令牌。

## 应用类型边界

- 支持基线为企业自建应用与个人 `user_access_token`；发布前可按下方部署验收流程验证实际租户配置。
- 部分妙记接口的官方页面标注仅支持企业自建应用。采用商店应用时必须逐接口验证，不能默认妙记搜索、AI 产物和逐字稿全部可用。
- 不使用妙记接口时，个人云盘 Docx 仍可作为主链路：`meetings` 发现资料，`doc` 读取智能纪要和文字记录。
- 个人账号不能脱离应用直接取得 OpenAPI token；公共或大规模分发应由 OAuth 网关保管 App Secret。

## 部署验收

先查看程序内置权限清单：

```powershell
& .\scripts\feishu-meetings.ps1 permissions
```

完成用户授权：

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

使用一条无敏感内容的妙记和智能纪要验证：

```powershell
& .\scripts\feishu-meetings.ps1 doctor --probe --minute '<妙记URL或minute_token>' --transcript --doc '<智能纪要URL>'
```

以下探测应全部为 `ok: true`：

- `minutes_search`
- `cloud_documents_search`
- `minute_basic`
- `minute_artifacts`
- `minute_transcript`
- `document_content`

`refresh_token_saved` 也必须为 `true`，否则 access token 过期后需要用户重新登录。

## 常见权限错误

| 错误 | 含义 | 处理 |
|---|---|---|
| 20027 | 应用没有 `offline_access` | 开通“持续访问已授权的数据”，发布后重新 OAuth |
| 20029 | 重定向 URL 不匹配 | 后台地址必须与命令使用的完整 URL 一致 |
| 99991672 | 应用没有所需接口权限 | 管理员开通权限、发布版本并审批 |
| 99991679 | 用户令牌没有所需 user scope | 应用发布后让当前用户重新 OAuth |
| 2091005 或 403 | 当前身份没有单条妙记/文档资源权限 | 由资源所有者通过飞书正常分享流程授权 |

官方参考：[获取访问凭证](https://open.larkoffice.com/document/server-docs/api-call-guide/calling-process/get-access-token)、[获取妙记 AI 产物](https://open.larkoffice.com/document/uAjLw4CM/ukTMukTMukTM/minutes-v1/minute/artifacts)、[获取新版云文档纯文本](https://open.larkoffice.com/document/server-docs/docs/docs/docx-v1/document/raw_content)。

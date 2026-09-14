# 飞书会议知识提取（Codex Skill）

`feishu-meeting-memory` 是一个面向 Codex 的只读 Skill，用个人飞书 OAuth 身份查找录音豆生成的妙记、智能纪要、文字记录和关联云文档，并提取会议结论、明确决定、行动项与带时间戳的原话。

运行 Skill 不需要安装 Python、Node.js、Go 或飞书 CLI。仓库包含适用于 Windows、macOS 和 Linux 的 64 位独立程序。每位使用者通过自己的飞书账号完成 OAuth，数据可见范围与该账号在飞书中的原有权限一致。

## 能做什么

- 查询今天、最近或指定日期范围内的录音会议。
- 按标题、主题、客户、项目或人员定位候选会议。
- 读取智能纪要、文字记录、妙记 AI 总结、待办、章节和关键词。
- 导出带说话人和时间戳的 TXT/SRT 原始转写稿。
- 提取核心结论、明确决定、负责人、截止时间和待确认事项。
- 结合多场会议整理项目脉络、周报或跨会议对比。
- 返回飞书原始链接，让用户在自己的飞书账号权限下继续查看。

Skill 默认只读：不会修改云文档、发送消息、申请文档权限，也不会自动下载原始音视频。

## 工作方式

```text
录音豆
  └─► 飞书妙记 / 智能纪要 / 文字记录
          └─► 当前用户完成飞书 OAuth
                  └─► Codex 调用本 Skill
                          └─► 会议列表、总结、行动项、原话证据
```

App ID 用于标识飞书应用，个人 OAuth 令牌用于标识当前使用者。接口权限和单条会议/文档的访问权限是两道独立的限制：Skill 只能读取当前登录用户原本有权查看的资料。

## 授权模式

### 同一组织使用

一个飞书组织只需由管理员创建并发布一个企业自建应用。管理员把成员加入应用可用范围并安全预置应用凭据后，成员不需要创建应用；安装 Skill 后首次查询会议时，浏览器会自动打开飞书授权页。每位成员仍需登录自己的账号并同意授权，Skill 只读取该账号原本可见的资料。

### 独立使用

不属于上述组织的使用者，需要由所在组织的管理员创建自己的飞书应用。公共仓库不会包含任何 App Secret，也不会共用项目维护者的用户令牌。

当前本地架构在换取和刷新用户令牌时需要 App Secret。若要面向组织外用户提供“安装后只点授权”的体验，需要部署服务端 OAuth 网关；部分妙记接口仅支持企业自建应用，采用商店应用前还需逐接口确认兼容性。

## 支持平台

| 操作系统 | CPU 架构 | 最终用户需要额外运行环境 |
|---|---|---|
| Windows | x64、ARM64 | 不需要 |
| macOS | Intel、Apple Silicon | 不需要 |
| Linux | x64、ARM64 | 不需要 |

预编译文件的校验值位于 [`bin/SHA256SUMS`](bin/SHA256SUMS)。这些程序目前没有商业代码签名；受企业终端策略限制时，应从源码重建并由企业签名。

## 安装与更新

### 首次安装

在 Codex 中发送：

```text
请使用 $skill-installer 安装这个 GitHub 仓库中的 Skill：
https://github.com/zhimoai/feishu-meeting-memory
```

Codex 会把完整 Skill 安装到用户级 Skill 目录。若安装后没有立即出现，重启 Codex，再输入 `$feishu-meeting-memory` 检查是否可以选中。

也可以手动把整个仓库复制到用户级 Skill 目录：

- Windows：`%USERPROFILE%\.codex\skills\feishu-meeting-memory`
- macOS/Linux：`$HOME/.codex/skills/feishu-meeting-memory`

不能只复制 `SKILL.md`；`agents/`、`bin/`、`references/` 和 `scripts/` 都是运行所需内容。完整步骤见[首次安装与授权指引](references/installation.md)。

### 更新已安装的 Skill

`config.json` 保存本机的 App Secret 和个人 OAuth 令牌，更新时必须保留。推荐在 Codex 中发送：

```text
请更新已安装的 feishu-meeting-memory Skill：
https://github.com/zhimoai/feishu-meeting-memory

更新前把现有 config.json 安全备份到 Skill 目录之外，不要读取或输出其中内容；
安装最新版本后恢复 config.json，运行 doctor 验证；
只有验证成功后才能删除备份，失败则恢复原版本。
```

通过 Git 克隆安装时，也可以在 Skill 目录执行 `git pull --ff-only`。`config.json` 已被 Git 忽略，正常更新不会覆盖它；更新前仍建议保留一份安全备份。更新完成后运行 `doctor`。若新版增加了 OAuth scope，还需要在飞书后台开通权限、发布应用版本，并重新执行 `oauth-login --full`。

## 管理员一次性准备飞书应用

同一个飞书组织只需由管理员准备一个企业自建应用，普通成员不需要分别创建应用。管理员只做一次以下操作：

1. 在[飞书开放平台开发者后台](https://open.feishu.cn/app)创建企业自建应用。
2. 开通会议搜索、纪要、逐字稿、云文档读取和自动续期所需的只读权限。
3. 在“安全设置 → 重定向 URL”添加 `http://127.0.0.1:8080/callback`。
4. 发布应用，并把实际使用者加入应用可用范围。
5. 通过企业设备管理或其他安全渠道，为成员预置只含应用信息、不含任何个人 token 的 `config.json`。

详细点击路径、权限清单和管理员验收方法见[管理员一次性配置](references/deployment-permissions.md)。官方入门资料见[企业自建应用开发流程](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process)。

应用新增权限后需要重新发布，已授权成员也需再次授权。飞书访问凭证的身份区别见[官方访问凭证说明](https://open.feishu.cn/document/server-docs/api-call-guide/calling-process/get-access-token)。

## 首次配置与授权

### 组织成员

管理员应通过企业设备管理或其他安全渠道，将只包含 App ID/Secret、尚未写入任何个人 token 的 `config.json` 放到 Skill 根目录。成员安装后可直接在 Codex 中发起会议请求：

```text
$feishu-meeting-memory 最近开了哪些会？
```

Skill 检测到尚未授权时会自动打开浏览器。使用者登录自己的飞书账号并同意授权后，原会议请求会继续执行。组织成员无需进入飞书开发者后台，也无需手工填写或复制 token。

### 独立部署者

复制 [`config.example.json`](config.example.json)，把副本命名为 `config.json`。用文本编辑器打开，只替换 `app_id` 和 `app_secret`：

```json
{
  "app_id": "这里填写 App ID",
  "app_secret": "这里填写 App Secret"
}
```

不要删除示例中其余字段，也不要手工填写 token。JSON 必须使用英文双引号，最后一个字段后面不能多逗号。

在 Codex 中发起第一次会议请求即可自动进入 OAuth。直接使用命令行时，可手工运行：

Windows：

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

macOS/Linux：

```bash
sh ./scripts/feishu-meetings.sh oauth-login --full
```

命令会自动打开浏览器。登录录音设备绑定的飞书账号并同意授权。

### 检查授权状态

Windows：

```powershell
& .\scripts\feishu-meetings.ps1 doctor
```

macOS/Linux：

```bash
sh ./scripts/feishu-meetings.sh doctor
```

看到顶层 `"ok": true`，并确认 `token_source` 为 `user_access_token`、`refresh_token_saved` 为 `true`，即表示配置与授权正常。

Skill 根目录中现在有：

```text
feishu-meeting-memory/
├── config.example.json    # 可以公开的填写示例
└── config.json            # 本机敏感配置，不可分享或提交
```

`config.json` 已在 `.gitignore` 中排除，但它会保存 App Secret、user access token 和 refresh token。组织内分发时只允许发送尚未授权、不含个人 token 的初始配置；完成 OAuth 后的配置只能留在本人设备上。独立部署者应从干净的仓库副本创建自己的配置。

配置文件其他字段通常不需要修改：

| 字段 | 是否必填 | 说明 |
|---|---|---|
| `app_id` | 是 | “凭证与基础信息”页面中的 App ID |
| `app_secret` | 是 | 同一页面中的 App Secret |
| `oauth_redirect_uri` | 是 | 必须与后台重定向 URL 完全一致 |
| `oauth_scope` | 是 | 默认完整只读权限，通常无需修改 |
| `api_base` | 是 | 中国版飞书保持 `https://open.feishu.cn` |
| `http_timeout_seconds` | 否 | 请求超时秒数，默认 30 |

不要手工填写 `user_access_token` 或 `refresh_token`。第一次执行 `oauth-login --full` 后，程序会自动把这些字段写回 `config.json`。

完整授权包含 `offline_access`。保存 refresh token 后，程序会在 access token 临近过期时自动刷新；只有授权失效、用户更换账号或应用权限发生变化时才需要重新登录。

## 验证

首次授权完成后，重新执行原会议提问就是最直接的验证：

```text
$feishu-meeting-memory 最近开了哪些会？
```

如果需要排查授权状态，再运行：

```powershell
& .\scripts\feishu-meetings.ps1 doctor
```

看到顶层 `"ok": true` 即表示本机配置与授权正常。接口逐项探测和测试资源验收只用于管理员部署或故障排查，见[管理员一次性配置](references/deployment-permissions.md)。

## 在 Codex 中使用

可以直接提问，也可以显式点名 Skill：

```text
$feishu-meeting-memory 今天开了哪些会？
$feishu-meeting-memory 最近关于新客户的会议有哪些？
$feishu-meeting-memory 总结“终端门店扫码发红包方案讨论”，列出决定、负责人和截止时间。
$feishu-meeting-memory 找出客户项目会议中关于报价的原话和时间戳。
$feishu-meeting-memory 把最近十场项目会议整理成进展、风险和下一步。
```

## 命令概览

| 命令 | 用途 |
|---|---|
| `permissions` | 查看完整只读权限、可选权限和 OAuth 回调地址 |
| `doctor` | 检查配置、个人授权和接口能力 |
| `oauth-login --full` | 浏览器登录并保存当前用户授权 |
| `meetings` | 从个人云盘发现并合并录音会议资料 |
| `search` | 搜索当前用户可见的飞书妙记 |
| `doc-search` | 搜索云文档和知识库页面 |
| `show` | 读取妙记详情、AI 产物或原始转写稿 |
| `evidence` | 生成单场会议的完整本地证据包 |
| `doc` | 读取 Docx、Wiki 或旧版 Doc |
| `note` / `note-transcript` | 读取智能纪要和 unified 逐字稿 |
| `meeting` | 读取飞书视频会议与录制关系 |

运行启动脚本并加 `--help` 可查看全部命令。更详细的能力边界见[飞书能力说明](references/feishu-capabilities.md)。

## 已知限制

- 个人飞书账号不能脱离飞书应用直接调用 OpenAPI。
- 部分妙记接口可能只支持特定应用类型；没有妙记 API 能力时，以个人云盘中的智能纪要和文字记录 Docx 为主链路。
- scope 不会绕过飞书中的文档分享、资源 ACL、企业密级、DLP 或跨租户限制。
- 刚结束的录音可能仍在上传、转写或生成纪要，短时间内搜索不到不等于没有开会。
- 共享录音设备不能自动证明实际参会人；没有账号绑定时只能保留“说话人 1”等转写标签。
- 原始稿来自语音识别，可能存在错字、断句或说话人误判。

## 安全说明

- 不要把 App Secret、access token、refresh token 或真实 `config.json` 提交到 Git。
- 不要把一个用户的配置文件复制给其他用户；每个人都应使用自己的飞书账号 OAuth。
- Secret 一旦进入提交历史、Issue、截图或聊天，应立即在飞书开放平台重置，不能只删除文本。
- 会议正文和逐字稿属于敏感数据；默认生成的长文本或证据文件位于操作系统临时目录，应按企业数据策略处理。
- 提交安全问题前请阅读 [`SECURITY.md`](SECURITY.md)，不要在公开 Issue 中粘贴客户会议内容或凭证。

## 开发与验证

只有维护者从源码重建时才需要 Go。最终用户不需要 Go。

```powershell
go test ./...
go vet ./...
& .\scripts\build-binaries.ps1
```

目录结构：

```text
feishu-meeting-memory/
├── SKILL.md                 # Codex 工作流入口
├── config.example.json      # 可公开的配置示例
├── config.json              # 本机敏感配置，已被 Git 忽略
├── agents/openai.yaml       # Skill 展示信息
├── bin/                     # 六个平台独立程序
├── cmd/feishu-meetings/     # Go 源码和测试
├── references/              # 安装、权限、能力和提取规则
└── scripts/                 # 启动与构建脚本
```

## 文档

- [安装、更新与授权](references/installation.md)
- [完整部署权限](references/deployment-permissions.md)
- [配置、身份模型与故障排查](references/setup.md)
- [飞书 API 能力与限制](references/feishu-capabilities.md)
- [会议知识提取规则](references/knowledge-extraction.md)

## 开源许可

本项目采用 [MIT License](LICENSE)。

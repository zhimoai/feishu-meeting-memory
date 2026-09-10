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

## 支持平台

| 操作系统 | CPU 架构 | 最终用户需要额外运行环境 |
|---|---|---|
| Windows | x64、ARM64 | 不需要 |
| macOS | Intel、Apple Silicon | 不需要 |
| Linux | x64、ARM64 | 不需要 |

预编译文件的校验值位于 [`bin/SHA256SUMS`](bin/SHA256SUMS)。这些程序目前没有商业代码签名；受企业终端策略限制时，应从源码重建并由企业签名。

## 快速安装到 Codex

在 Codex 中发送：

```text
请使用 $skill-installer 安装这个 GitHub 仓库中的 Skill：
https://github.com/zhimoai/feishu-meeting-memory
```

Codex 会把完整 Skill 安装到用户级 Skill 目录。若安装后没有立即出现，重启 Codex，再输入 `$feishu-meeting-memory` 检查是否可以选中。

也可以手动把整个仓库复制到：

- Windows：`%USERPROFILE%\.agents\skills\feishu-meeting-memory`
- macOS/Linux：`$HOME/.agents/skills/feishu-meeting-memory`

不能只复制 `SKILL.md`；`agents/`、`bin/`、`references/` 和 `scripts/` 都是运行所需内容。完整步骤见[首次安装与授权指引](references/installation.md)。

## 准备飞书应用

每位使用者需要一个能够发起个人 OAuth 的已发布飞书应用。完整的妙记 API 能力依赖企业自建应用，并要求实际使用者位于应用可用范围。

### 第 1 步：进入开发者后台

1. 打开[飞书开放平台开发者后台](https://open.feishu.cn/app)，使用准备读取会议的飞书账号登录。
2. 点击“创建企业自建应用”。如果看不到这个按钮，说明当前账号可能没有开发应用权限，需要联系所在飞书组织的管理员。
3. 填写应用名称，例如“飞书会议知识提取”，上传图标并填写简单描述，然后创建。

官方入门资料：[企业自建应用开发流程](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process)。

### 第 2 步：复制 App ID 和 App Secret

进入刚创建的应用，在左侧打开“凭证与基础信息”：

1. 复制 `App ID`，通常以 `cli_` 开头。
2. 点击显示并复制 `App Secret`。
3. 将凭据保存在密码管理器中，不得写入截图、群聊或 GitHub。

这两个值稍后写入 Skill 根目录的 `config.json`。App ID 可以公开识别应用，App Secret 必须保密。

### 第 3 步：开通只读权限

在左侧点击“权限管理”，逐项搜索并开通[完整部署权限清单](references/deployment-permissions.md)中的权限，包括云文档读取、妙记搜索与读取、原始转写稿和 `offline_access`。

可在 Skill 目录运行以下命令，查看程序要求的准确 scope：

```powershell
& .\scripts\feishu-meetings.ps1 permissions
```

默认完整授权使用：

```text
space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access
```

权限配置应遵循最小权限原则，不需要开通云文档写入、消息发送等无关能力。若后台显示的中文名称与本文不同，以后台搜索到的 scope 和 API 报错提示为准。

### 第 4 步：添加 OAuth 回调地址

打开左侧“安全设置”，停留在“重定向 URL”页签，添加下面的完整地址：

```text
http://127.0.0.1:8080/callback
```

协议、IP、端口和路径必须一字不差；`localhost`、缺少 `/callback` 或换成其他端口都不是同一个地址。

### 第 5 步：添加测试人员并发布

1. 打开“测试企业和人员”，把准备测试的飞书账号加入测试范围。
2. 打开“版本管理与发布”，点击“创建版本”。
3. 填写版本号和更新说明，确认应用可用范围包含实际使用者。
4. 提交并发布；如果组织要求管理员审批，等待管理员通过。
5. 回到应用首页，确认看到“当前修改均已发布”后再进行 OAuth。

新增权限后，需要再次创建并发布应用版本；已经授权的用户还需重新运行 `oauth-login --full`。飞书访问凭证的身份区别可参考[官方访问凭证说明](https://open.feishu.cn/document/server-docs/api-call-guide/calling-process/get-access-token)。

## 首次配置与授权

首次配置包括三个步骤：准备配置文件、完成飞书授权、检查运行状态。

### 1. 准备配置文件

在安装后的 Skill 根目录中复制 [`config.example.json`](config.example.json)，把副本命名为 `config.json`。用记事本或其他文本编辑器打开，只替换下面两项：

```json
{
  "app_id": "这里填写 App ID",
  "app_secret": "这里填写 App Secret"
}
```

不要删除示例中其余字段，也不要手工填写 token。JSON 必须使用英文双引号，最后一个字段后面不能多逗号。

### 2. 登录并授权

在 Skill 根目录打开终端，运行：

Windows：

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

macOS/Linux：

```bash
sh ./scripts/feishu-meetings.sh oauth-login --full
```

命令会自动打开浏览器。登录录音设备绑定的飞书账号并同意授权即可。

### 3. 检查是否成功

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

`config.json` 已在 `.gitignore` 中排除，但它会保存 App Secret、user access token 和 refresh token。分发时应使用未配置的仓库副本；每位使用者分别创建自己的 `config.json` 并完成 OAuth。

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

## 验证完整流程

Windows 示例：

```powershell
& .\scripts\feishu-meetings.ps1 permissions
& .\scripts\feishu-meetings.ps1 doctor
& .\scripts\feishu-meetings.ps1 meetings --days 30 --limit 10
& .\scripts\feishu-meetings.ps1 doctor --probe
```

如有无敏感内容的测试妙记和智能纪要，可进一步验证详情、AI 产物、逐字稿和文档正文：

```powershell
& .\scripts\feishu-meetings.ps1 doctor --probe `
  --minute '<妙记 URL 或 minute_token>' `
  --transcript `
  --doc '<智能纪要 URL>'
```

验收标准是 `doctor` 显示个人授权、refresh token 和部署权限完整，且六项在线探测均为成功。

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

- [首次安装与授权](references/installation.md)
- [完整部署权限](references/deployment-permissions.md)
- [配置、身份模型与故障排查](references/setup.md)
- [飞书 API 能力与限制](references/feishu-capabilities.md)
- [会议知识提取规则](references/knowledge-extraction.md)

## 开源许可

本项目采用 [MIT License](LICENSE)。

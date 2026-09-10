# 飞书录音会议 Skill 首次安装与授权指引

本指引适用于个人授权版 `feishu-meeting-memory`：每位使用者把录音设备绑定到自己的飞书账号，Skill 通过该用户自己的 OAuth 身份查找和读取会议资料。

最终用户不需要安装 Python、Node.js、Go 或飞书 CLI。Skill 已包含 Windows、macOS 和 Linux 的独立程序。

## 一、先分清两种角色

### 飞书应用管理员：只需准备一次

管理员负责创建应用、开通权限、配置回调地址、发布版本和设置可用范围。

### 普通使用者：每人操作一次

使用者安装 Skill 后，用自己的飞书账号完成浏览器授权。每个人获得独立的用户令牌，只能访问本人原本有权查看的会议资料。

个人账号不能脱离应用直接调用飞书 OpenAPI。当前本机试点需要一个已发布的飞书应用；正式大范围分发时，应使用 OAuth 网关保管 App Secret，不能把 Secret 广泛发到个人电脑。

## 二、管理员准备飞书应用

### 1. 创建或选择应用

当前需要完整使用妙记 API 时，选择企业自建应用。只走云文档流程时可以继续评估商店应用，但要逐项确认目标接口支持该应用类型。

### 2. 开通完整个人只读权限

部署前先查看 [完整部署权限清单](deployment-permissions.md)，也可以在 Skill 目录运行：

```powershell
& .\scripts\feishu-meetings.ps1 permissions
```

在飞书开放平台进入应用的“权限管理”，开通以下权限：

| 用途 | 权限 |
|---|---|
| 持续刷新个人授权 | `offline_access`（持续访问已授权的数据） |
| 枚举个人云盘会议文档 | `space:document:retrieve` 或 `drive:drive:readonly` |
| 读取智能纪要和文字记录 | `docx:document:readonly` |
| 搜索云文档 | `search:docs:read` |
| 搜索本人可见妙记 | `minutes:minutes.search:read` |

完整提取飞书 AI 产物和原始录音稿还必须开通：

| 用途 | 权限 |
|---|---|
| 妙记基础信息 | `minutes:minutes.basic:read` |
| 总结、待办、章节和关键词 | `minutes:minutes.artifacts:read` |
| 导出妙记逐字稿 | `minutes:minutes.transcript:export` |
| 智能纪要关系 | `vc:note:read` |
| 知识库页面 | `wiki:node:retrieve` |
| 联系人姓名检索 | `contact:user:search`（可选） |

飞书后台若显示旧版同义权限，以后台当前权限名和 API 报错建议为准，不要同时申请不需要的写权限。

### 3. 添加重定向 URL

进入“安全设置 → 重定向 URL”，添加以下完整地址：

```text
http://127.0.0.1:8080/callback
```

协议、IP、端口和路径必须完全一致。不要只填 `127.0.0.1`。

### 4. 设置可用范围并发布

1. 把实际试用人员加入应用可用范围。
2. 创建新版本并发布。
3. 确认管理员已批准新增权限。
4. 权限或回调地址有变化时，必须重新发布后再让用户授权。

### 5. 准备试点凭据

本机试点需要 App ID 和 App Secret。只通过安全渠道交给受控测试电脑，不要把 Secret 写进群聊、文档、Skill 包或源码。Secret 一旦出现在截图或聊天中，应在联调结束后重置。

## 三、安装 Skill

### 推荐：从公共 GitHub 仓库安装

在 Codex 中发送：

```text
请使用 $skill-installer 安装这个 GitHub 仓库中的 Skill：
https://github.com/zhimoai/feishu-meeting-memory
```

安装器会下载完整 Skill 目录。安装完成后输入 `$feishu-meeting-memory` 检查是否可以选中；若没有立即出现，重启 Codex。不要在安装提示、仓库 URL 或聊天中附带 GitHub Token、飞书 App Secret 或用户令牌。

### 方式 A：用户级安装

适合在该用户的所有 Codex 项目中使用。

- Windows：`%USERPROFILE%\.agents\skills\feishu-meeting-memory`
- macOS/Linux：`$HOME/.agents/skills/feishu-meeting-memory`

把完整 Skill 目录复制到上述位置。复制后应当直接看到以下内容：

```text
feishu-meeting-memory/
├── SKILL.md
├── agents/
├── bin/
├── references/
└── scripts/
```

不要只复制 `SKILL.md`，也不要形成 `feishu-meeting-memory/feishu-meeting-memory/SKILL.md` 这样的双层目录。

### 方式 B：项目级安装

只希望某个项目使用时，把完整目录放到：

```text
<项目目录>/.agents/skills/feishu-meeting-memory
```

Codex 会自动发现本地 Skill；如果列表中没有出现，重启 Codex。可在 Codex 中输入 `$feishu-meeting-memory` 检查是否能选中。OpenAI 官方说明见 [Build skills](https://developers.openai.com/codex/skills)。

## 四、生成本机配置

在 Skill 目录打开终端。

### Windows

```powershell
& .\scripts\configure.ps1
```

依次输入 App ID 和 App Secret。Secret 输入时不会显示。若 PowerShell 阻止本地脚本，可以只对本次进程放开：

```powershell
Set-ExecutionPolicy -Scope Process Bypass
& .\scripts\configure.ps1
```

### macOS/Linux

```bash
sh ./scripts/configure.sh
```

配置文件保存在用户自己的系统配置目录，不放在 Skill 中：

- Windows：`%APPDATA%\feishu-meeting-memory\config.json`
- macOS：`~/Library/Application Support/feishu-meeting-memory/config.json`
- Linux：`${XDG_CONFIG_HOME:-~/.config}/feishu-meeting-memory/config.json`

不要复制其他用户的配置文件，因为其中可能包含其个人令牌。

## 五、用个人飞书账号授权

### Windows

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

### macOS/Linux

```bash
sh ./scripts/feishu-meetings.sh oauth-login --full
```

浏览器打开后：

1. 登录当前录音设备绑定的飞书账号。
2. 核对授权应用和只读权限。
3. 点击同意。
4. 页面显示“飞书授权成功”后关闭页面并回到终端。

完整授权会申请 refresh token，后续 access token 临近过期时自动刷新。正常情况下不需要每次查询前重新登录。只需要云文档列表、不需要妙记详情和原始转写稿时，才使用不带 `--full` 的基础授权。

应用权限增加、用户切换账号或刷新授权失效后，需要重新运行 OAuth。重新授权会覆盖本机旧的个人令牌。

## 六、验证完整流程

### 1. 检查授权状态

```powershell
& .\scripts\feishu-meetings.ps1 permissions
& .\scripts\feishu-meetings.ps1 doctor
```

应重点确认：

- `token_source` 为 `user_access_token`；
- `refresh_token_saved` 为 `true`；
- 未输出 App Secret、access token 或 refresh token。

### 2. 查询最近会议

```powershell
& .\scripts\feishu-meetings.ps1 meetings --days 30 --limit 10
```

程序会从当前用户个人云盘中识别“智能纪要、文字记录、我的笔记”，按会议合并后返回标题、时间和原始链接。

### 3. 查询今天会议

```powershell
& .\scripts\feishu-meetings.ps1 meetings --date 2026-09-09
```

把日期替换为当天日期。

### 4. 按标题筛选

```powershell
& .\scripts\feishu-meetings.ps1 meetings --days 30 --query '客户'
```

标题筛选不能代替正文语义判断。用户询问“关于新客户的会议”时，Codex 应先列出近期候选，再读取候选智能纪要确认。

### 5. 读取智能纪要正文

从会议结果中取 `kind` 为 `summary` 的 URL：

```powershell
& .\scripts\feishu-meetings.ps1 doc 'https://example.feishu.cn/docx/文档Token'
```

### 6. 验证增强能力

```powershell
& .\scripts\feishu-meetings.ps1 doctor --probe
& .\scripts\feishu-meetings.ps1 search --days 7 --limit 5
```

需要验证具体妙记详情时，使用一条无敏感内容的测试妙记：

```powershell
& .\scripts\feishu-meetings.ps1 doctor --probe --minute '<minute_token或妙记URL>' --transcript --doc '<智能纪要URL>'
```

验证完成的最低标准：能够列出个人会议、打开返回的飞书链接、读取一篇本人有权访问的智能纪要；增强模式还应能读取妙记产物或逐字稿。

## 七、在 Codex 中使用

可以直接提问，也可以显式点名 Skill：

```text
$feishu-meeting-memory 今天开了哪些会？
$feishu-meeting-memory 最近关于新客户的会议有哪些？
$feishu-meeting-memory 总结“终端门店扫码发红包方案讨论”，列出决定、负责人和截止时间。
$feishu-meeting-memory 找出这场会议中关于报价的原话和时间戳。
```

Skill 默认只读取资料，不修改文档、不发送消息、不申请文档权限，也不下载原始音视频。

## 八、常见问题

| 现象或错误 | 原因与处理 |
|---|---|
| 授权页提示重定向 URL 有误，错误码 20029 | 后台重定向 URL 与本机命令不完全一致；确认是 `http://127.0.0.1:8080/callback`，重新发布。 |
| 授权页提示缺少 `offline_access`，错误码 20027 | 在权限管理开通“持续访问已授权的数据”，重新发布后再次 OAuth。 |
| API 返回 99991679 | 当前用户 OAuth 没有对应权限；后台开通并发布后，让该用户重新授权。 |
| API 返回 99991672 | 应用本身缺少接口权限；管理员补权限、发布版本并批准。 |
| 飞书界面能看到，API 读取不到 | scope 与单条文档 ACL 是两层权限；确认授权账号正确，并确认该账号对文档有访问权。 |
| 刚结束录音但列表中没有 | 妙记、智能纪要或文字记录仍在生成；稍后重试。不要把空结果直接解释为没有开会。 |
| 8080 端口被占用 | 关闭占用程序，或者在后台新增另一个完整回调地址，再用 `oauth-login --redirect-uri <地址>`。 |
| Windows 阻止运行程序 | 当前独立程序未做商业代码签名；企业电脑应由内部构建/签名流程处理。不要从未知来源下载替代程序。 |
| 电脑没有 Python | 不受影响；最终用户只运行 Skill 自带的独立程序和系统 PowerShell/shell。 |

## 九、安全检查

- 每个人都使用自己的飞书账号 OAuth，不共享 token。
- 不把 `%APPDATA%\feishu-meeting-memory\config.json` 发给别人。
- 不在聊天、工单或截图中展示 App Secret 和 token。
- App Secret 泄露后立即在飞书开放平台重置。
- 用户离职、设备丢失或授权不再需要时，在飞书中撤销应用授权，并删除该设备上的本地配置。
- 正式多人分发前增加 OAuth 网关，让终端不再保存 App Secret。

## 十、验收清单

### 管理员

- [ ] 已按 `permissions` 输出开通完整个人只读权限。
- [ ] 已开通 `offline_access`。
- [ ] 已添加精确的回调 URL。
- [ ] 已设置用户可用范围。
- [ ] 已发布新版本并批准权限。

### 使用者

- [ ] 录音设备已绑定到本人的飞书账号。
- [ ] Skill 完整目录已放到 `.agents/skills`。
- [ ] 已运行配置脚本。
- [ ] 已使用本人账号完成 OAuth。
- [ ] `doctor` 显示个人令牌和 refresh token 正常。
- [ ] `meetings --days 30` 能返回会议列表。
- [ ] 至少一篇智能纪要可以通过 `doc` 读取。
- [ ] 一条妙记可以导出带说话人与时间戳的完整转写稿。

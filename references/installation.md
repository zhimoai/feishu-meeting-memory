# 飞书会议知识提取 Skill 安装、更新与授权指引

本指引适用于个人授权版 `feishu-meeting-memory`：每位使用者把录音设备绑定到自己的飞书账号，Skill 通过该用户自己的 OAuth 身份查找和读取会议资料。同一飞书组织只需要一个应用，普通成员无需各自创建。

最终用户不需要安装 Python、Node.js、Go 或飞书 CLI。Skill 已包含 Windows、macOS 和 Linux 的独立程序。

## 一、先分清两种角色

### 飞书应用管理员：整个组织只准备一次

管理员负责创建应用、开通权限、配置回调地址、发布版本和设置可用范围。

### 普通使用者：每人操作一次

使用者安装 Skill 后，用自己的飞书账号完成浏览器授权。每个人获得独立的用户令牌，只能访问本人原本有权查看的会议资料。

个人账号不能脱离应用直接调用飞书 OpenAPI，因此需要一个已发布的飞书应用。组织外的独立部署者需要自己的应用；公共或大规模分发应使用 OAuth 网关保管 App Secret，避免将 Secret 分发到终端设备。

## 二、管理员准备飞书应用

普通组织成员跳过本节，直接从“三、安装 Skill”开始。管理员只需为整个组织配置一次。

### 1. 登录飞书开放平台

1. 打开[飞书开放平台开发者后台](https://open.feishu.cn/app)。
2. 使用准备读取会议资料的飞书账号登录。
3. 点击“创建企业自建应用”。如果页面没有这个按钮，需要联系所在飞书组织的管理员开通开发权限。
4. 填写应用名称，例如“飞书会议知识提取”，再填写描述、上传图标并确认创建。

完整妙记流程按企业自建应用设计。商店应用的接口支持范围不同，采用前必须逐项确认目标接口兼容性。官方说明：[企业自建应用开发流程](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process)。

### 2. 获取 App ID 和 App Secret

创建成功后，进入该应用，在左侧点击“凭证与基础信息”：

1. 找到 `App ID`，它通常以 `cli_` 开头，复制备用。
2. 找到 `App Secret`，点击显示后复制备用。
3. 将 App Secret 保存在密码管理器或密钥管理系统中，不得写入截图、群聊、Issue 或 GitHub。

App ID 用来识别应用；App Secret 相当于应用密码。两者稍后填入 Skill 根目录的 `config.json`。

### 3. 开通只读权限

在应用左侧进入“权限管理”，按照[管理员一次性配置](deployment-permissions.md)开通会议搜索、纪要、逐字稿、云文档读取和自动续期权限。也可以在 Skill 目录运行下面的命令查看准确清单：

```powershell
& .\scripts\feishu-meetings.ps1 permissions
```

本 Skill 查询个人会议时使用用户身份。权限只决定接口是否可以调用，不会自动赋予某篇会议或文档的访问权；实际读取范围仍等于登录用户在飞书界面中原本可以查看的范围。

### 4. 添加重定向 URL

1. 点击左侧“安全设置”。
2. 选择顶部“重定向 URL”页签。
3. 在输入框中粘贴下面的完整地址并点击添加：

```text
http://127.0.0.1:8080/callback
```

协议、IP、端口和路径必须完全一致。不要只填 `127.0.0.1`，不要改成 `localhost`，也不要省略 `/callback`。

### 5. 添加测试人员

打开左侧“测试企业和人员”，把需要授权的飞书账号加入测试人员。测试范围应遵循最小权限原则，验证访问边界后再逐步扩大。

### 6. 创建版本并发布

1. 点击左侧“版本管理与发布”。
2. 点击“创建版本”，填写版本号，例如 `1.0.0`，并填写更新说明。
3. 确认应用可用范围包含实际测试人员。
4. 提交并发布。如果组织启用了管理员审核，等待管理员批准。
5. 回到应用首页，确认顶部显示“当前修改均已发布”。

新增权限或修改回调地址后，都要重新创建并发布版本。已经授权过的用户还必须重新执行 `oauth-login --full`，否则旧令牌中没有新增权限。

### 7. 准备组织初始配置

管理员为成员准备只包含 App ID/Secret、尚未写入个人 token 的 `config.json`，再通过企业设备管理或其他安全渠道预置到 Skill 根目录。不要把已完成个人 OAuth 的配置发给任何人。

不得将真实凭据写入 README、示例文件、群聊、公共网盘、Skill 包或源码；Secret 一旦泄露，应立即在飞书开放平台重置。无法安全预置 Secret 的大规模场景应改用服务端 OAuth 网关。

官方参考：

- [飞书开放平台开发者后台](https://open.feishu.cn/app)
- [企业自建应用开发流程](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process)
- [访问凭证类型与获取方式](https://open.feishu.cn/document/server-docs/api-call-guide/calling-process/get-access-token)

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

- Windows：`%USERPROFILE%\.codex\skills\feishu-meeting-memory`
- macOS/Linux：`$HOME/.codex/skills/feishu-meeting-memory`

把完整 Skill 目录复制到上述位置。复制后应当直接看到以下内容：

```text
feishu-meeting-memory/
├── SKILL.md
├── config.example.json
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

## 四、更新 Skill

更新前必须保留 `config.json`。该文件包含 App Secret、user access token 和 refresh token，不在 Git 仓库中，也不会从 GitHub 重新下载。

### 推荐：让 Codex 完成更新

在 Codex 中发送以下内容：

```text
请更新已安装的 feishu-meeting-memory Skill：
https://github.com/zhimoai/feishu-meeting-memory

要求：
1. 找到当前实际生效的安装目录。
2. 将现有 config.json 安全备份到 Skill 目录之外，不要读取或输出其中内容。
3. 用 $skill-installer 安装 GitHub 上的最新版本；如果目标目录已存在，先把旧目录移动到 Skill 发现目录之外，不要直接覆盖。
4. 将 config.json 恢复到新版本根目录并运行 doctor。
5. 只有 doctor 验证成功后才能删除旧目录和临时备份；失败时恢复原版本。
```

`$skill-installer` 不会覆盖已经存在的同名目录，因此更新必须先保存配置并移开旧目录。旧目录不能留在 `.codex/skills` 或项目 `.agents/skills` 中，否则 Codex 可能同时发现两个同名 Skill。

### Git 克隆安装

如果安装目录中存在 `.git`，可以在该目录执行：

```bash
git pull --ff-only
```

`config.json` 已被 `.gitignore` 排除，正常拉取不会覆盖它。如果 `git pull --ff-only` 提示存在本地修改或分支分叉，应停止更新并先处理差异，不要使用强制重置覆盖本地文件。

### 更新后验证

Windows：

```powershell
& .\scripts\feishu-meetings.ps1 doctor
```

macOS/Linux：

```bash
sh ./scripts/feishu-meetings.sh doctor
```

确认顶层 `"ok": true`、`token_source` 为 `user_access_token`，并且 `refresh_token_saved` 为 `true`。一般更新不需要重新授权；如果新版增加或调整了 OAuth scope，则需要：

1. 在飞书开放平台开通新增权限。
2. 创建并发布新的应用版本。
3. 管理员或独立部署者将 `config.example.json` 中更新后的非敏感配置项合并到 `config.json`。
4. 重新运行 `oauth-login --full`。

## 五、生成本机配置

### 组织成员

组织成员不需要创建应用。管理员应通过企业设备管理或其他安全渠道提供一份尚未写入个人 token 的组织初始配置。

使用 `$skill-installer` 安装后，配置文件应放在：

| 安装方式 | `config.json` 路径 |
|---|---|
| Windows 用户级安装 | `%USERPROFILE%\.codex\skills\feishu-meeting-memory\config.json` |
| macOS/Linux 用户级安装 | `~/.codex/skills/feishu-meeting-memory/config.json` |
| 项目级安装 | `<项目目录>/.agents/skills/feishu-meeting-memory/config.json` |

Windows 用户可以把 `%USERPROFILE%\.codex\skills\feishu-meeting-memory` 直接粘贴到文件资源管理器地址栏。`config.json` 必须和 `SKILL.md` 位于同一层，不能命名为 `config.json.txt`。

组织初始配置内容如下，其中前两个值由管理员填写：

```json
{
  "app_id": "组织统一的飞书 App ID",
  "app_secret": "组织统一的飞书 App Secret",
  "oauth_redirect_uri": "http://127.0.0.1:8080/callback",
  "oauth_scope": "space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access",
  "api_base": "https://open.feishu.cn",
  "http_timeout_seconds": 30
}
```

确认文件已放好后直接进入“六、首次授权”，普通成员无需打开或修改它。

### 独立部署者

如果不属于已部署该应用的组织，在文件管理器中打开 Skill 目录，复制根目录的 `config.example.json`，把副本命名为 `config.json`。用记事本或其他文本编辑器打开 `config.json`，只替换自己应用的 `app_id` 和 `app_secret`，其余字段保持默认值。

也可以在终端中完成复制：

### Windows（可选）

```powershell
Copy-Item .\config.example.json .\config.json
notepad .\config.json
```

### macOS/Linux（可选）

```bash
cp ./config.example.json ./config.json
chmod 600 ./config.json
```

编辑时只需要先替换 `app_id` 和 `app_secret`。保持 `oauth_redirect_uri`、`oauth_scope` 和 `api_base` 的默认值。JSON 文件必须使用英文双引号，不能添加注释或多余逗号。

| 字段 | 填写方式 |
|---|---|
| `app_id` | 粘贴“凭证与基础信息”中的 App ID |
| `app_secret` | 粘贴同一页面中的 App Secret |
| `oauth_redirect_uri` | 保持 `http://127.0.0.1:8080/callback` |
| `oauth_scope` | 保持示例中的完整只读 scope |
| `api_base` | 中国版飞书保持 `https://open.feishu.cn` |
| `http_timeout_seconds` | 保持 30；网络较慢时再调整 |

不要手工添加 token。完成下一步 OAuth 后，程序会自动写入 user access token 和 refresh token，并在需要时自动刷新。仓库已经通过 `.gitignore` 排除 `config.json`，但它仍然是敏感文件。只可分发未授权、不含任何个人 token 的组织初始配置；不得复制其他用户已经授权的 `config.json`，也不要把已经配置过的 Skill 目录整体发给别人。

## 六、首次授权

推荐直接在 Codex 中提出第一次会议问题：

```text
$feishu-meeting-memory 最近开了哪些会？
```

Skill 检测到已有应用配置但尚无个人令牌时，会自动打开飞书授权页。使用者登录录音设备绑定的飞书账号并同意后，Skill 会继续执行刚才的会议请求。安装过程不能替用户静默同意授权，浏览器中的确认只需本人完成一次。

只有直接使用命令行或排障时，才需要手工运行下面的命令。

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

## 七、验证

查询成功返回会议列表就表示主流程已经跑通。需要检查授权状态时再运行：

```powershell
& .\scripts\feishu-meetings.ps1 doctor
```

应重点确认：

- `token_source` 为 `user_access_token`；
- `refresh_token_saved` 为 `true`；
- 未输出 App Secret、access token 或 refresh token。

需要直接测试命令行时可以查询最近会议：

```powershell
& .\scripts\feishu-meetings.ps1 meetings --days 30 --limit 10
```

程序会从当前用户个人云盘中识别“智能纪要、文字记录、我的笔记”，按会议合并后返回标题、时间和原始链接。

标题筛选、文档正文、妙记产物和逐字稿等进阶验收命令见[管理员一次性配置](deployment-permissions.md)与[接入和排障参考](setup.md)。普通使用者无需执行完整探测。

## 八、在 Codex 中使用

可以直接提问，也可以显式点名 Skill：

```text
$feishu-meeting-memory 今天开了哪些会？
$feishu-meeting-memory 最近关于新客户的会议有哪些？
$feishu-meeting-memory 总结“终端门店扫码发红包方案讨论”，列出决定、负责人和截止时间。
$feishu-meeting-memory 找出这场会议中关于报价的原话和时间戳。
```

Skill 默认只读取资料，不修改文档、不发送消息、不申请文档权限，也不下载原始音视频。

## 九、常见问题

| 现象或错误 | 原因与处理 |
|---|---|
| 授权页提示重定向 URL 有误，错误码 20029 | 后台重定向 URL 与本机命令不完全一致；确认是 `http://127.0.0.1:8080/callback`，重新发布。 |
| 授权页提示缺少 `offline_access`，错误码 20027 | 在权限管理开通“持续访问已授权的数据”，重新发布后再次 OAuth。 |
| API 返回 99991679 | 当前用户 OAuth 没有对应权限；后台开通并发布后，让该用户重新授权。 |
| API 返回 99991672 | 应用本身缺少接口权限；管理员补权限、发布版本并批准。 |
| 飞书界面能看到，API 读取不到 | scope 与单条文档 ACL 是两层权限；确认授权账号正确，并确认该账号对文档有访问权。 |
| 刚结束录音但列表中没有 | 妙记、智能纪要或文字记录仍在生成；稍后重试。不要把空结果直接解释为没有开会。 |
| 8080 端口被占用 | 关闭占用程序，或者在后台新增另一个完整回调地址，再用 `oauth-login --redirect-uri <地址>`。 |
| Windows 阻止运行程序 | 预编译程序未做商业代码签名；企业电脑应由内部构建/签名流程处理。不要从未知来源下载替代程序。 |
| 电脑没有 Python | 不受影响；最终用户只运行 Skill 自带的独立程序和系统 PowerShell/shell。 |

## 十、安全检查

- 每个人都使用自己的飞书账号 OAuth，不共享 token。
- 不提交或分享 Skill 根目录的 `config.json`。
- 更新或重新安装 Skill 前，只在可信位置备份自己的 `config.json`，完成后放回 Skill 根目录。
- 不在聊天、工单或截图中展示 App Secret 和 token。
- App Secret 泄露后立即在飞书开放平台重置。
- 用户离职、设备丢失或授权不再需要时，在飞书中撤销应用授权，并删除该设备上的本地配置。
- 公共或大规模分发应使用 OAuth 网关，使终端不再保存 App Secret。

## 十一、验收清单

### 管理员

- [ ] 已按 `permissions` 输出开通完整个人只读权限。
- [ ] 已开通 `offline_access`。
- [ ] 已添加精确的回调 URL。
- [ ] 已设置用户可用范围。
- [ ] 已发布新版本并批准权限。

### 使用者

- [ ] 录音设备已绑定到本人的飞书账号。
- [ ] Skill 完整目录已放到用户级 `.codex/skills` 或项目级 `.agents/skills`。
- [ ] Skill 根目录已有管理员预置的组织初始配置；独立部署者则已填写自己的应用信息。
- [ ] 首次会议提问时已使用本人账号完成浏览器 OAuth。
- [ ] `meetings --days 30` 能返回会议列表。
- [ ] 需要深度提取时，至少一篇本人可见的智能纪要可以读取。

# 接入与权限配置

面向首次使用者的逐步操作说明见 [installation.md](installation.md)。本文保留身份模型、权限边界和开发验收细节。

## 1. 授权模型：一个应用，每人登录自己的账号

个人账号不能脱离应用直接调用飞书 OpenAPI。当前本机试点使用一个已经创建并发布的飞书应用发起 OAuth；每位使用者在自己的电脑上登录自己的飞书账号并授权，程序保存该用户自己的短期 `user_access_token` 和 `refresh_token`。App ID 只标识应用，不能代替用户身份。

完整链路：

```text
应用配置（App ID/Secret）
        └─► 用户浏览器 OAuth
                └─► 个人 user_access_token + refresh_token
                        └─► 个人云盘会议列表与本人可见正文
```

不同用户不得复制或共用 token。scope 只授予接口能力，文档与妙记 ACL 仍以当前登录用户在飞书中的实际权限为准。

当前截图中的 App Secret 已经出现在聊天/截图中，试点跑通后应在飞书开放平台重置。新 Secret 只放在 Skill 根目录中被 Git 忽略的 `config.json`，或正式部署时放入服务端密钥管理器；不写进源码、Git、Issue 或聊天记录。

## 2. 安装者生成配置文件

真实配置固定放在当前 Skill 根目录：`<skill-dir>/config.json`。启动脚本会自动把这个路径传给独立程序，OAuth 获取的用户令牌也写回同一个文件。

Windows 本机试点在 Skill 目录运行：

```powershell
& .\scripts\configure.ps1
```

macOS/Linux 运行：

```bash
sh ./scripts/configure.sh
```

脚本会交互询问 App ID 和 App Secret，Secret 输入不回显；Windows 文件 ACL 只保留当前用户，macOS/Linux 文件权限为 `0600`。随后运行：

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

浏览器会打开飞书授权页。登录当前设备使用者自己的账号并同意授权；成功后回调到 `http://127.0.0.1:8080/callback`。该地址必须事先添加到应用后台“安全设置 → 重定向 URL”。OAuth 默认请求 `offline_access`，access token 临近过期时命令会自动刷新；只有刷新授权失效时才需要重新登录。

格式模板见 Skill 根目录的 [config.example.json](../config.example.json)。复制为 `config.json` 后再填写；不要直接在模板中填写真实密钥。

开发者仍可用 `FEISHU_CONFIG_FILE` 临时指向另一份 JSON，环境变量也继续优先于配置文件；普通使用者不需要设置这些覆盖项。

## 3. 本机试点与正式分发

### 当前本机试点

继续使用现有企业自建应用即可。配置中保存 App ID/Secret，每次 OAuth 登录的是实际使用录音豆的个人账号。这样能同时验证个人云盘、妙记搜索以及文档正文；不需要复制当前用户的 token 给其他电脑。

### 同一企业内的小范围试用

可以让应用管理员把试用人员加入应用可用范围，再让每个人各自运行 `oauth-login`。但 App Secret 不适合广泛分发，只能在受控公司电脑上短期试用。

### 正式多人分发

应增加一个 OAuth 网关，由服务端保管 App Secret 并完成授权码交换与令牌刷新；终端不保存 App Secret，也不拿别人的 token。若改为商店应用，仍需逐项确认目标接口支持商店应用。妙记部分接口标注仅支持企业自建应用，因此无企业应用时应以个人云盘 Docx 流程为主，不能承诺全部妙记产物 API 可用。

完全没有任何飞书应用时，不能使用官方 OpenAPI，只能返回链接让用户在已登录浏览器中查看，或使用易受页面变化影响的浏览器自动化。

## 4. 安装后无需 Python

技能包内已包含以下静态程序：Windows x64/ARM64、macOS Intel/Apple Silicon、Linux x64/ARM64。Windows 启动器会自动选择匹配的 `.exe`，macOS/Linux 启动器会选择匹配的 Mach-O/ELF 文件并在需要时补充执行权限。

当前本地测试阶段把完整目录放到用户级 `$HOME/.agents/skills/feishu-meeting-memory`，或项目级 `.agents/skills/feishu-meeting-memory`。具体步骤见 [installation.md](installation.md)。正式对外分发应在功能验收后打包为插件；本阶段不要使用旧的 `dist` 压缩包。

发布或复制技能时必须保留 `bin/`、`scripts/feishu-meetings.ps1` 和 `scripts/feishu-meetings.sh`。不要只复制 `SKILL.md`。可用 `bin/SHA256SUMS` 检查二进制完整性；源码位于 `cmd/feishu-meetings/`，重建脚本位于 `scripts/build-binaries.ps1`，只有开发者重建时才需要 Go。

预编译程序未做商业代码签名。若企业终端策略禁止运行未签名程序，应由企业自己的构建/签名流水线从源码重建并签名，而不是让最终用户安装 Python。

## 5. 飞书开放平台权限

完整会议发现、智能纪要、妙记 AI 产物和原始转写稿所需的必需/可选 scope，以 [deployment-permissions.md](deployment-permissions.md) 为唯一清单。部署时也可运行 `permissions` 读取当前程序内置的权限配置。

全部默认权限都是只读能力或 OAuth 自动续期能力。部分租户仍显示旧的聚合权限名，例如 `minutes:minutes:readonly` 或旧的文字记录下载权限；以当前飞书开发者后台和 API 报错给出的 scope 为准，不同时申请新旧同义权限。

开通权限后还需要：

1. 创建并发布应用版本。
2. 由管理员安装/启用该版本，并把应用可用范围限制在实际使用部门或人员。
3. 确保录音豆生成的妙记、智能纪要及导出的云文档对所选身份可见。接口 scope 和单条资源 ACL 是两道独立的门。
4. 用一个无敏感内容的测试录音验证完整链路，不要一开始就用真实客户会议。

## 6. 验证

```powershell
& .\scripts\feishu-meetings.ps1 permissions
& .\scripts\feishu-meetings.ps1 oauth-login --full
& .\scripts\feishu-meetings.ps1 doctor
& .\scripts\feishu-meetings.ps1 meetings --days 30
& .\scripts\feishu-meetings.ps1 meetings --query '客户' --days 30
& .\scripts\feishu-meetings.ps1 doctor --probe
& .\scripts\feishu-meetings.ps1 search --days 7 --limit 5
& .\scripts\feishu-meetings.ps1 search --days 7 --mine --limit 5
& .\scripts\feishu-meetings.ps1 doc-search --days 7 --limit 5
& .\scripts\feishu-meetings.ps1 show <minute_token> --artifacts summary,todos
& .\scripts\feishu-meetings.ps1 show <minute_token> --artifacts transcript
& .\scripts\feishu-meetings.ps1 doctor --probe --minute <minute_token> --transcript --doc <docx_or_wiki_url>
```

普通 `doctor` 只验证配置和取 token。`doctor --probe` 以最小返回量探测妙记/云文档搜索 scope；加入 `--minute`、`--doc` 或 `--transcript` 时会读取对应测试资源但立即丢弃内容，只报告每条能力是否成功。使用一条无敏感内容的测试资料完成验收，单个成功结果不能证明其他资源 ACL 也相同。

## 7. 常见故障

| 现象 | 判断与处理 |
|---|---|
| 认证失败 | 检查 App ID、新 Secret、应用版本是否已发布；不要复用截图里的 Secret。 |
| 缺少 scope | 在权限管理中增加报错明确指出的只读 scope，重新发布并管理员批准。 |
| 有 scope 但某条妙记 2091005/403 | 这是资源 ACL，不是接口权限；让所有者以正常飞书分享流程授权。 |
| 只有 App ID/Secret，仍不能查询最近会议 | 应用凭据不是个人身份；运行 `oauth-login`，由当前使用者登录自己的飞书账号。 |
| access token 过期 | 保存了 refresh token 时会在调用前自动刷新；没有 refresh token 或刷新授权已过期时重新运行 `oauth-login`。 |
| `search` 要求用户身份 | 这是接口硬性要求，不是关键词或分页问题。运行个人 OAuth；只有 App ID/Secret 时不能列出个人妙记。 |
| `user-search` 提示需要用户身份 | 联系人搜索不能使用 App Secret/tenant token；改用 OAuth 用户令牌或直接提供已知 open_id。 |
| 刚录完查不到 | 妙记仍在上传/转写/生成产物；稍后重试。 |
| 飞书云盘中可见但 `doc-search` 为零 | 用户个人根目录不等于应用空间；使用用户 OAuth 后执行 `drive-meetings`，不要继续改搜索词。 |
| `doctor --probe` 部分成功 | 根据 `probes` 中失败项补对应 scope；搜索成功但详情失败通常是详情 scope 或单条资源 ACL。 |
| `note-transcript` 失败 | unified Note 逐字稿通常需要用户身份；普通 Note 应读取 `verbatim_doc_token` 对应的 Doc。 |
| 文档正文无权 | Note 权限不自动等于 Doc 权限；对目标文档单独授权。 |

## 8. 域名

中国版飞书默认使用 `https://open.feishu.cn`。国际版 Lark 或企业网关可通过 `FEISHU_API_BASE` 覆盖基础地址。只允许 HTTPS；测试环境允许显式传入本机 HTTP 地址。

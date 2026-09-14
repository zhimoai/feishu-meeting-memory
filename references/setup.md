# 接入与权限配置

面向首次使用者的逐步操作说明见 [installation.md](installation.md)。本文保留身份模型、权限边界和开发验收细节。

## 1. 授权模型：一个应用，每人登录自己的账号

个人账号不能脱离应用直接调用飞书 OpenAPI。系统使用已创建并发布的飞书应用发起 OAuth；每位使用者在自己的设备上登录飞书账号并授权，程序保存该用户自己的短期 `user_access_token` 和 `refresh_token`。App ID 只标识应用，不能代替用户身份。

完整链路：

```text
应用配置（App ID/Secret）
        └─► 用户浏览器 OAuth
                └─► 个人 user_access_token + refresh_token
                        └─► 个人云盘会议列表与本人可见正文
```

不同用户不得复制或共用 token。scope 只授予接口能力，文档与妙记 ACL 仍以当前登录用户在飞书中的实际权限为准。

App Secret 只应保存在 Skill 根目录中被 Git 忽略的 `config.json`，或多人部署时保存在服务端密钥管理器中；不得写入源码、Git、Issue、日志或聊天记录。已经泄露的 Secret 必须在飞书开放平台重置。

## 2. 配置文件

真实配置固定放在当前 Skill 根目录：`<skill-dir>/config.json`。启动脚本会自动把这个路径传给独立程序，OAuth 获取的用户令牌也写回同一个文件。

同一组织内，管理员可以安全预置只含 App ID/Secret、没有任何个人 token 的初始配置，普通成员无需编辑。组织外独立部署者才需要复制示例并填写自己的应用信息：

Windows：

```powershell
Copy-Item .\config.example.json .\config.json
notepad .\config.json
```

macOS/Linux 运行：

```bash
cp ./config.example.json ./config.json
chmod 600 ./config.json
```

在 `config.json` 中只替换 App ID 和 App Secret，其余字段保持模板默认值。Windows 用户应确保该文件只对自己的账号可读；macOS/Linux 示例已经将权限设为 `0600`。

在 Codex 中首次提出会议问题时，Skill 会在缺少个人令牌的情况下自动发起 OAuth 并在授权成功后继续原请求。直接使用命令行或排障时才需要手工运行：

```powershell
& .\scripts\feishu-meetings.ps1 oauth-login --full
```

浏览器会打开飞书授权页。登录当前设备使用者自己的账号并同意授权；成功后回调到 `http://127.0.0.1:8080/callback`。该地址必须事先添加到应用后台“安全设置 → 重定向 URL”。OAuth 默认请求 `offline_access`，access token 临近过期时命令会自动刷新；只有刷新授权失效时才需要重新登录。

格式模板见 Skill 根目录的 [config.example.json](../config.example.json)。不要直接在模板中填写真实密钥。

高级配置可通过 `FEISHU_CONFIG_FILE` 指向另一份 JSON；环境变量的优先级高于配置文件。普通使用者无需设置这些覆盖项。

## 3. 部署模式

### 个人本地使用

适合个人或单台受控设备。App ID/Secret 和该用户的 OAuth 令牌保存在本地 `config.json`；个人云盘、妙记和文档访问均使用该用户身份。

### 企业内部部署

同一组织只需要一个企业自建应用。应用管理员将使用者加入应用可用范围并安全预置初始配置；每位使用者首次会议查询时分别用自己的账号完成 OAuth。如需在终端保存 App Secret，应将部署范围限制在受控设备，并配合操作系统权限、磁盘加密和凭据轮换策略。

### 公共或大规模分发

应使用 OAuth 网关，由服务端保管 App Secret 并完成授权码交换与令牌刷新，使终端不保存 App Secret。商店应用需要逐项确认目标接口兼容性；部分妙记接口仅支持企业自建应用，因此无法使用企业应用时应以个人云盘 Docx 流程为主。

完全没有任何飞书应用时，不能使用官方 OpenAPI，只能返回链接让用户在已登录浏览器中查看，或使用易受页面变化影响的浏览器自动化。

## 4. 安装后无需 Python

技能包内已包含以下静态程序：Windows x64/ARM64、macOS Intel/Apple Silicon、Linux x64/ARM64。Windows 启动器会自动选择匹配的 `.exe`，macOS/Linux 启动器会选择匹配的 Mach-O/ELF 文件并在需要时补充执行权限。

完整目录可安装到用户级 `$HOME/.codex/skills/feishu-meeting-memory`，或项目级 `.agents/skills/feishu-meeting-memory`。安装和更新步骤见 [installation.md](installation.md)。

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

普通使用者以一次真实的近期会议查询作为主流程验证即可。下面的逐项命令仅供管理员部署验收和故障排查：

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
| 认证失败 | 检查 App ID、App Secret 和应用版本发布状态；若 Secret 曾经泄露，应先重置再重新配置。 |
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

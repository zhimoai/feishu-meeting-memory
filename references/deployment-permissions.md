# 飞书管理员一次性配置

本页只面向飞书应用管理员。普通组织成员无需创建应用、配置权限或执行部署验收；应用可用后，他们只需安装 Skill，并在首次会议查询时完成个人 OAuth。

## 管理员需要做什么

1. 在[飞书开放平台](https://open.feishu.cn/app)创建企业自建应用。
2. 在“权限管理”开通下方完整只读权限。
3. 在“安全设置 → 重定向 URL”添加 `http://127.0.0.1:8080/callback`。
4. 发布应用，并将实际使用者加入应用可用范围。
5. 通过企业设备管理或其他安全渠道预置仅含 App ID/Secret 的 `config.json`；不得把 App Secret 提交到公共仓库。

同一组织只需一个应用。每位成员仍需使用自己的飞书账号完成 OAuth，得到独立的 user access token；应用可用范围不会绕过文档、妙记、企业密级或 DLP 权限。

## 完整只读权限

可在 Skill 目录运行 `feishu-meetings permissions` 查看程序内置清单。默认完整权限为：

```text
space:document:retrieve docx:document:readonly search:docs:read minutes:minutes.search:read minutes:minutes.basic:read minutes:minutes.artifacts:read minutes:minutes.transcript:export vc:note:read wiki:node:retrieve offline_access
```

| 能力 | 权限 |
|---|---|
| 个人授权自动续期 | `offline_access` |
| 枚举个人云盘会议文档 | `space:document:retrieve` |
| 读取智能纪要和文字记录 | `docx:document:readonly` |
| 搜索云文档和知识库 | `search:docs:read` |
| 搜索和读取妙记 | `minutes:minutes.search:read`、`minutes:minutes.basic:read` |
| 读取总结、待办、章节和关键词 | `minutes:minutes.artifacts:read` |
| 导出带说话人和时间戳的逐字稿 | `minutes:minutes.transcript:export` |
| 解析智能纪要和 Wiki 关系 | `vc:note:read`、`wiki:node:retrieve` |

按姓名查找人员时可额外开通 `contact:user:search`。只有需要处理飞书视频会议 `meeting_id` 时，才增加 `vc:meeting.meetingevent:read` 和 `vc:record:readonly`。

这些权限不包含修改文档、下载原始音视频、发送消息、创建任务或管理协作者。

## 用户首次授权

应用配置已经预置时，成员可以直接在 Codex 中提问：

```text
$feishu-meeting-memory 最近开了哪些会？
```

Skill 会在缺少个人令牌时自动启动 `oauth-login --full`。浏览器授权页仍需用户本人登录并确认，授权成功后继续原请求。该确认不能被安装程序静默跳过。

## 管理员验证

使用一个无敏感内容的测试账号，在 Codex 中提问：

```text
$feishu-meeting-memory 最近开了哪些会？
```

能够完成浏览器授权并返回会议列表，就表示基础流程已通。只有失败时才运行 `doctor --probe`；具体排查命令见[接入和排障参考](setup.md)，普通成员无需执行。

新增权限后需要重新发布应用，并让已经授权的成员再次完成 OAuth。

## 部署边界

- 企业自建应用仅服务其所属组织及应用可用范围内的成员；组织外用户不能直接复用。
- 当前桌面模式需要在本机换取和刷新令牌，因此本地配置包含 App Secret。面向大量终端时应使用服务端 OAuth 网关保管 Secret。
- 公共 GitHub 仓库只能发布代码和配置示例，不能包含真实 App Secret、user access token 或 refresh token。
- 部分妙记接口仅支持企业自建应用；若改用商店应用，应逐接口验证兼容性。无法使用妙记接口时，仍可通过个人云盘 Docx 发现并读取智能纪要和文字记录。

官方参考：[企业自建应用开发流程](https://open.feishu.cn/document/home/introduction-to-custom-app-development/self-built-application-development-process)、[获取访问凭证](https://open.feishu.cn/document/server-docs/api-call-guide/calling-process/get-access-token)。

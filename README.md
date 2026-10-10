# Isekai MCP Gateway

Streamable HTTP MCP、上游 OIDC 登录、Gateway OAuth，以及页面读取、编辑会话、页面变更和上传工具。
架构规定的 27 个业务工具及 Resources、迁移与验证说明见 [Wiki 工具部署说明](docs/wiki-tools.md)。

OAuth 注册复用、指纹算法、存储拆分及迁移注意事项见 [OAuth 存储说明](docs/oauth-storage.md)。

## 依赖与启动

- Go 1.25.6，纯 Go 构建，依赖锁定于 `go.mod` / `go.sum`。
- 独立 MySQL 8.0+ 数据库，InnoDB、utf8mb4。Gateway 用户需要迁移时的 DDL 权限和运行时的读写权限。
- 上游 OIDC 服务 confidential client，启用 Standard Flow、S256 PKCE，关闭 implicit、password 和 service accounts。
- 已配置 IsekaiServiceBridge 和现有 Wiki OIDC 绑定；Bridge key 开启 `allowBootstrapIdentity`。

以下命令在项目根目录执行。初始配置文件已生成；再次执行 `init-dev` 会拒绝覆盖。

使用 PowerShell 脚本按 `系统-arch` 编译，产物位于 `bin/<系统-arch>/gateway`（Windows 为 `gateway.exe`）：

```powershell
./make.ps1 windows-amd64
./make.ps1 linux-arm64
```

脚本使用 `CGO_ENABLED=0`，目标组合由当前 Go 工具链校验，也可从其他目录通过脚本路径调用。

```powershell
$env:CGO_ENABLED = '0'
go build -o bin/gateway.exe ./cmd/gateway
# 仅首次创建配置时执行：
./bin/gateway.exe init-dev
```

填写 `config/server.json`、`config/app.json` 和 `config/development.env.ps1`，然后：

```powershell
. ./config/development.env.ps1
./bin/gateway.exe check-config
./bin/gateway.exe migrate
./bin/gateway.exe serve
```

无 action 时显示帮助。CLI flags 可放在 action 前后，例如 `gateway --config-dir D:\gateway-config migrate`。

模板和帮助正文放在项目根目录的 `templates/`，静态资源放在独立的 `assets/`。Go 源码目录与模块职责见 [项目结构](docs/project-structure.md)。
`--data-dir` 指定这些目录的父目录，默认进程工作目录。例如从其他目录启动：

```powershell
D:\gateway\bin\gateway.exe --config-dir D:\gateway\config --data-dir D:\gateway serve
```

构建时通过 `go:embed` 内嵌模板、帮助正文和静态资源。`templates/` 与 `assets/` 分别优先使用外部目录；
对应目录不存在时使用内置内容。外部目录存在时必须提供完整文件集；缺失、不可读或模板解析错误会导致启动及 `check-config` 失败。
`serve` 默认监听模板目录，文件变化稳定 200ms 后重新解析并原子发布；失败时记录错误并保留上一份有效模板。
新增子目录、编辑器原子保存和模板目录的删除、重新创建均触发加载；删除目录后使用内置模板。
Windows 使用父目录的系统递归通知，避免子目录监听句柄阻止整体目录替换；其他平台通过 fsnotify 注册子目录。
通过 `serve --watch-templates=false` 关闭监听。静态资源由独立路由模块按请求读取，外部资源编辑后下一次请求生效；资源来源在启动时选择。
静态资源 URL 为 `/assets/auth/auth.js` 和 `/assets/auth/style.css`；帮助 Resource URI 保持原定义。
迁移 SQL 嵌入二进制，由独立迁移命令执行。
迁移操作 Gateway 独立数据库的 OAuth、编辑会话、提交记录、确认、上传及站点连接表，启动不会自动迁移。
数据库加密密钥、cookie 密钥与签名 JWKS 必须持久保存；替换密钥需重启，并会影响现有凭据。
本地开发配置支持 loopback HTTP；公网部署使用 HTTPS 反向代理。
Wiki API 允许 HTTP 或 HTTPS；使用 HTTP 时仅在服务启动时警告。

也可在 `server.json` 的 `server.tls` 设置 `enabled: true`、`certFile` 和 `keyFile`，直接提供 HTTPS。
证书路径相对于配置目录，证书须为 PEM 格式；TLS 配置修改后需要重启。
开发自签证书及客户端信任配置见 [开发 HTTPS](docs/development-https.md)。

## 开发环境配置

`server.json`：填写 `storage.host/port/database/username`，密码支持环境变量或 `storage.password` 直接值。
设置 Gateway 的公开地址后，同步更新 `server.publicUrl`、`auth.gateway.issuer`、`auth.gateway.resource`
和 `server.allowedHosts`。Host 白名单按收到的 Host 精确匹配，非默认端口需包含端口。

`app.json`：填写 上游 OIDC 服务 `issuer/clientId/redirectUri`、Wiki `apiUrl/siteId` 和 Bridge `keyId`。
默认回调为 `http://127.0.0.1:3000/auth/oidc_client/callback`，在上游 OIDC 服务中登记该精确地址，使用 query 回调模式。
Wiki 使用已有的 上游 OIDC 服务 sub 绑定；pairwise subject 配置需要确认两个应用的 sub 一致。

`development.env.ps1`：填写 MySQL 密码、上游 OIDC 服务 client secret 和现有 Bridge secret。
Bridge secret 按原始字符串使用，至少 32 字节，不能自行改变 hex/base64 的解释。
生成的加密与签名材料只供该开发环境使用，配置和密钥文件已加入 `.gitignore`。
Windows 文件权限由目录 ACL 决定；部署时限制服务配置目录的访问用户。

全部密钥支持直接填写的镜像字段；直接值和 Env 字段不能同时非空。填写直接值时，必须把对应 Env 字段设为 `""`，包括存在默认 Env 名称的字段。字段清单、JWKS 对象格式和填写示例见 [直接填写密钥](docs/inline-secrets.md)。

## MCP 与 OAuth

客户端连接 `http://127.0.0.1:3000/mcp`，传输类型为 Streamable HTTP。
未认证时返回 401 和 `WWW-Authenticate` 中的 resource metadata 地址。
完整流程为：发现 → 预注册/CIMD/DCR → S256 授权 → 上游 OIDC 服务 → Wiki 绑定 → 客户端风险确认 → consent → 换码 → MCP。

- 授权、换码和 refresh 请求须提供与配置完全一致的 `resource`。
- 默认 scope 为 `mcp:access`。DCR 的 `scope` 表示客户端允许申请的范围，用户另行确认。
- 提交同意时，后端自动纳入本次授权请求的基础 scope（openid、offline_access、profile、email）；自定义权限按表单勾选授予。
- 需要刷新时，客户端 metadata 包含 `refresh_token` grant，申请并同意 `offline_access`。
- refresh 不携带 `scope`；每次返回新 refresh token，旧令牌重用撤销整个 Grant 家族。
- Gateway access token 为 provider 加密生成的不透明 JWE，只作为 bearer credential 使用。
- OAuth token 响应去除 provider 内部生成的 `id_token`；MCP 拒绝 上游 OIDC 服务 token 和 ID token。
- CIMD 缓存 5 分钟，抓取限公网 HTTPS、443 端口，校验每次 DNS 与重定向目标。
- DCR 首次登记 24 小时有效，用户首次信任需等待服务端 5 秒倒计时。
- `/auth/clients` 管理个人信任，`/auth/grants` 管理授权；两者的状态变更均为 POST + CSRF。
- introspection 仅允许管理员预注册且 `resourceClient=true` 的 confidential client。
- 退出 上游 OIDC 服务 不触发 Gateway 授权撤销，本版本未接入 上游 OIDC 服务 logout 通知。

预注册客户端字段采用 OAuth metadata 命名，secret 使用环境变量引用：

```json
{
  "client_id": "development-agent",
  "client_name": "Development Agent",
  "redirect_uris": ["http://127.0.0.1:9876/callback"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "scope": "mcp:access offline_access"
}
```

## Help

`wiki.help` 接受 `{ "topic": "index" }`，topic 可省略；可用主题通过 index 查看。
`wiki://help/index` 和 `wiki://help/{topic}` 返回同源 Markdown。

## 热重载与健康检查

`app.json` 内容稳定 500ms 后触发验证、上游 OIDC 服务 discovery 和 Bridge 探测，全部成功才发布新快照。
无效配置保留旧快照；同一登录事务使用原配置完成，重启也可从加密事务恢复。
`siteId` 改动要求重启，避免已有授权的 Wiki 身份跨站点混用。
`server.json`、Gateway 授权密钥、存储与插件设置要求重启。

- `/health/live`：listener 存活。
- `/health/ready`：初始上游配置已探测成功且数据库可用；不代表每个上游此刻均可访问。
- `gateway write-schema`：从 Go 配置类型生成两个 JSON Schema。

反向代理需保留公开 Host，关闭 `/mcp` 响应缓冲并支持 SSE，代理访问日志也需去除 OAuth 敏感查询参数。
默认 HTTP 日志仅记录路由、状态和耗时。`logging.traceRequest` 默认为 `false`，支持热更新；设为 `true` 并将 `logging.level` 设为 `debug` 或 `trace` 时，以 DEBUG 输出 HTTP 请求 URL、完整请求头、下游读取的请求体、响应状态、完整响应头及响应体，不进行脱敏。请求与响应通过 `request_id` 关联；流式响应持续透传，在处理结束后输出响应体。provider 的原始错误文本仅在 `logging.level` 为 `debug`/`trace` 时记录。

`app.json` 的 `logging.consoleFormat` 默认为 `text`，也可设为 `json`。TRACE、DEBUG、INFO 输出到 stdout，WARN 和 ERROR 输出到 stderr；业务错误仍按错误分类和日志级别过滤。

`logging.file.enabled` 默认为 `false`。启用后同时追加写入 JSON Lines 文件：`split: true` 将普通日志写入 `path`（默认 `./logs/log.jsonl`），将 WARN 和 ERROR 写入 `errorPath`（默认 `./logs/error.jsonl`）；`split: false` 将所有日志写入 `path`。相对路径基于进程工作目录，缺失的目录会自动创建。文件打开失败会阻止启动，运行时写入失败会报告到控制台。日志级别支持热更新，格式、文件路径及拆分方式变更需要重启。当前文件采用追加写入，需由部署环境安排轮转和清理。

## 验证状态

初次编写阶段结束后，等待开发配置填写，再进行运行测试。
验收矩阵见 [基础版本验收](docs/base-version-validation.md)。真实 MySQL、上游 OIDC 服务 与第三方客户端互通结论
必须以实际运行结果更新，不根据构建成功宣称通过。

## CLI actions 与密钥生成

支持 `serve`、`migrate`、`init-dev`、`check-config`、`write-schema`、`generate-keys`、`export-templates`、`export-assets`。使用 `gateway --help` 或 `gateway serve --help` 查看帮助。
`--config-dir` 默认 `config`，`--data-dir` 默认 `.`，也可使用 `gateway serve --config-dir D:\gateway-config --data-dir D:\gateway`。
旧 `--check-config`、`--write-schema` 开关继续映射到对应 action；不能与显式 action 或彼此混用。

```powershell
./bin/gateway.exe generate-keys
```

此命令仅向 stdout 输出带缩进的 JSON，不需要配置或外部服务。输出字段映射如下：

| 输出字段 | 配置字段 |
| --- | --- |
| cookieSecret | server.json 的 auth.gateway.cookieSecret |
| storageEncryptionKey | server.json 的 auth.gateway.storageEncryptionKey |
| signingJwks | server.json 的 auth.gateway.signingJwks（完整对象） |
| serviceBridgeSecret | app.json 的 mediawiki.serviceBridge.secret |

填写直接值时可省略对应的 `cookieSecretEnv`、`storageEncryptionKeyEnv`、`signingJwksEnv` 和 `secretEnv`；直接值与 Env 字段都省略时回退到默认 Env 名称。
ServiceBridge 共享密钥需要在网关与 MediaWiki 两端填写相同的字符串原文；不要先 Base64 解码。
数据库密码与上游 OIDC client secret 由相应系统提供。生成的密钥需持久保存；每次调用都会生成新密钥。

## 自定义模板和资源

两个导出命令无需配置、数据库或上游服务；默认跳过已有文件并报告数量，添加 `--overwrite` 覆盖已有普通文件。目标路径中的符号链接或文件与目录冲突会报错。导出期间请勿同时修改目标目录。

```powershell
./bin/gateway.exe --data-dir ./data export-templates
./bin/gateway.exe --data-dir ./data export-assets
./bin/gateway.exe --data-dir ./data export-templates --overwrite
./bin/gateway.exe --data-dir ./data serve
```

`templates/html/layouts/` 和 `templates/html/partials/` 放公共 HTML 定义；`templates/html/pages/` 放页面，可使用子目录。
每个页面独立解析公共文件，再解析页面文件；以相对于 pages 的路径（去掉 `.html`）调用 `RenderHTML`。
公共 layout 使用 Go 模板 `define`、`template`、`block`，页面通过 `template` 选择 layout，通过同名 `define` 覆盖 block。多个页面可以各自定义 `content`；支持多种和嵌套 layout。

```gotemplate
{{/* html/layouts/example.html */}}
{{ define "layout/example" }}<main>{{ block "content" . }}{{ end }}</main>{{ end }}

{{/* html/pages/example.html */}}
{{ template "layout/example" . }}
{{ define "content" }}<h1>{{ .Title }}</h1>{{ end }}
```

帮助文件位于 `templates/text/help/*.md`。自定义提示词可放在 `templates/text/prompts/*.tmpl`，公共片段放在 `templates/text/partials/`；这些目录可以按需创建。
文本文件递归加载 `.md` 和 `.tmpl`，通过 `RenderText` 使用相对于 text 的完整路径（保留扩展名）或显式定义名称。
文件以相对路径命名，可使用 `{{ template "partials/rules.tmpl" . }}` 引用另一个文件。
HTML 变量自动转义；文本变量保持原文；缺失的 map 字段返回错误。所有渲染成功后才输出内容。
每个 HTML 页面解析时公共文件按路径排序后加载；文本文件按路径排序加载。重载失败不会替换有效快照。

静态资源模块提供 `/assets/*`，不暴露目录列表与 Go 源文件；模板套件与资源模块分别加载和导出。

页面续页缓存由 `app.json` 的 `mediawiki.pageCacheTtlSeconds` 控制，默认 `600` 秒，必须为正整数。缓存覆盖页面源码、HTML、章节和依赖解析结果，按站点、用户凭据、授权与配置隔离。读取第一页始终查询上游并刷新对应缓存；提交编辑请求后清理该页面所有用户的缓存，后续读取重新获取。续页命中时仍查询页面权限及正式修订可见性；缓存过期或进程重启后按游标固定的修订重新获取，并校验内容或解析哈希。缓存使用进程内存，最多保留 256 项、64 MiB；命中不延长有效期。

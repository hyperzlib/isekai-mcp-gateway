# {{.SiteTitle}} MCP 接入信息

> {{.SiteTitle}} 提供的 MCP 服务，支持通过 AI 助手访问和操作 Wiki。

## 基本信息
- MCP Server 地址：{{.Endpoint}}
- 传输方式：Streamable HTTP

## 配置方式
请查询你现在所处的Agent环境或Harness的说明，搜索其中关于MCP的配置，然后在配置文件中添加此MCP服务器。

## 用户登录方式
如果当前Agent支持OAuth登录，则无需额外配置。如果不支持OAuth，需要提示用户在 [密钥管理]({{.SiteUrl}}/auth/apikeys) 中添加 API 密钥，在用户提供 API 密钥后，在MCP配置中添加 `X-API-Key` Header，或添加 `Authorization: Bearer sk-xxx`
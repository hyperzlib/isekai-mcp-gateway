此方式适用于不支持 OAuth 浏览器授权的客户端，改用你自己创建的 API Key 访问。

1. 登录后打开「API Keys」页面，创建一个密钥并立即复制（明文只显示一次）。
2. 在客户端的 MCP 配置里，把下方地址作为服务器 URL，认证方式选择自定义 Header。
3. Header 名称填 `X-API-Key`，值填完整的 `sk-` 开头密钥（部分客户端也可将其作为 Bearer 令牌填入 Authorization Header）。
4. 保存后刷新工具列表即可使用；密钥可在「API Keys」页面随时删除。

```text title="服务器 URL"
{{mcp_url}}
```

```text title="请求头示例"
X-API-Key: sk-xxxxxxxxxxxxxxxxxxxxxxxx
```

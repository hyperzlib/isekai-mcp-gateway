# MCP 连接

使用管理员提供的 `/mcp` URL，以 Streamable HTTP 连接。
未登录时服务器返回 401 和 OAuth 资源元数据位置。
支持 OAuth 的客户端将打开浏览器，完成 上游 OIDC 服务 登录、客户端确认和权限选择。
连接和调用需要 `mcp:access`。客户端应在每个 HTTP 请求发送 Gateway bearer token。

连接后调用 `wiki.help` 阅读帮助并验证连接。
重新启动 Gateway 后已有 OAuth 授权仍在有效期内；客户端可重新建立 MCP 连接。

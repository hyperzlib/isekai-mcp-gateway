1. 在 Cursor 设置中打开 MCP 页面，点击添加远程 MCP 服务器。
2. 填入下方的 URL，保存并开启开关，按提示在浏览器中完成授权。
3. 在 Agent 对话中就能使用 Wiki 工具了。

::: details 偏好手动编辑配置文件？

```json title="~/.cursor/mcp.json"
{
  "mcpServers": {
    "isekai": {
      "url": {{mcp_url_json}}
    }
  }
}
```

把以上配置合并进mcp.json并重启 Cursor 即可。

:::

[Cursor 官方文档 ↗](https://cursor.com/docs/mcp)

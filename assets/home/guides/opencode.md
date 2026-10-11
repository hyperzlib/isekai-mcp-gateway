1. 把下方的配置合并到 opencode.json 中。
2. 运行授权命令，在浏览器中登录本站并确认权限。
3. 回到 OpenCode，把 Wiki 相关任务交给它即可。

```json title="opencode.json"
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "isekai": {
      "enabled": true,
      "type": "remote",
      "url": {{mcp_url_json}}
    }
  }
}
```

```sh title="授权与检查"
opencode mcp auth isekai
opencode mcp list
```

::: details 用的是 OpenCode 新版（V2 配置格式）？

```json title="opencode.jsonc"
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "servers": {
      "isekai": {
        "type": "remote",
        "url": {{mcp_url_json}}
      }
    }
  }
}
```

添加后如需登录，在 OpenCode 中输入 /mcps，选择本服务器并登录。

:::

[OpenCode 官方文档 ↗](https://opencode.ai/docs/mcp-servers/)

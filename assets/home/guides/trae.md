1. 登录后打开「API Keys」页面，创建一个密钥并立即复制（明文只显示一次）。
2. 在 TRAE 左下角点击头像，进入 设置 > MCP。
3. 在 MCP Servers 管理 区域点击右上角的 创建 > 手动配置，粘贴下方配置，并把 `sk-xxxx` 换成刚复制的密钥。
4. 点击 确认 保存，服务器状态正常即连接成功。
5. 打开智能体设置，把这个 MCP Server 勾选给要用的智能体；之后在对话中让它读取 Wiki 即可，例如「用 wiki.me 看看我是谁」。

```json title="手动配置"
{
  "mcpServers": {
    "isekai": {
      "url": {{mcp_url_json}},
      "headers": {
        "X-API-Key": "sk-xxxxxxxxxxxxxxxxxxxxxxxx",
        "RUN_MCP_TIMEOUT_MS": "120000"
      }
    }
  }
}
```

::: details 工具没出现或调用失败？

- 配置保存成功不等于可用：TRAE 需要把该 MCP Server 加入智能体后，工具才会出现在对话中。
- `url` 用完整地址并保留末尾的 `/mcp`；`headers` 里的键和值都要加引号，不要使用中文引号。
- 鉴权失败通常是密钥错误或已删除；重新创建后替换 `X-API-Key` 的值。
- 单次调用超时可以在 `headers` 里调大 `RUN_MCP_TIMEOUT_MS`（毫秒），启动阶段的超时用 `START_MCP_TIMEOUT_MS`。
- 粘贴前确认 JSON 合法：多一个逗号或漏一个括号都会让整份配置被拒绝。

:::

::: details 想跟着项目走，而不是每次都手动加？

TRAE 还支持项目级配置：在项目根目录的 `.trae/mcp.json` 里写同样的 `mcpServers` 结构，首次使用时到 设置 > MCP 打开项目级 MCP 开关即可。字段名是 `mcpServers`，与 VS Code 的 `servers` 不同，不要直接照搬。

:::

::: details 为什么需要 API Key

TRAE 的 MCP 暂时不支持通过 OAuth ，在浏览器中直接登录。

:::

[TRAE 官方文档 ↗](https://docs.trae.ai/ide/model-context-protocol)

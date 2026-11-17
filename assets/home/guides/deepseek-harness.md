1. 登录后打开「API Keys」页面，创建一个密钥并复制。
2. 打开 `cordis.patch.yml`，把下方条目追加到文件末尾，并把 `sk-xxxx` 换成刚复制的密钥。
3. 保存后立即生效；此时一般就能在 DeepSeek Harness 中操作网站上的内容了。如果没有生效，重启 DeepSeek Harness 试试。
4. 在会话中让它读取 Wiki，例如「用 wiki.me 看看我是谁」，能返回用户名即接入成功。

```yaml title="cordis.patch.yml"
- insert:
    - id: mcp-isekai
      name: "@deepseek-ai/dsh-mcp-client"
      config:
        serverName: isekai
        transport: streamable-http
        url: "{{mcp_url}}"
        headers:
          X-API-Key: sk-xxxxxxxxxxxxxxxxxxxxxxxx
        toolCallTimeoutMs: 120000
```

配置文件位置：

```text title="配置路径"
Windows          %USERPROFILE%\.dsh\profiles\desktop\cordis.patch.yml
macOS / Linux    ~/.dsh/profiles/desktop/cordis.patch.yml
dsh web 或 headless   ~/.dsh/profiles/web/cordis.patch.yml
```

::: details 工具没出现或调用失败？

- 文件顶层必须是数组：与已有的 `- id:`、`- insert:` 项同级，新内容同样以 `- insert:` 开头，不要缩进进别的条目。
- `serverName` 只能是 `[A-Za-z0-9_-]{1,32}`，它决定工具前缀：`isekai` → `mcp__isekai__wiki_search`。
- `url` 用完整地址并在末尾保留 `/mcp`；401 表示密钥无效或已被删除，重新创建后替换。
- 没报错也没工具：确认当前编辑的是正在使用的 profile（桌面端是 `desktop`）。
- 单次调用超时可以调大 `toolCallTimeoutMs`，默认 60000。

:::

::: details 为什么用 API Key，而不是浏览器授权？

DeepSeek Harness 的 `@deepseek-ai/dsh-mcp-client` 暂时不支持通过 OAuth 打开浏览器登录，只能手动添加 `X-API-Key` 来绑定账号。

:::

[DeepSeek Harness ↗](https://github.com/deepseek-ai/deepseek-harness)

# OAuth 授权

每个工具声明所需权限：读取需要 wiki:read，编辑已有页面需要 wiki:edit，新建页面需要 wiki:create，移动、删除、上传分别需要 wiki:move、wiki:delete、wiki:upload，连接本身需要 mcp:access。收到 INSUFFICIENT_SCOPE 时，说明当前授权缺少所需权限，请用户重新发起授权并勾选对应权限。

wiki.me 输入 {} 获取当前 Wiki 用户 ID、用户名、显示名及 MCP clientInfo，仅需 mcp:access；clientInfo 由客户端自行声明，不代表已验证的身份。

wiki.logout 输入 {} 撤销当前连接对应的授权及关联令牌；成功后后续 MCP 请求被拒绝，需要重新授权。

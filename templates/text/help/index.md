# Gateway 帮助

编辑使用单页面草稿（有过期时间），上传使用 prepare → PUT → commit。调用前检查所需 scope；工具发现不会自动授予权限。

{{if .}}可用主题：{{range .Topics}}{{.}} {{end}}

可用功能：{{range .Features}}{{.}} {{end}}

可用工具：
{{range .Operations}}
- {{.}}
{{end}}
{{end}}

通过 wiki.help(topic) 或 wiki://help/{topic} 获取同源帮助。核心流程也可完全通过 Tools 使用。

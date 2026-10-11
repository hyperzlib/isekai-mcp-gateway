# 页面读取

resolve_page_url 将完整 HTTP(S) 页面 URL 或站内绝对路径（如 /首页、/index.php?title=首页）解析为 title、page_id。

页面读取工具统一用顶层 title 或 page_id 选择页面，恰好提供一个，优先使用含命名空间的 title。get_page_structure、get_page_dependencies 还接受 revision_id 锁定某次正式修订版。

- get_latest_page：最新可见正文；启用 Moderation 时可能是当前用户提交的审核中的修订版，其 temp_revision_id 不能当作正式 revision_id。
- get_baseline_page：始终读取公开正式修订。

默认不跟随重定向，需要目标正文时传 follow_redirects=true。结果返回规范 title、page_id、revision_id、redirect_chain 和 resources；编辑优先使用 page: {page_id}，新建时才用 title。

如果需要编辑先前新建，但仍在审核中， 没有正式创建的页面，也需要使用 title 指定。

分页用 next_cursor 续页，并保持相同的 title/page_id 与筛选参数。source 和 html 类结果用 UTF-8 字节 offset/limit 读取。上游失败不能解释为页面不存在。

title 需要符合：

```javascript
let titleEncoded = encodeURIComponent(titleText)
```

编码格式。（需要转义的字符与encodeURIComponent一致，不强制使用js进行编码）

## 章节结构和页面依赖

get_page_structure 返回章节目录（index、level、anchor 等字段）。编辑章节前检查 fromtitle：模板嵌入的章节要改来源模板；目录里的标题可能含显示用 HTML，不能直接当作 wikitext 标题。

get_page_dependencies 返回 {type, data, resources?}，dependency_types 可按需选择 templates、links、images、categories、external_links（省略为全部）。

续页固定首次解析的正式修订；若模板变化导致解析结果改变会返回 SNAPSHOT_UNAVAILABLE，此时重新开始读取。

## 文件与嵌入引用

get_latest_page、get_baseline_page 在 format=source（含默认格式）时额外返回完整 linked_pages 和 file_urls。linked_pages 包含命名空间、模板展开产生的页面链接和不存在页面的链接，无链接时为 []。file_urls 完整页面（含模板展开）用到的 `文件:名称` 映射到原始文件 URL。HTML 不返回这两个字段。

也可以用 get_file_urls 直接解析 1–500 个带文件命名空间的标题，会返回独立的 file_urls；缺失文件会被省略。

get_template_data 读取页面是否声明了 TemplateData：has_template_data=false、data=null 表示只有页面没有声明，不要把从 wikitext 推测的参数当作已声明项。has_template_data=true 时 data 含参数定义、类型、别名、必填项和说明，可用 language 指定说明语言。

get_transclusions 查询哪些页面嵌入了当前页面（一般是指模板页面被其他页面嵌入），与 get_backlinks 的普通链接不同，适合评估修改模板的影响范围。

资源 URI（如 pages/by-title、/history、/template-data 等）直接取结果里的 resources 链接即可，作为 Tool 的备用入口；筛选和分页仍优先用 Tool。

## 子页面结构

list_subpages 用顶层 title 或 page_id（二选一）分页列出同一命名空间下的全部后代，包含多层子页面和重定向。limit 默认 20，范围 1–100；续页传 cursor，并保持原选择参数和 limit。results 每条包含 pageid、ns、title、relative_title、depth 和 resources，直接子页面 depth=1。不存在的父标题仍可查询其后代，父 page_id=null；不存在的 page_id 返回 NOT_FOUND。

异世界百科使用子页面结构组织复杂设定。

命名格式例如：`世界观页面名/设定名` 或 `世界观页面名/人设/人物名`

你可以阅读上级页面来获取更详细的设定信息。

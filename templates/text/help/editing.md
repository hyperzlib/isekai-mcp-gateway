# 工具使用

begin_edit 或 begin_create 返回 edit_id、version 和 expires_at 过期时间。每次修改传最新 base_version。两个工具分别需要 wiki:edit 和 wiki:create 授权，授权被拒时看 wiki.help(authorization)，不要重试。begin_create 遇到已存在的页面（包括该页面有待审修订）会失败，此时改用 begin_edit 编辑本人待审快照，详见 `wiki.help(moderation)`。

同一轮任务中，对同一页面的所有修改和提交应复用已有的 edit_id，不要在每次 commit_edit 后重新调用 begin_edit 或 begin_create。

update_edit 支持 replace_content、replace_text、replace_lines、insert_lines、delete_lines、apply_patch。operations 按顺序应用，任何一步失败则整批不保存。replace_text 必须唯一精确匹配。行号从 1 开始，范围包含两端；展示行号不属于正文。

get_edit_content 默认返回 200 行及行号；expected_version 固定分页。长单行可用互斥的 offset/limit 字节输入读取。search_edit_content 使用字面或 Go 线性时间正则，逐行搜索；默认字面匹配、大小写敏感、上下文 3 行、最多 20 处命中。

仅 ready 可修改及提交。loading 时读取 get_edit_status，其他正文操作等待占用结束。error 状态仅保留草稿读取、搜索、状态与关闭入口。

提交前使用 preview_edit 检查页面渲染，并通过 get_edit_diff 核对修改范围及内容。重点检查模板、内部链接、标签、表格和非预期删除。对于较大的修改，应检查完整差异或分段核对.

当前任务中不再需要编辑该页面时，调用 close_edit 关闭会话。任务结束前，应关闭本轮任务中不再使用的Edit Session。

过期、提交失败和冲突的恢复流程见 wiki.help(recovery)。

commit_edit 必须明确提供 ai_involvement，具体看 [AI标注](#AI标注) 章节

# AI标注

本站面向世界观编写者，读者阅读时关心设定层面的决定由谁做出。

使用 commit_edit 时需要提供以下标签中的一个。

- none: 仅帮忙将用户提供的内容发布，或转换为 wikitext 、进行排版后发布，中途不引入新内容。
- ai_organized: AI 仅组织既有材料、措辞排版，并且在仅排版的基础上添加总结、见解等，主观上没有修改设定内容。
- ai_assisted: 在写作前作者已经提出设定骨架（规则、因果、与其他设定的关系及其自洽逻辑），AI 只展开细节、补充表述。判断标准：删去 AI 的贡献后设定仍然基本成立。
- ai_generated: 用户仅给出设定方向或笼统概念，设定层面的决定主要由 AI 产生。若用户在后续大量对话中探讨、指导和完善了设定内容或逻辑，则可以降为 ai_assisted。

如果本次编辑只涉及已有内容的整理、纠错或格式修改，不应因为页面编辑历史或上次提交时存在AI生成内容而提高标注等级，标注等级仅代表当前提交的版本。

# apply_patch

apply_patch 使用了一个略微修改版本的 V4A diff format。与原版不同之处在于不要包含文件头或其他 diff 指令，包括 `*** Update File:`、`*** Add File:`、`*** Delete File:`、`*** Move to:`、`*** End of File`、`diff --git`、`---` / `+++` 文件头、`@@ -1,3 +1,3 @@` 或 `\ No newline at end of file`。保留V4A格式所需的 `*** Begin Patch`、`*** End Patch`、`@@` 上下文定位及行首 `+`、`-`、空格标记。这里只禁止文件操作指令及标准unified diff格式。

如果你不了解 V4A diff format，先使用 `wiki.help(patch)` 读取文档。

# 编写页面

如果你不知道 wikitext 的语法，先使用 `wiki.help(wikitext)` 读取 wikitext 的编写指南。写好后先 preview_edit 检查渲染。用户没有另有要求时，文风按 `wiki.help(default-writing-style)` 的默认写作风格处理。

本站常用 Template 组件的写法可以通过 `wiki.get_latest_page` 读取站内页面《常用组件》的 wikitext 得到。页面中各组件“用法：”后的代码即对应的模板调用语法。模板参数说明可用 `wiki.get_template_data` 查询。

## 内容编写与排版规范

编辑百科内容时，遵循以下默认规则：

- 不要主动添加 Emoji、颜文字或装饰性符号，除非它们本身属于内容。
- 中文与英文、数字之间不默认添加空格，遵循自然的中文排版习惯。例如使用“通过MediaWiki API编辑页面”，而不是“通过 MediaWiki API 编辑页面”。
- 不要滥用粗体、斜体、引号、分隔线或其他强调性格式。
- 不要添加没有实际内容价值的开场白、结束语或总结。
- 保持原有专有名词、术语、链接及必要的格式，不要为了统一排版而修改其含义。
- 用户明确指定的格式、文风和页面既有约定优先于上述默认规则。

编写或大幅改写百科正文时，如果用户没有指定文风，请先阅读 `wiki.help(default-writing-style)` 指南，并以此作为默认写作风格。编辑已有页面时，应优先保持页面现有的合理文风。

# 创建新页面

新建页面的命名规则见 `wiki.help(page-creation)`

# 发布现有文档

如果用户要求将现有文档上传到本站，在用户没有另外要求下，默认仅将文档内容转换为 wikitext，不修改文档内容，并且标注为 ai_involvement=none 后提交。

如果文档中有文件，先按 `wiki.help(uploads)` 的 prepare_upload → commit_upload 流程上传文件，拿到可在 wikitext 中引用的 `[[文件:名称]]`，把原文档里的图片链接替换为上传后的新链接，再提交页面内容。

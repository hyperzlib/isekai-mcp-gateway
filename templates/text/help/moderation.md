{{if .}}{{if .ModerationEnabled}}# 待审编辑

读取最新页面内容时，如果当前用户有待审内容，会返回最新待审正文，baseline 读取始终返回会公开展示的正式修订版。moderation_base_revision_id 与正式 base_revision_id 分开保存，零值表示要求不存在有效待审快照。

提交入队和相同待审正文的 null edit 都是成功结果，并结束会话。Gateway 返回本次结果的快照标识，不跟踪审批。公开保存成功后重置待审提交基线为零并继续同一会话。
{{else}}# 待审编辑

本站未启用待审编辑扩展。提交按公开保存处理，无需 moderation_baserevid，commit 返回的 content_source 为 baseline。{{end}}{{end}}

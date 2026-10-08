# 恢复与冲突

MediaWiki API 网络故障不会自动重发。commit 的保存请求可能已到达上游，先读 get_edit_status，再调用 pull_edit；不能盲目再次发布。unsync 保留草稿，pull 成功后恢复 ready。

pull 后最新正文与草稿相同时仅刷新基线；不同时，所有差异写入同一草稿的 current/edited 冲突块。通过 get_edit_status 或 get_edit_conflict 获取冲突范围，用 get_edit_content 读取，再用 replace_lines、replace_text 或 apply_patch 解决，删除冲突分隔符并保留最终正文。残留、损坏或孤立标记会阻止预览及提交，解决后主动提交最新版本。

公开保存及核心 nochange 刷新基线并保留会话，使用返回的新 version 继续编辑。提交审核及 Moderation null edit 结束会话，后续重新 begin_edit。

移动、删除或上传结果未知时按明确目标核验上游状态。unknown 上传票据禁止再次发布。原正式页面被删除或同名重建时进入 error，可读取草稿或关闭。

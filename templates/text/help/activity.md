# 贡献与最近修改

wiki.get_user_contributions 使用 user_id 或 username，恰好提供一个。wiki.get_my_contributions 从当前认证连接取得 MediaWiki 用户 ID，无需也不允许传入用户选择器。贡献查询覆盖本站公开修订，不包括尚未发布的 Moderation 编辑。

wiki.get_recent_changes 默认查询全站 edit 和 new；types 可以指定 edit、new、log、categorize、external。加入 log 可查看近期移动、删除和上传操作。可选 user_id 或 username 用于筛选某个用户。最近修改受站点的保留期限限制。

三个工具均支持 limit（默认 20，最大 100）、cursor、start/end（带时区的 RFC3339）、direction（默认 older）、namespaces、show 和 tag。older 按新到旧，start 不能早于 end；newer 按旧到新，start 不能晚于 end。不要混用互相矛盾的 show 条件。

results 保留 MediaWiki 的修订、父修订、页面、时间、摘要和标记字段，附带页面资源链接。next_cursor 为字符串或 null；续页保持全部筛选参数不变。

资源使用 application/json，并采用默认的第一页和筛选条件：

- wiki://me/contributions
- wiki://users/by-id/{user_id}/contributions
- wiki://users/by-name/{encoded_username}/contributions
- wiki://recent-changes

encoded_username 为完整用户名作为一个路径段的 UTF-8 百分号编码，只解码一次。资源返回 next_cursor 后，使用对应 Tool 继续读取；资源中的 default 链接指向默认查询，不表示保存了本次筛选条件。

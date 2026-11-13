# 草稿状态

草稿会话（edit_id）有四个可用状态，外加 closed 与过期两种终点：

- ready：唯一可以 update_edit、preview_edit、commit_edit、close_edit 的状态。
  若返回的 conflict_count > 0，预览与提交会被拒绝：先按冲突范围改草稿、删掉冲突分隔符，再预览提交。
- loading：有操作正在占用（提交、pull 或快照加载）。此时只能读 get_edit_status / get_edit_content，
  其余操作等它回到 ready；不要重试写操作。
- unsync：上游结果未知或正文已变。草稿保留，但不能改、不能提交，也不要重复发布。
  处置固定为：先 get_edit_status，再 pull_edit。pull 成功回到 ready，差异以 current/edited 冲突块写进草稿，
  解决冲突后才可提交。
- error：原正式页面被删除或同名重建。只保留读草稿、搜索、查状态与 close_edit。

转移：begin_edit / begin_create → loading 或 ready；ready → unsync（提交结果未知、上游冲突）；
unsync → ready（pull 成功）；ready → error（页面消失）；
任意状态 → closed（close_edit 成功，或提交入队 / null edit 结束会话）；expires_at 到期即作废，需重新 begin。

每次成功的改写都会递增 version；下一次调用必须传最新 base_version，不要复用旧值、不要靠猜。
close_edit 在 loading 时只登记 close_requested，操作结束后才真正 closed。
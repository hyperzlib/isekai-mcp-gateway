# 操作确认

move_page、delete_page、close_edit、commit_upload 以及提交他人创建过的页面可能返回确认要求（POLICY_CHALLENGE）。如果用户没有明确提到允许此操作，则向用户展示返回的确认参数，获得确认后在独立调用中重新提交原工具，附带 challenge_responses: [{challenge_id, confirmed: true}]。

确认绑定该次调用的参数：关闭确认额外绑定草稿版本；移动目标、删除原因或上传意图变化会使旧确认失效。明确失败可复用同一确认重试；结果未知时不要重用，先查询结果。

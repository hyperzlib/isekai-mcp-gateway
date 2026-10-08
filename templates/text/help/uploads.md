# 上传文件

prepare_upload 提交 filename、mime_type、size_bytes、sha256、comment、overwrite。使用返回 URL 和 headers 发送原始字节 HTTP PUT，接收成功后调用 commit_upload。PUT 只暂存文件。

首次 warning 不会被忽略。向用户展示 warnings，下一次 commit_upload 提供该票据返回的全部 confirmed_warnings。相同文件无需再次传输。prepare_upload 的恢复分支仅提供 retry_upload_id。

上传 URL 是短期接收凭据，应保密。暂存文件有固定期限。结果未知时查询上游文件信息，该票据禁止再次发布。

# Sub2API v1.3.7

本版本修复媒体图片、音频、视频并发错误使用能力默认值的问题，统一改为使用账号配置中的并发数。

- 媒体任务入队和 worker 抢占按账号 ID 加锁并统计，账号下不同模型共享同一个并发预算；`LoadFactor` 继续只影响调度权重。
- `manual_review` 任务继续占用一个账号预算槽位，但不会再把能力级默认并发 2 作为拒绝条件；账号并发未满时新请求进入队列并返回 202。
- 真实达到账号并发上限时返回结构化 `account_concurrency_exhausted`、占用快照和 `Retry-After`；上游 429 与本站账号限流分开记录。
- 媒体失败诊断保存经过脱敏的上游错误码、状态码、request ID 和消息；`ops_error_logs` 增加账号并发、在途、排队和人工核对快照。
- 新增 `252_media_account_concurrency_diagnostics.sql`，只增加可空诊断字段，不改变现有业务数据。

发布前已完成生产 relay 的只读 `GET /v1/models` 连通性核验，未提交新的付费生成请求。详细脱敏记录见 `CHANNEL_VERIFICATION_CURRENT.json`。

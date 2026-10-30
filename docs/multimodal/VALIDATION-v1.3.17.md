# v1.3.17 验证记录

## 本地回归

- 媒体管理端与诊断界面定向测试：2 个文件、18 项通过；文档浏览器脚本测试：12 项通过；标准客户端 4 项通过；OpenAPI 3.1 验收覆盖 27 个路径、30 个操作并通过。
- `go test -tags=unit ./...`：通过；完整前端测试：381 个文件、2,917 项通过；`pnpm lint:check`、`pnpm typecheck`、i18n 校验及 `pnpm build`：通过。
- 受限部署器、管理器和 Release 清单测试：74 项通过、1 项因缺少本地集成数据库而跳过，包含显式 `--immediate` 参数传递与任务持久化。`.playwright-mcp/` 是现存本地目录，未纳入发布。
- 数据库集成测试仅在提供隔离的 `MEDIA_TEST_DATABASE_URL` 时运行；CI Release 工作流配置独立 PostgreSQL 并执行集成测试。

## 上游与生产

- 上游真实契约验证：模型 `gemini-omni-1.1-flash` 在 `https://img.yunfei.best` 完成一次单图视频生成。以整数 `duration`、单张图片 data URL 字符串提交到 `Alias.images`，经模型/账号/分组绑定从入站图片对象数组映射而来；任务完成，成品端点 HTTP 200，响应 `video/mp4`，389,371 字节。提交次数 1，凭证未保存。
- 单独的错误类型请求曾将 `duration` 作为字符串，上游以 HTTP 400 明确拒绝且未返回任务 ID。改为整数后只提交一次；不确定的接受结果不会自动重试。
- 前端构建输出现存的 Browserslist 数据过旧、混合动态/静态导入和大 chunk 提示；构建成功。
- v1.3.16 Release 的隔离 PostgreSQL 集成测试发现旧断言仍要求 Create 同步提交；已在 v1.3.17 更新测试契约为“持久化后 queued、worker 再提交”，其余本地回归保持通过。正式 Release、生产立即切换、回滚状态及三个生产入口的线上版本/健康检查在部署后补记。立即切换跳过空闲等待和 drain，可能中断正在进行的应用请求；正式 Release 工作流仍执行备份、恢复演练和上线检查，并保护 PostgreSQL 与 Redis 容器。


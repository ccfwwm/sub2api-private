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
- v1.3.16 Release 的隔离 PostgreSQL 集成测试发现旧断言仍要求 Create 同步提交；已在 v1.3.17 更新测试契约为“持久化后 queued、worker 再提交”，其余本地回归保持通过。正式 Release Action `41031800577` 完成并发布 `v1.3.17`，源码提交 `f1c0c3579fa846e5a750b2ff048fb0a9b289b967`。
- 生产立即切换：自动部署任务 `e8180a2ea1e54b5db924153a337c5e17` 于 2026-10-30 12:08 UTC 返回 `succeeded / online_verified`；管理器任务 `3a01e0b8d028400286e2638562ec68e6`，备份 `e7d7722357ef4879afa69cf751641e97`，切换模式 `immediate`。应用镜像 digest 为 `sha256:82a78f27ac157ede3c31f39231fb2f14004bfafd3df4c53c0f8ef7b161f83310`，文档镜像 digest 为 `sha256:b5b7dfb646e34f97bc4e4757e053ad95444fcdf9e60223b7998d0f2a7363c822`。PostgreSQL 和 Redis 容器 ID、启动时间与切换前一致；管理器完成备份、隔离恢复演练和上线检查。此次部署没有新增供应商付费生成请求。
- 三个生产入口 `https://code.aicodeme.cn`、`https://code.aicodeme.xyz`、`https://hkcode.aicodeme.xyz` 的 `/health`、`/docs/imagevideo`、`/docs/app.mjs`、`/api/v1/public-docs/imagevideo` 均返回 HTTP 200；三个 `/api/v1/settings/public` 均报告版本 `1.3.17`。文档脚本响应使用 `Cache-Control: no-store`，侧栏徽标硬编码为 `MEDIA API · v1.3.16`；不影响文档测试、预览、提交、任务查询或成品查看，记录为下一版本修复项。
- 部署期间识别到生产主机预装的自动部署器不接受 `--immediate`。从正式 Release 下载的 `server-management.tar.gz` 与 `management-checksums.txt` 校验一致，管理源码提交与标签一致；按包内 `install-manager.sh` 更新管理脚本后，使用受限自动部署入口完成上述升级。一次早期直接调用管理器因缺少预登记的 `deployment.json` 在备份前失败，未切换生产；历史失败任务保留。
- 立即切换跳过空闲等待和 drain，可能中断正在进行的应用请求；正式部署仍执行备份、恢复演练、迁移门禁和上线检查，并保护 PostgreSQL 与 Redis 容器。


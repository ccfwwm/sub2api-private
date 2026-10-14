# imagevideo 文档与媒体任务生产发布验收

2026-10-14 已部署 `v1.2.5`，生产自动任务状态为 `succeeded / online_verified`。公开文档：https://code.aicodeme.xyz/docs/imagevideo 。管理员与用户使用记录内新增“媒体任务”Tab，支持分页、筛选、列设置与详情。

## 发布身份与分支

- 生产提交：`ced4df335010b5f808fbe1fb8bb0cc812f3d9579`。
- 私有 Release：https://github.com/ccfwwm/sub2api-private/releases/tag/v1.2.5 。
- 固定镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:9c5983e801c3d5d0768b94e6af2aaf573a8e65f5fa1ad1730d61e7a89bf18d45`。
- 自动部署任务：`6394daca931f4d84be5f8a7be994e4c7`；管理器任务：`2fef393d1ca545eba65981418407ce37`。
- 备份：`f68c95c30e384da887df321232e7c509`。8 个备份文件校验值全部通过；备份与凭据保留在服务器 root-only 目录。
- 功能及发布修复均已进入私有 `main`；本地 `main` 跟踪 `origin/main`。后续开发直接使用 `main`，普通 push 不发布生产，约定见 `DEV_GUIDE.md`。

## 测试与发布方式

`v1.2.4` 的 6 处 Go 静态检查错误已修复，原标签保持不变。`v1.2.5` 源码 CI [38532903345](https://github.com/ccfwwm/sub2api-private/actions/runs/38532903345) 全部通过；正式发布 [38532903463](https://github.com/ccfwwm/sub2api-private/actions/runs/38532903463) 的源代码测试全部通过，包括前端、Go 单元与 PostgreSQL 集成、客户端契约、管理工具、静态检查和依赖检查。

GitHub 账户付款或额度限制阻止镜像作业启动，没有构建步骤被执行。使用已通过检查的精确提交 `git archive`，在本地专用 Docker-in-Docker 中按原始 Dockerfile 构建，不含本地配置、未跟踪文件或凭据。新镜像通过 smoke（健康、数据库初始化、嵌入前端、版本和管理员登录），以及完整备份、数据库指纹、恢复、切换、捕获实际运行程序的回滚、索引 247 升级和应急切回演练。

本地演练入口只允许本轮专用 Docker engine ID，固定镜像仓库改为隔离环境的 localhost 仓库；生产管理代码的备份、锁、健康、迁移和回滚检查未移除。GHCR 发布与本地已演练镜像摘要完全相同，提交标签、版本、文件层一致。正式 Release 资产包含精确提交、297 条迁移清单、受审查 SQL 和管理器文件校验值；服务器现有受限自动部署器重新完成生产备份、真实数据库副本恢复及迁移演练后，仅切换应用容器。

GitHub 的自动镜像作业仍受账户付款或额度限制，需要在后续自动发布前处理；本次生产发布与独立验收已完成。

## 生产独立验收

验收时间：`2026-10-14T09:43:21.513644+00:00`。

- 本机入口与 `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 均健康 HTTP 200、版本 `1.2.5`，免登录文档 HTTP 200。
- 应用 healthy，镜像版本、提交及摘要匹配；应用持久化挂载、网络和端口保持原样，维护标记已撤除。
- 旧 296 条迁移 checksum 全部一致，仅增加 `247_media_task_list_indexes.sql`。全局及用户维度两个索引 valid/ready，定义包含 `created_at DESC, id DESC`。
- 管理器迁移证据确认 `restore_verified`、`rollback_verified`、`data_unchanged` 均为 true；演练核对媒体任务、账务、成本、制品和原业务表指纹。
- 基线中的 6 个历史视频/音频文件大小和 SHA256 全部保持一致。
- 34 个相邻容器 ID、镜像、启动时间、挂载、网络和端口完全一致。`gptimageweb-pro-frontend-1` 容器与镜像 ID 保持原样，但在 `08:33:05 UTC` 重启过，早于本次部署开始 `09:28:23 UTC`；未发现本次切换重建该服务。
- 管理员真实列表验收时有 11 条任务，以每页 2 条遍历 6 页，无重复或遗漏，顺序与数据库 `created_at DESC,id DESC` 一致。3 组组合筛选、空结果、旧数组响应及 `before` 游标均通过，4 组非法参数均返回 400。匿名访问用户和管理员任务列表返回 401。
- 两个在线 Python/JavaScript 下载文件 SHA256 与发布源码一致，根地址正确，不含参考站名称或域名。
- 无登录、无模拟 API 的独立浏览器环境验证：10 个公开章节、复制完整 Markdown 与共用内容源一致（统一 Windows 剪贴板换行后），390px 手机目录与音乐锚点正常，无横向页面溢出、无 JavaScript 错误。

生产管理员 API 验收使用服务器已有 API Key，凭据始终留在服务器，没有创建或修改账号。普通用户生产登录会话及登录后的媒体 Tab 未进行现场浏览器复测；跨用户隔离、用户字段隐私，以及管理员/用户桌面、手机、深浅主题界面已通过本地单元、数据库集成和模拟浏览器验证，详见 [实施验证记录](IMAGEVIDEO_DOCS_VALIDATION.md)。生产没有执行退款、成本调整、受理核对或新的收费生成。

验收期间生产任务与账务数量随业务增长；不能以实时条数完全相等替代备份一致性验证。此次没有发起任何收费媒体生成。

## 证据与截图

- [生产验收摘要](production-v1.2.5-imagevideo-evidence.json)。
- [在线文档桌面截图](../../output/imagevideo/screenshots/production-docs-desktop.png)。
- [在线文档手机截图](../../output/imagevideo/screenshots/production-docs-mobile.png)。
- [媒体界面及本地验证截图索引](IMAGEVIDEO_DOCS_VALIDATION.md#浏览器验证与截图)。

本地临时隔离容器与专用卷已移除。生产备份、基线、发布任务和持久化数据保持原样；未执行全局清理、数据库恢复或相邻服务重启。

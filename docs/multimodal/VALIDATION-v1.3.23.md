# v1.3.23 部署验收

北京时间 2026-11-03 22:25:41 完成用户明确授权的立即切换。生产部署流水线 [41651720707](https://github.com/ccfwwm/sub2api-private/actions/runs/41651720707) 成功，自动任务 `08b1219d51204ff88c9f622e7e11f420` 为 `succeeded / online_verified`，管理器任务 `8b5d8a041df549e4b11762068d135c7f` 成功，备份 `19a0d36205034498aac6860343ccf919`。

- 发布标签 `v1.3.23`，源码 `fba2fc0fe87a85d32762a5b4fe956b67a1df3c5b`。
- 应用 digest `sha256:c693c004ffcb9497c5d6656b357f3e382f929b6a77eb79875837edbfeb1258fe`，文档 digest `sha256:7e6508464637ea6a951d3eeaa822de192f5feec7b3d033b4e2a45c1f0c84605d`，运行镜像身份及来源均核对一致。
- 三个公网入口 `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 的 `/health` 和公共设置接口均为 HTTP 200，版本均为 `1.3.23`；文档页和脚本均为 HTTP 200。文档版本徽标从公共设置动态读取，不在脚本中硬编码版本。
- 公网账号页面资源含 `openai_images_request_params` 和参数规则编辑控件；渠道监控资源含 `probe_type` 和 `custom_response_json_path`。这是已部署资源检查，本轮没有执行管理员浏览器表单验收。
- 生产登记的 310 个迁移与正式清单逐项一致；旧 309 个迁移 checksum 未变，仅增加 255。两张监控表的新增字段符合旧数据默认值，周期检测没有自动启用。
- PostgreSQL 与 Redis 容器 ID、启动时间均与部署前一致，未重启。

源码构建运行 [41640915567](https://github.com/ccfwwm/sub2api-private/actions/runs/41640915567) 的前端 lint、类型检查、关键回归，Go 全量单测、集成测试、漏洞检查、静态检查，文档与公开客户端校验和部署工具测试均通过。此前本地前端完整 397 个文件、3156 项测试和构建通过；部署工具 99 项运行，1 项专用 PostgreSQL fixture 测试跳过。镜像启动、匿名拉取、备份恢复、回退与救援演练通过。

首次发布在演练通过后因缺失版本说明停止；后续 finalize 运行 [41645953357](https://github.com/ccfwwm/sub2api-private/actions/runs/41645953357) 再次通过来源、镜像和演练检查，但创建 Release 被 Actions 令牌 HTTP 403 拒绝。因此最终用现有已授权 GitHub CLI 从原标签打包、逐项校验并发布同一固定镜像。没有移动原标签或重建产物。

常规生产部署运行 [41647925073](https://github.com/ccfwwm/sub2api-private/actions/runs/41647925073) 已完成备份、迁移恢复及旧数据检查；管理器任务 `c6465583084a40db842e98438efa058f` 的 `monitor_probe_migration.restore_verified`、`rollback_verified`、`data_unchanged` 均为 true，对应备份 `406625c3d3c1425c9a8e7eea04a871da`。迁移 255 在恢复验证后已应用，应用在等待自然空闲时仍为 v1.3.22。用户明确授权立即切换后，停止旧等待任务并保留其非成功记录，再执行上方成功的立即升级任务，重新备份与恢复验证。

本轮未提交真实渠道付费生图请求。新的参数和探测行为由回归测试覆盖，历史渠道案例仅作为基线，不能视为本版本真实生图验收。脱敏结果见 [生产证据](production-v1.3.23-evidence.json)，功能操作见 [使用说明](../IMAGE_PARAMS_AND_MONITOR_PROBES_CN.md)。

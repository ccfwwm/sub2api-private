# v1.3.22 生产验证记录

## 修复与测试

- 图片计费优先级已修复：渠道配置的按张价格覆盖分组通用图片尺寸价；分组模型专属图片价仍优先。
- 回归测试覆盖分组 4K 图片价 `$0.04` 与渠道按张价 `$0.70` 同时配置时，单张 `TotalCost` 和 `ActualCost` 均为 `$0.70`；也覆盖分组模型专属价优先于渠道价。
- 正式 Release `v1.3.22`：工作流 [41344212951](https://github.com/ccfwwm/sub2api-private/actions/runs/41344212951) 成功，发布提交 `f2ff604d1dafa860a2bdd69a3c9eae64c6f57189`。

## 生产切换

- 部署工作流 [41346870340](https://github.com/ccfwwm/sub2api-private/actions/runs/41346870340) 与宿主管理器任务 `61da3845d04845a2b4bcbaf3559a7e7f` 均成功；任务于 2026-11-01 20:34:48 UTC 完成，`cutover_mode=immediate`，没有等待空闲或 drain。部署前备份 ID 为 `52a50e7df2444d229706fdc153c760c2`，隔离恢复演练通过。
- 应用镜像 `ghcr.io/ccfwwm/sub2api-private@sha256:f0206d336667ba8b7dbfd31a5be36ca033ebb94ebc2d708372f13d96395b0101` healthy；文档镜像 `ghcr.io/ccfwwm/sub2api-private-docs@sha256:83935987f6341c5707731b87f41e1634ec8e18b6bea7b4b156d4ecd9abc9eb27` running，均与正式 Release manifest 一致。
- PostgreSQL 容器 `f3593bf6d71bbfa09c56e6fa6fde5e49c5dd0aa16f54a09e589fb336037fbd73` 和 Redis 容器 `9860f0d8485675414aaebccf339e69f60dfb97209a50631680c176212869e2bb` 的容器 ID、启动时间与切换前相同。309 项数据库迁移清单及校验和匹配。

## 线上核验

- `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 的 `/health` 与 `/api/v1/settings/public` 均返回 HTTP 200，公共版本均为 `1.3.22`。
- 三个入口的 `/docs/imagevideo` 和 `/docs/app.mjs` 均返回 HTTP 200，文档脚本包含 v1.3.22 标识。
- 本次没有提交真实渠道或付费图片生成请求。生产计费路径通过上述回归测试验证，线上验收核对了正式镜像身份、健康状态和公开版本。

当前渠道状态见 [CHANNEL_VERIFICATION_CURRENT.json](CHANNEL_VERIFICATION_CURRENT.json)。

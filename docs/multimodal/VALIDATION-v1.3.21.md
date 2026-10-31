# v1.3.21 生产验证记录

## 发布身份

- 正式 Release：[`v1.3.21`](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.3.21)，当前最新正式版本。
- 源码提交：`49d508b06682fc66e1b00110ff4dafced2d1e029`。
- 应用镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:983add395ca89187b8f8e7c49cdf951b3d6409b5b726810c9feb166ddafe8d16`。
- 文档镜像：`ghcr.io/ccfwwm/sub2api-private-docs@sha256:04455f8957d1bf2d0f29c8883dd89bd0a9e8c6e075e238fb93315896d96e87a4`。
- 本次使用已发布摘要，没有重新构建镜像或移动版本标签。

## 生产切换

- 2026-11-01 01:45:33（北京时间）宿主管理器任务 `a234f692dd92450abef63c869b80adb7` 以立即模式完成，最终状态 `succeeded`；备份 ID 为 `ad52ef93992e4502ae9c27116d76d9b3`。
- 切换前备份、隔离恢复和回滚演练通过。上游迁移涉及七个文件；迁移演练的恢复、回滚、管理员 Key schema、图片计费默认值及账务/媒体数据检查均通过。
- PostgreSQL 当前 309 条迁移与 v1.3.21 发布 manifest 的 309 条迁移逐项匹配，文件名和 checksum 均一致，无新增差异。
- 应用容器 `b1163fe6e2555bb901ac5f34aa20ba3c314502563f8cddfe42f02c0d9084433b` healthy，文档容器 `4bc8f073e42a247f6dd52c921dd85d7b9d5d59c530faedd6fa4ba4df757f0495` running；二者 revision 均为发布提交。PostgreSQL、Redis 容器身份和启动时间与切换前相同。

## 线上核验

- `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 的 `/health` 和 `/api/v1/settings/public` 均为 HTTP 200，返回公共版本 `1.3.21`。
- 三个入口的 `/docs/`、`/docs/imagevideo`、`/docs/app.mjs` 和 `/api/v1/public-docs/imagevideo` 均为 HTTP 200。浏览器实际渲染的文档徽标为 `MEDIA API · v1.3.21`。
- CI `41225974495` 与安全扫描 `41225974534` 均成功；正式 Release 工作流 `41203819601` 成功。
- 最新管理器包按服务器 `final-checksums.txt` 校验成功并已安装；安装只更新管理脚本，没有重启应用、数据库或 Redis。
- 本次切换没有提交付费生成请求。已记录的 Anthropic 低额请求发生在 v1.3.19，返回 HTTP 502、供应商费用 `$0.00`；它不作为 v1.3.21 的成功渠道验收。当前版本尚无新的成功真实生成记录。

生产总记录见 [PRODUCTION_ROLLOUT.md](PRODUCTION_ROLLOUT.md)，当前渠道状态见 [CHANNEL_VERIFICATION_CURRENT.json](CHANNEL_VERIFICATION_CURRENT.json)。

# v1.3.15 验证记录

## 本地回归

- `pnpm --dir frontend install --frozen-lockfile`、`node --test docs-web/app.test.mjs`：通过，文档单测 8 项通过。
- 媒体管理端和相关页面定向测试：4 个文件、26 项通过；前端全量测试：381 个文件、2,910 项通过。
- `pnpm --dir frontend lint:check`、`pnpm --dir frontend typecheck`、i18n 校验及 `pnpm --dir frontend build`：通过。构建保留现有 Browserslist 数据过旧、动态/静态导入和大 chunk 提示。
- `go test -tags=unit ./...`：通过。
- npm 官方源生产依赖完整审计由现有例外检查器通过；`decode-uri-component` 锁定 `0.5.1`。现存 `xlsx` 高危公告仅适用于管理员导出路径，按仓库已有、未过期例外处理，不代表依赖零漏洞。
- 浏览器本地 fixture 回归：空值能力目录正常选择；图片 2 个、视频 3 个（含拖入）、音频 2 个进入估价和提交 JSON；视频任务完成后展示可控件播放的结果预览。390px 视口无横向溢出、页面无脚本错误。API 全部由 fixture 接管，没有连接供应商或产生费用。
- 数据库迁移：无。

## 发布与生产

- 正式 Release：首次流水线完成全部构建与隔离验收，但因 `CHANNEL_VERIFICATION_CURRENT.json` 仍标记 v1.3.13 被发布清单门禁拒绝，未创建半成品 Release；记录已明确标注沿用 v1.3.13 的真实渠道验收基线并重新触发。
- 生产切换：已完成。GitHub Actions Run `41001306958` 校验正式 Release `v1.3.15` 后提交受限主机任务；远端任务 `e583a903d1454b34bbc2000577633048` 返回 `succeeded / online_verified`，备份 ID 为 `1bd7d274e0504895a621cb799e133a4e`，管理器任务为 `372a61be47804ddfb9bf6e59d8115e43`，应用提交为 `6db5379d750ad26171afc33ada8c7bf5e62b72ef`。
- 上线后独立核验：已完成。`https://code.aicodeme.cn`、`https://code.aicodeme.xyz`、`https://hkcode.aicodeme.xyz` 的 `/health` 和 `/docs/imagevideo` 均返回 HTTP 200；三个入口的 `/docs/app.mjs` 均标记 `MEDIA API · v1.3.15`，包含视频上传、拖拽、预览以及请求/响应查看逻辑；三个入口的 `/api/v1/public-docs/imagevideo` 均返回 HTTP 200。部署回报已包含线上验证阶段，PostgreSQL、Redis 和已有任务快照由管理器切换门禁核验。
- 切换前只读基线（2026-10-30）：三个生产入口 `/health`、`/docs/imagevideo`、文档脚本和样式均 HTTP 200；文档脚本仍标记 `v1.3.12`，公开文档 API 返回内容版本 15。
- 供应商付费生成：本次 UI 回归不重复提交；沿用 v1.3.13 已完成的映射模型及视频成品生产验收作为后端链路基线。

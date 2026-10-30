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
- 生产切换：待执行。按用户已授权的直接切换方式，完成正式 Release 清单校验、生产备份、隔离恢复演练和管理器升级。
- 上线后独立核验：待执行。检查三个生产域名的版本、健康、文档脚本/CSS、模型选择/上传界面，并确认 PostgreSQL、Redis 与已有未结算任务快照保持不变。
- 切换前只读基线（2026-10-30）：三个生产入口 `/health`、`/docs/imagevideo`、文档脚本和样式均 HTTP 200；文档脚本仍标记 `v1.3.12`，公开文档 API 返回内容版本 15。
- 供应商付费生成：本次 UI 回归不重复提交；沿用 v1.3.13 已完成的映射模型及视频成品生产验收作为后端链路基线。

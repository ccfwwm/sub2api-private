# imagevideo 文档与媒体任务生产发布验收

2026-10-14 已部署 `v1.2.6`，生产自动任务状态为 `succeeded / online_verified`。公开文档：[https://code.aicodeme.xyz/docs/imagevideo](https://code.aicodeme.xyz/docs/imagevideo)。管理员与用户使用记录内提供“媒体任务”Tab，支持分页、筛选、列设置与详情。

## 发布身份

- 生产提交：`cecaf2f8e1af01d94dfbea3a472974d75826cf4e`。
- 私有 Release：[v1.2.6](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.2.6)。
- 固定镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:be1a6ee34c58a4c32e9dc548544cb64a650a8f1f57fc5faf9fd73173e4ba41e8`。
- 自动部署任务：`6146851894e9488084c3ffe99fc2269a`；管理器任务：`fdf532e572ef48329d77345dec7f7b3b`。
- 备份：`fe873997a6b44f4ca5d91ea085cc7181`；备份清单 8 项校验通过，凭据和原始备份只保留在生产服务器受限目录。
- 代码已合并并推送到私有 `main`；后续开发继续使用 `main`。

## 公网与运行时回归

以下入口均返回 HTTP 200，公开配置版本均为 `1.2.6`，匿名文档页面可访问：

- `https://code.aicodeme.xyz`
- `https://code.aicodeme.cn`
- `https://hkcode.aicodeme.xyz`
- 生产本机入口 `http://127.0.0.1:8081`

生产应用为 healthy，容器标签、提交和镜像摘要与发布清单一致。PostgreSQL、Redis 和定价服务的容器 ID 与启动时间未发生变化；应用挂载、网络、端口及历史媒体文件保持不变。297 条迁移 checksum 与发布清单一致，两个媒体时间索引均为 valid/ready，定义分别为 `created_at DESC, id DESC` 和 `user_id, created_at DESC, id DESC`。

## 媒体任务接口回归

管理员真实列表以每页 2 条遍历 8 页，共 15 条记录，无重复或遗漏，顺序与数据库 `created_at DESC,id DESC` 一致。组合筛选、空结果、旧数组响应、`before` 游标和 4 组非法参数均通过；匿名访问用户和管理员任务列表均返回 401。用户字段隔离、管理员成本字段、详情和账务版本冲突保护已由本地单元、PostgreSQL 集成及模拟浏览器回归覆盖。

生产下载的 Python 和 JavaScript 示例与 `v1.2.6` 发布源码一致：

- `imagevideo.py`：`47c802cc1777d5c01af8b15ed1314e61e325842d94480f29f5da0f217f7054cb`
- `imagevideo.mjs`：`07c7841220f3ed87defba4e88cb6ce9089297da61caad9d1e8048ef04e0d1fed`

没有发起新的收费媒体生成请求。生产验收使用服务器已有只读管理 API Key，凭据没有写入仓库或报告。

## 构建说明

GitHub Actions 的本次 CI、Security Scan 和 Private Release 任务因账号账单/额度限制显示失败，未进入镜像构建步骤。生产使用同一提交在本地专用 Docker 中完成测试和构建后推送的固定镜像；生产控制器按摘要校验并完成备份、切换和在线验证。该平台限制不影响本次已部署镜像的提交、摘要和公网回归结果。

## 证据边界

本记录证明发布身份、生产运行状态、接口兼容性和数据保留结果；尚未逐一对全部媒体模型和素材组合执行收费生成验收，也未执行退款、成本调整或受理核对。历史 `v1.2.5` 记录保留在 [IMAGEVIDEO_PRODUCTION_V125.md](IMAGEVIDEO_PRODUCTION_V125.md)。

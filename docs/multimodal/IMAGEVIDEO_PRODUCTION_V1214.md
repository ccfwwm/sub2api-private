# v1.2.14 媒体参考素材生产交付记录

2026-10-20 已将 Sub2API 从 v1.2.13 升级到 v1.2.14。分支 `main`，本轮父 HEAD `f860bf193f5148d9ff0a6ba6fec3f2d51711b180`，实际构建提交 `f0d5fb9367cec40fd30ec16801d677b1ba1aa867`，构建 ID `media-v1214-f0d5fb936`。完整机器记录见 [PRODUCTION_v1.2.14.json](PRODUCTION_v1.2.14.json)。

修复媒体参考素材、未知可选参数及素材对象附加属性转发；V6K 使用 `images`、`videos`、`audios`，内部类型保留 `video`。原生和云飞协议保留自己的字段映射。新任务加密保存实际发送摘要、请求体 SHA-256、素材计数和脱敏响应，区分 `prepared`、`attempted`、`accepted`；历史无快照时显示未记录。API Key 的原任务查询返回 `submission`，管理员详情展示完整脱敏审阅摘要。普通用户详情目前展示失败原因、金额及素材测量数量，不展示该审阅面板。

## 部署与恢复证据

- 升级任务 `8bf4acce65b44bdbba1c94f814dba7d3` 最终 `succeeded`，采用自然空闲、排空后切换。
- 备份 `53a679cf9a7b492f9ea84229db690125` 包含一致性数据库、Redis、应用文件、配置及新旧程序应急镜像；隔离恢复的数据指纹与新旧程序启动演练通过。没有恢复覆盖生产数据库。
- PostgreSQL、Redis 的容器 ID 和启动时间均保持不变，298 条迁移与旧版本清单完全一致；没有修改定价或历史账务。
- 应用镜像 `ghcr.io/ccfwwm/sub2api-private:v1.2.14`，digest `sha256:493e74ac187d444f1cca625fc5f3e6dd64c9c42a99a8aae2941e7f8aac6dca12`，本地大小 372,010,656 字节。
- 文档镜像 `ghcr.io/ccfwwm/sub2api-private-docs:v1.2.14`，digest `sha256:a9168b691cb1341b50b0bb5d3c5f559acbdf399486bf4722b01336d47777c74e`，本地大小 191,945,335 字节。
- 本地清单 `output/media-v1214/deployment.json` SHA-256 `b13abdb051bebe4c25373057bfcf46bd00a7936c75e041c21876c2887ecac6c1`。GitHub Release 附清单、校验和、管理工具及本交付记录。

三个公开入口 `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 的首页、健康、Key、文档及公开版本接口全部 200；运行版本 1.2.14，公开文档正文与构建源完全一致。浏览器绕过旧缓存后显示 v1.2.14，登录后的用户媒体任务列表、详情和费用信息正常。

## 五份素材真实测试

使用用户原账号现有媒体 Key。输入清单 SHA-256 `0f8a636e2f76f80fdcbd285afa09b6deccf154f0fd7cde92cf242f54fc39e08e`，包含官网视频 1 个、图片 4 张。两次均使用精确模型 `TTseadanco2.5-720`、5 秒、同一提示词及全部五个 URL，无换模型、删素材或纯文本降级。已用 2/3 次，剩余一次未使用。

| 场景 | 网关任务 | 实际发送 | 上游结果 | 实际结算 |
| --- | --- | --- | --- | --- |
| 保留清单中的 role、media_type 属性 | `mt_3f1f39a1-a990-4e4e-bdb7-da19506ebc10` | 图片 4、视频 1，URL 全部匹配 | HTTP 400 `invalid_request`，参数不正确 | USD 0 |
| 按项目原参数使用 `{url}` 对象 | `mt_15a957f6-ee08-46c0-afab-08a37495cabe` | 图片 4、视频 1，URL 全部匹配 | HTTP 404 `task_not_found`，任务或文件不存在或已失效 | USD 0 |

两次预留 USD 3 均已释放。回执阶段均为 `attempted`，没有有效上游任务 ID，不能声称已受理或已生成。请求体 SHA-256 分别为 `81682087f8264eed90a63495364477b2cbef79703731f1c3d97b101c7f2c98ac`、`158ccb81745be2bf391aafe820a63ed1c93799b9d5791e587da4d2e6eeebbd15`；完整脱敏请求、响应和上游 request ID 保存在机器记录。

已证明本版本完整转发参考素材；TT 接收这组素材并输出正确成片的验收仍未通过。第二次错误来自 POST 创建请求的真实响应，不能解释为详情页隐藏字段，也不能据此断言是本地素材 URL 过期。供应商当前 OpenAPI 1.2.0 描述了 `images`、`videos` URL 对象，是否适用于该 TT 模型仍需供应商核对上述 request ID。没有生成产物可进行内容验收。

## 检查与限制

前端 369 个测试文件、2,829 项测试；57 项隔离 PostgreSQL 媒体集成测试；58 项部署工具测试；3 项文档测试；14 组规范及 Python/Node 客户端模拟检查；媒体单测和相关 handler 单测均通过。前端生产构建、媒体 go vet/govulncheck、生产依赖审计例外检查、全新镜像数据库初始化/版本/管理员登录检查通过。

GitHub Actions 因账户账单/额度限制没有启动，本轮沿用上一版本的本地构建和既有生产管理器，没有伪造托管 CI 成功。未重跑整个后端单测、完整 lint/security 和 Go race。用户用量统计的模型分布接口在切换前已返回 HTTP 500，媒体任务列表和详情可正常使用；该独立统计问题不计作媒体修复验收通过。

ZeroWall 桌面没有重打安装器；DSH 仍为 `683cb0318b9a0fa09362794588d01acb0366850f`。本轮没有改动已发布的 AI Cloud 0.1.30、Videos 0.2.2。完整 Host/Electron 验收及旧插件显式逐包回滚兼容性限制沿用插件发布记录，不将其报告为已通过。

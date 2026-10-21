# 多模态生产发布记录

## 当前生产：v1.2.16

北京时间 **2026-10-21 23:41:31** 已按用户授权完成立即切换。升级任务最终状态为 `succeeded`；公网 `/health` 和 `/api/v1/settings/public` 均返回 HTTP 200，版本为 `1.2.16`。

- 应用提交及不可变标签：`5625b929e19d4b27a638747f0bf230ca220073df`、`v1.2.16`。
- 应用镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:a8715d8e2870ce55bd6ae8284a86364c88365a08d4577ab46b41196793e9305e`。
- 文档镜像：`ghcr.io/ccfwwm/sub2api-private-docs@sha256:027d9a11c79c9787c300c5700e83a34ad2c01611a3c833b81242ea917c9525eb`。
- [正式发布 v1.2.16](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.2.16)：19 个资产，下载回读 SHA 校验通过。
- 升级任务：`17abb0b0faea4941a30dfae52644f65e`；备份：`00c7a5494e624c45992536c5d32d1c01`。

本次使用 `upgrade v1.2.16 --confirm UPGRADE --immediate`，跳过自然空闲和请求排空等待，保留备份、隔离恢复验证和失败回滚。PostgreSQL、Redis 的容器 ID、镜像、启动时间和健康状态与切换前一致；没有重启数据服务。

### 生产验收

- 使用测试 Key 请求 `/v1/models`：原 HTTP 409 已恢复为 HTTP 200，返回 14 个模型，包括 `XXsedanco2.5S`、`grok-imagine-video`、`grok-imagine-video-1.5`。
- `code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 的健康和公共设置接口均为 HTTP 200、版本 `1.2.16`；`/docs/imagevideo` 正常。
- 使用真实管理员浏览器会话验收：版本、媒体管理侧栏、五个页面分区和 `account_id=2139` 深链接均正确，模型广场正常加载。
- 账号 2117 一键拉取发现 40 个模型，新增 34 个待绑定项，合并后共 41 个，启用数量保持 7 个；原有七个视频模型显示渠道定价匹配。验收草稿没有保存，已刷新清理。
- 409 根因是账号 2093、数据库分组 66 的历史 `image → image.url` 素材映射被通用重命名校验拒绝；已增加兼容并隔离无效绑定及分组目录错误。界面错误中的标识 19 与数据库分组 ID 不是同一层标识。

### 验证范围与剩余边界

本地完整前端测试通过 375 个文件、2857 项用例，lint、typecheck 和生产构建通过；后端 `go test -tags unit ./...`、`go vet -tags unit ./...` 通过。全新隔离数据库的媒体与迁移测试 336 项全部通过。Linux/amd64 govulncheck 未发现可达漏洞，另有两个未调用模块漏洞。

GitHub CI **未全绿**：集成任务缺少 ffmpeg，golangci-lint 报告 17 个既有问题；发布任务因正式 Release 已存在而拒绝重复发布，没有覆盖已部署镜像或版本标签。上述远端失败与本地通过记录均保留。

本轮没有新增图片、视频、音频生成测试，不将模型列表和管理界面验收视为全部媒体生成或计费验收。`TTseadanco2.5-720` 未出现在本次 Key 的模型列表中，未标记为通过；历史未知任务仍须先核对再决定是否补测。

脱敏摘要见 [production-v1.2.16-evidence.json](production-v1.2.16-evidence.json)。本次记录为发布后的文档同步，不修改 `v1.2.16` 标签及已发布产物。

## 历史生产：v1.1.2

北京时间 **2026-10-13 02:29** 已完成 v1.1.2 切换与验收。媒体平台 `imagevideo` 融入现有账号、分组和渠道模型定价；用户使用模型广场和使用记录，旧 `/admin/media` 跳转到渠道管理。价格手动配置，不同步供应商价格、不建立来源或模板。配置步骤见 [v1.1.2 配置说明](../MEDIA_PLATFORM_V112_CN.md)。

- 应用提交：`ecf8332560d49b59a881a6a28b66b2b38b63ba38`，基线仍为官方 v0.2.15。
- 不可变镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:fa60314a66d84281c50cbc57e0d9cb5d1b2b57ca9471e4019e58702fdf623788`。
- [私有发布](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.1.2)、[发布验证与镜像构建](https://github.com/ccfwwm/sub2api-private/actions/runs/38283872233)、[离线导出](https://github.com/ccfwwm/sub2api-private/actions/runs/38288365384)。直接发布已验证候选的清单与 digest，未重新构建镜像。
- 部署工具修复提交：`14f73edf0500547038b7c95a8bdad2ccbfe7a97e`；管理包取自该提交的 canonical Git 文件，服务器工具版本 `85d114fc794af996`，30 项管理测试通过。应用镜像及标签身份不变。

| 操作 | 任务 ID | 备份 ID | 状态 |
|---|---|---|---|
| 副本迁移演练 | `40cf4bc14c1d4eebb85843c85047fb04` | `f943948c885d46c7b5898c9c6dc9e09d` | succeeded |
| 新增字段 apply | `b98f8f6a38f344cca33ae58a92eb3446` | `62205baa88f548f3b1ccca441c2a57f9` | succeeded |
| prepare | `192f2b28c0a34883a00eccc1dc5eeeb6` | `9d86a4bd18784dcba40f1419ef71cabd` | succeeded |
| upgrade | `bf528689fa3f498cb75dde487cfaa27e` | `2b926fb6ca764b33bcef4179e7cc6d08` | succeeded |

只执行 244/245 的两个可空 JSONB 加列，旧迁移及校验值不变。副本验证恢复指纹一致、新旧程序可启动、新增字段不改变原有业务数据；成功操作的四份备份完整性校验通过，备份留在服务器。按用户已授权的即时方式切换，当时有 3 个在途请求；旧应用关闭耗时接近 120 秒退出上限，期间有连接中断/维护响应。普通脚本仍保留自然空闲/排空门禁。

最终核验：首页、health、版本接口 HTTP 200，公共版本 `1.1.2`；未认证模型及媒体管理 API 为 401，Nginx 配置通过。应用挂载、网络及 `127.0.0.1:8081` 映射未变；PostgreSQL、Redis、定价覆盖服务、codebuddy 的容器 ID、启动时间、完整挂载和网络均未变。维护标记不存在，`draining:false`。新前端包含媒体平台及旧媒体地址跳转。媒体账号/分组/售价/成本配置均为 0，八个媒体表为空；没有在生产执行媒体生成或导入本地测试配置。

两次初始迁移演练因 SQL 注释与严格事务格式门禁不一致而停止，均未修改生产数据库。修复忽略整行注释并生成固定的允许语句，保留原始文件 checksum，新增测试读取实际 244/245 文件验证事务往返；失败记录保留。提前执行的前端检查已在真实 v1.1.2 上重新验证，不采用旧版本页面作为发布证据。

创建 Git 标签另外触发了 [重复发布运行](https://github.com/ccfwwm/sub2api-private/actions/runs/38288299157)：其测试与演练通过，最终因 Release 已存在而拒绝重复发布，但版本镜像标签被额外构建更新。生产一直使用正式清单中的 image ID/digest，没有再次切换。[标签修复运行](https://github.com/ccfwwm/sub2api-private/actions/runs/38292178093) 比较成功候选与正式发布清单，恢复 registry 的 `v1.1.2` 标签并核验其 digest 为 `fa60314…`。另增加源码测试结束、镜像打包开始前的发布存在性检查；已发布或 API 状态无法确定时拒绝覆盖版本标签。

本地及发布 CI 验证见 [VALIDATION.md](VALIDATION.md)。真实渠道累计费用 **5.540890 USD**，见 [REAL_CHANNEL_VALIDATION.md](REAL_CHANNEL_VALIDATION.md)；当前测试 Key 无权调用 `00sd2.5`、`LLseadanco2.5`，不标为实测通过。

脱敏证据：[production-v1.1.2-evidence.json](production-v1.1.2-evidence.json)。服务器原件为 `/root/sub2api-ops/state/releases/v1.1.2/production-rollout-evidence.json`，整体记录为 `/root/sub2api-ops/state/rollout-media-v1.1.2.json`。本次迁移和升级已完成，不重复执行 apply；后续使用 `/root/sub2api-manage status`。

## 历史生产：v1.1.1

生产已于北京时间 2026-10-12 23:06 切换到私有多模态版本 **v1.1.1**，23:11 完成独立验收。应用基线为官方 v0.2.15。

## 发布身份与验收

- 应用提交：`b8f1ddf4f45670e7cf70ed906930c17ca91137e9`。
- 不可变镜像：`ghcr.io/ccfwwm/sub2api-private@sha256:85b53db49022fa76677f1f629c2084d4e88d26a4bf3a62b533b59327f5870193`。
- 镜像 ID：`sha256:2b9a70ec07e546709736ead444160d9a6bac285e171e52d42102330835bb72ec`。
- [私有发布 v1.1.1](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.1.1)，标签与镜像保持不可变。
- GitHub Actions：[完整 CI](https://github.com/ccfwwm/sub2api-private/actions/runs/38260190147)、[安全扫描](https://github.com/ccfwwm/sub2api-private/actions/runs/38260190402)、[正式发布](https://github.com/ccfwwm/sub2api-private/actions/runs/38261350808)、[离线镜像导出](https://github.com/ccfwwm/sub2api-private/actions/runs/38265441617) 均成功。发布包含镜像启动、隔离恢复与受控切换演练。
- 本地前后端、账务与真实渠道结果见 [VALIDATION.md](VALIDATION.md) 和 [REAL_CHANNEL_VALIDATION.md](REAL_CHANNEL_VALIDATION.md)。14 个模型实测通过，累计供应商测试费用为 5.178424 USD。
- 管理工具修复提交：`0b8639ef2751069e49cb7260c6ba541676e8b08e`；本地管理/迁移工具测试共 28 项通过。服务器已安装工具版本 `ed839bca85a6bc69`。

## 生产执行结果

[生产站点](https://code.aicodeme.xyz) 公共版本为 `1.1.1`，应用容器 healthy，维护标记已解除，`draining:false`。新管理入口为 [媒体配置](https://code.aicodeme.xyz/admin/media)，使用现有管理员账号登录。

| 操作 | 任务 ID | 备份 ID | 最终状态 |
|---|---|---|---|
| 生产备份副本迁移演练 | `8ba199bc35474569a85463d4cfa7ab65` | `2656719d08494732b4d280b95b3ba254` | succeeded |
| 加法迁移 apply | `4340344be6494fccac67f4582eab7294` | `7e3cba1f0f7540cf9e3f9ee8edeb4acb` | succeeded |
| prepare | `cf0f2d5b82644c449a8557a968d18d09` | `2ea9f0a172364b269a7a5f0b147834aa` | succeeded |
| upgrade | `497d463109ce4e08bb6d62667be9b613` | `762b404e54f541859f831c1614e43c09` | succeeded |

副本演练证明恢复指纹一致、旧实际程序可启动、新迁移不改变核心业务数据。四份备份的完整性校验通过，保留在服务器本地，不上传 GitHub。

## 即时切换与验收修正

用户明确要求“不要等待，直接切换”。原在线等待任务 `4da57db0a663404aaf23dc454d32575b` 已停止并保留 `UserRequestedImmediateCutover` 记录。本次 root 进程只对本次调用跳过空闲/排空等待，保留备份、sealed receipt、镜像身份、迁移校验和回退检查；允许切换时中断在途请求。正常管理工具仍采用最多 15 分钟在线等待及 120 秒排空门禁。

即时切换 worker 完成迁移、prepare 和 upgrade 后，最终验收因直接比较 Docker 的 `Mounts` 数组而失败。连续 inspect 证明数组顺序变化、所有挂载字段一致；验收脚本改为按唯一 Destination 排序后比较完整字段，独立重验全部通过，没有再次重启或重做迁移。原 systemd worker 的失败结果保留，整体发布记录保存失败历史及重验成功证据后标记 succeeded。

原 v1.1.0 候选在生产 SQL 执行前因安全检查取消。v1.1.1 已升级 `golang.org/x/image` 到 v0.45.0，加入 govulncheck 和前端生产依赖安全门禁，并重新完成生产副本迁移演练。

## 数据与配置边界

已执行经校验的 `243_media_tasks_and_pricing.sql` 加法迁移，增加新媒体表和旧额度表字段。事务使用 3 秒锁等待、60 秒语句超时；旧迁移及校验值保持一致。没有删除、重算或以备份覆盖历史业务数据。

切换只替换应用容器。PostgreSQL、Redis、定价覆盖服务、codebuddy 容器的 ID、启动时间、完整挂载和网络均与切换前一致；应用数据挂载、网络和 `127.0.0.1:8081` 端口保持一致。

38 个媒体能力模板默认关闭，生产不导入 staging 凭证、测试价卡或测试任务。新接口上线不表示每个模型已可调用；未实测场景、缺正式售价、缺成本配置或缺有限预留上限的配置需在后台完成验证后发布。成本未配置保持未知。

## 最终验收与证据

| 核验 | 结果 |
|---|---|
| 镜像 ID、发布 commit、版本、迁移清单 | 与 v1.1.1 发布匹配 |
| 首页、health、公共设置 | HTTP 200 |
| 未认证模型 API、媒体管理 API | HTTP 401 |
| Nginx 配置 | `nginx -t` 通过 |
| 应用挂载、网络、端口；相邻容器身份与挂载 | 与基线一致 |
| 八个新增媒体表 | 均为空，未导入测试配置或任务 |
| 维护状态 | 标记不存在，未排空 |

脱敏证据：[production-v1.1.1-evidence.json](production-v1.1.1-evidence.json)。服务器原件为 `/root/sub2api-ops/state/releases/v1.1.1/production-rollout-evidence.json`，整体发布记录为 `/root/sub2api-ops/state/rollout-media-v1.1.1.json`。

后续查看状态使用 `/root/sub2api-manage status`。本次迁移已完成，不重复执行 apply；未来只改前端/业务代码且迁移清单不变时，继续使用 prepare → upgrade。出现新媒体在途任务后，回滚前须停止新提交并处理任务，不能切回无法查询和结算它们的旧程序。

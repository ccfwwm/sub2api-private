# Sub2API 服务器管理脚本

服务器入口：`/root/sub2api-manage`。脚本通过 systemd 启动后台任务，SSH 断开后继续执行。所有写操作有全局互斥锁；重复操作会被拒绝。启动命令返回 JOB_ID 只表示任务已提交，必须检查最终状态。

## 查看任务和日志

```sh
/root/sub2api-manage status
/root/sub2api-manage logs
/root/sub2api-manage logs <任务ID>
/root/sub2api-manage list
```

`logs` 使用 tail，Ctrl-C 只退出查看，不会停止任务。任务状态和日志分别保存在 `/root/sub2api-ops/state/jobs`、`logs`，均仅 root 可读。`backup_id` 是备份 ID，和任务 ID 不同。

## 备份、验证、升级和回滚

```sh
# 在线备份：增加 CPU、磁盘和 Redis 快照负载，不重启业务容器
/root/sub2api-manage backup --confirm BACKUP
# 用备份实际恢复临时数据库，核对核心表指纹并验证程序启动
/root/sub2api-manage verify <备份ID> --confirm VERIFY
# 仅做升级前准备：备份并验证指定新镜像，不排空请求、不切换容器
/root/sub2api-manage prepare v0.2.15 --confirm PREPARE
# 升级到已经预置且注册了发布清单的私有镜像
/root/sub2api-manage upgrade v0.2.15 --confirm UPGRADE
# 只回滚程序到某次备份捕获的实际运行版本，保留当前数据库
/root/sub2api-manage rollback <备份ID> --confirm ROLLBACK
```

升级和回滚会自动先备份、演练，再排空请求并短暂停服。首次官方版本没有排空接口，在途请求可能中断，应选低峰。普通升级要求迁移记录和镜像清单完全相同；有差异就停止。切换失败会自动尝试恢复切换前程序和配置，绝不自动覆盖数据库。

## 恢复业务数据

```sh
/root/sub2api-manage restore <备份ID> --confirm RESTORE
```

先备份当前状态，再将指定备份恢复到**新的 PostgreSQL、Redis 容器和应用数据目录**。验证快照指纹和程序健康后，切换应用到恢复出的数据。原数据库、Redis 和目录不删除、不重建。恢复后，应用看到的是备份时的数据，备份之后的业务变化不会出现在恢复库中，仍保留在原库和恢复前备份内。恢复库只在 Docker 内部网络暴露，不公开端口。

备份包含 PostgreSQL 全库（仅排除 `public.ops_system_logs` 数据行，保留结构）、角色信息、Redis RDB、应用数据文件、Compose/.env/定价配置、当前容器运行镜像及校验和。数据库使用一致性快照；应用文件和 Redis 是在线抓取，不是跨存储原子快照。角色与 Compose/定价配置作为灾难恢复材料保存，普通恢复不重写宿主机角色、Compose 或定价服务。

恢复后的 PG/Redis 使用独立目录并设置自动启动。后续备份自动识别正在使用的恢复库。原有直接连接数据库的其他服务不会随应用恢复自动改地址；需要人工评估再调整。不要全局清理 Docker 镜像、网络、卷和 recovery 容器。

## 断电或异常后的应急切回

SSH 断开不会中断任务；服务器重启或进程被强制终止则可能中断。先查看状态与日志，再执行：

```sh
/root/sub2api-manage rescue <最近一次切换任务ID> --confirm RESCUE
```

rescue 不依赖当前应用健康，只允许最近一次已开始的切换任务，恢复该任务保存的切换前镜像和应用配置，并解除维护标记。它不恢复覆盖数据库，也不自动继续中断任务。状态 `failed_needs_inspection` 表示需要查日志，不能视为上线成功。

支持排空接口的当前程序先在线等待自然空闲，最多 15 分钟，期间不创建维护标记、不拒绝新请求。日志每 30 秒记录 `WAITING_FOR_IDLE`，任务状态为 `waiting_for_idle`。一直没有空闲则取消，生产继续开放。发现空闲后才进入维护，仍须通过在途数量归零门禁；此阶段最多等待 120 秒，每 15 秒记录 `DRAIN_STATUS`。日志仅含在途数量与维护状态。数量未归零、探测失败或中断会取消操作并立即解除本次维护，不强制断开业务连接。已有维护标记或程序已经排空时拒绝开始，需先检查原操作。专用媒体迁移也持久记录 `queued / backing_up / restoring_and_testing / waiting_for_idle / draining / applying_reviewed_additive_migration / succeeded`，并记录真实 systemd worker unit；失败记录 `failed_needs_inspection`、`failed_phase` 和异常类型。退出后仍显示旧的“备份中”不是成功证据，需结合后台 unit、日志与实际迁移清单检查，禁止直接重做 SQL 或恢复旧库。

## 维护边界

应用数据归档排除持续写入的 `data/logs` 运行日志；这些不是业务恢复输入。其余配置、业务文件均保留。数据库仅排除已约定的 `ops_system_logs` 数据行。

- 只管理 Sub2API 应用和本工具创建的恢复环境，不重启原 PostgreSQL、Redis、定价服务，不执行 Compose down、删卷或全局 prune。
- 运行镜像、备份、日志可能含密钥，禁止上传 GitHub或发送给他人。备份和应急镜像不自动删除。
- 当前镜像/恢复库覆盖配置保存在 `/root/sub2api-ops/state/active.json`。以后使用 Compose 维护时必须最后叠加该文件，否则可能切回旧官方镜像和原数据库。日常操作请统一使用此脚本。
- 此脚本与 `/opt/sub2api-deployer` Web 控制器互斥，避免两套部署入口同时修改应用。当前以命令行脚本为生产入口，后台镜像面板尚未连接控制器。
- 后续版本须先发布、下载并验证镜像，把对应的 `deployment.json` 和 `image-inspect.json` 放在 `/root/sub2api-ops/state/releases/<版本>/`，再调用 upgrade。脚本拒绝未登记的版本，当前已备好的版本无需联网下载。


## 私有多模态版本 v1.1.1 的新增迁移

普通 prepare 仍要求生产迁移清单和镜像一致，不允许隐式迁移。新增 `243_media_tasks_and_pricing.sql` 仅经专用流程执行：

v1.1.1 已于 2026-10-12 完成生产迁移和切换，见 [生产发布记录](../../docs/multimodal/PRODUCTION_ROLLOUT.md)。以下为首次迁移流程参考，已迁移的生产不要重复执行 rehearse/apply。

```sh
/root/sub2api-media-migrate rehearse v1.1.1 --confirm REHEARSE
/root/sub2api-media-migrate apply v1.1.1 --confirm APPLY
/root/sub2api-manage prepare v1.1.1 --confirm PREPARE
/root/sub2api-manage upgrade v1.1.1 --confirm UPGRADE
```

rehearse 做完整备份与隔离恢复，不改变生产 schema；必须验证核心业务数据指纹、新镜像和捕获的旧实际程序都可启动。apply 需要对应 sealed receipt、同一镜像/SQL/config/runtime，并做新的最终备份后短暂排空请求。243 仅增加媒体表、额度字段和索引，不覆盖余额/用户/Key/原价格。普通 prepare 完成后再 upgrade，仅替换应用容器。

新增媒体模板默认关闭，测试账号、测试价卡及测试法律 fixture 不进入生产。生产价卡由管理员填写。v1.2.0 起模型可直接启用，真实测试仅作状态记录。以后若已存在媒体在途任务，不可回滚到不会继续查询和结算它们的旧版本；先停止新提交并处理在途任务。紧急恢复程序不会用旧数据库备份覆盖最新业务。

## v1.1.2 统一媒体价格迁移

2026-10-13 已完成 `244_unified_media_model_pricing.sql` 与 `245_unified_media_account_cost.sql` 的备份副本演练和正式加列，并完成 prepare/upgrade。两个迁移只新增可空 `media JSONB` 字段，原数据不变。专用迁移选择器为 `--schema unified-pricing`；已迁移生产不再重复 rehearse/apply，普通 prepare 仍要求完整清单相等。

服务器管理代码为 `85d114fc794af996`，来源 `14f73edf0500547038b7c95a8bdad2ccbfe7a97e`。修复 SQL 整行注释导致的事务白名单误拒，保留原始文件校验值并生成两条固定允许语句；实际迁移文件回归与 30 项工具测试通过。历史失败任务保留，不代表最终升级失败；成功任务与备份 ID 见 [发布记录](../../docs/multimodal/PRODUCTION_ROLLOUT.md)。

媒体配置改用原有账号、分组和渠道模型定价，价格手动设置，不导入来源或模板。查看状态：`/root/sub2api-manage status`；配置步骤见 [v1.1.2 说明](../../docs/MEDIA_PLATFORM_V112_CN.md)。
# 正式 Release 与独立部署（v1.2.15 起）

发布工作流只构建和发布制品，不调用生产部署。管理员独立手动触发 GitHub 的 `deploy-production.yml`，通过专用受限 SSH Key 调用
`/root/sub2api-auto-deploy`。固定主机公钥，只允许 `deploy vMAJOR.MINOR.PATCH`
和 `status JOB_ID`；不允许任意 shell 命令。

只有发布后的正式 Release 才能部署。宿主机校验清单、校验和、源码版本、
本次真实渠道验证记录以及固定 digest 镜像，复用管理器的真实备份、隔离恢复演练、
迁移检查和回滚。普通提交、候选镜像和预发布不会切换生产。

独立部署工作流和手动 `upgrade` 都默认等待请求排空。
其他维护操作仍允许显式 `--immediate`；本次模板配置迁移拒绝该模式。
两种模式都会核对 PostgreSQL、Redis 和定价服务容器未被替换。
内外网健康、版本、镜像最终核验失败会回滚应用，并向 Actions 返回失败。

任务、日志和清单在 `/root/sub2api-ops/state/automatic` 持久保存；
GitHub 必须查询到最终 `succeeded` 才报告上线成功。
重复版本复核当前镜像与健康，旧版本不能覆盖新版本。

## v1.2.15 媒体模板与旧配置迁移

Release 必须包含 `249_media_protocol_templates.sql`、`250_migrate_legacy_media_template_refs.sql` 及校验值。先安装同一提交的管理代码，再登记本次精确镜像与清单。使用 `prepare v1.2.15 --confirm PREPARE` 完成备份副本演练，通过后才执行 `upgrade v1.2.15 --confirm UPGRADE`；不另行手工执行 SQL。

249 为固定配置类型扩展和制品文件名加列；250 会在账号 `extra.media_models` 保存等价模板引用或待配置状态。门禁以固定受审 SQL 正文摘要限制变换，不接受任意 UPDATE。隔离副本中的完整账号行必须与预先计算的批准变换结果相等，其他业务表、历史任务快照、账务、制品旧列和配置版本必须保持原值；再次变换应幂等。候选应用及捕获的真实旧程序均需在迁移后的副本启动通过。

正式升级先备份并演练，再等待闲置和请求排空；249/250 的事务在排空成功后、应用容器切换前执行。排空失败时不修改生产配置。回退使用本次捕获的旧镜像与原配置，不覆盖生产数据库；普通 rollback 仍拒绝未经证明的迁移清单差异。

## v1.2.4 媒体任务分页索引

`247_media_task_list_indexes.sql` 仅增加 `media_tasks` 的全局及用户时间/任务 ID 索引。管理器只接受这两条固定 CREATE INDEX、对应的原始 checksum 和唯一新增迁移；其他 schema 差异仍拒绝。正式 Release 包含该 SQL 及校验值，旧 worker 下载后新管理器可从同一 Release 的已验证 assets 读取。

生产切换前先完整备份、恢复隔离副本，比较原业务、媒体任务、账务及制品表指纹，再验证新旧实际程序能启动；通过后才用 3 秒锁等待和 60 秒语句超时的事务创建生产索引并登记迁移。失败不覆盖数据库，也不删旧表或账务记录。

## 文档草稿的同源预览

文档专用 `/docs/` location 使用 `X-Frame-Options: SAMEORIGIN` 和 `frame-ancestors 'self'`，使后台可在 iframe 中显示未发布草稿；其他路径继续继承原有禁止嵌入策略。location 同时显式保留 HSTS 和 nosniff。管理器只接受原始片段或已审阅的新片段，未知配置仍拒绝覆盖；Nginx 校验或平滑重载失败时恢复原文件。更新该配置不重启 API、文档或数据容器。

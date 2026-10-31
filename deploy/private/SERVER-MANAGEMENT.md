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
/root/sub2api-manage prepare v1.3.20 --confirm PREPARE
# 升级到已经预置且注册了发布清单的私有镜像
/root/sub2api-manage upgrade v1.3.20 --confirm UPGRADE
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
- 当前镜像/恢复库覆盖配置保存在 `/root/sub2api-ops/state/active.json`。以后使用 Compose 维护时必须在基础文件和可选本地 override 之后叠加该文件，否则可能切回旧官方镜像和原数据库；若已启用容量策略，再将 `/root/sub2api-ops/state/capacity-compose.yml` 叠加在其后。日常操作请统一使用此脚本。
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

## v1.3.2 媒体分组可用状态

正式 Release 应包含 `251_media_group_availability.sql` 及 checksum。管理器仅接受在 `groups` 增加 `media_availability VARCHAR(20) NOT NULL DEFAULT 'available'` 并限制为 `available` / `unavailable` 的固定语句；迁移清单必须只有该项新增，SQL 在锁超时与语句超时保护下原子执行。旧 worker 只会从匹配版本、匹配迁移清单的已校验 Release assets 取回迁移文件。先完成备份副本演练，再按 `prepare v1.3.2 --confirm PREPARE` 和 `upgrade v1.3.2 --confirm UPGRADE` 执行，不要手工运行 SQL。

## 单机容量基线与扩容

本仓库提供容量采集器和 Compose 资源覆盖层；它们不包含机器规格、连接池或数据库参数的预设值。Compose 基础文件的默认池值只是开发/通用默认值，不能作为线上容量证据。维持单台宿主机、一个应用实例、一个 PostgreSQL 和一个 Redis；本流程不启动第二副本，也不引入外部数据库。

安装包含容量工具的正式管理包后，在 Linux 宿主机以 root 运行七天采样。每分钟记录宿主机 CPU、内存、文件系统和块设备 I/O、应用健康探针延迟、各容器 CPU/内存/重启/OOM 状态、PostgreSQL 连接及等待、运行超过 1 秒的查询、可选 `pg_stat_statements` 慢查询指纹，以及 Redis 内存、客户端、拒绝连接、淘汰键和探针延迟。采集结果是 JSONL，不保存 SQL 正文、环境变量或凭据；输出文件应由 root 私有保存。

```sh
umask 077
/root/sub2api-capacity-collect \
  --output /root/sub2api-ops/capacity-baseline.jsonl \
  --interval-seconds 60 \
  --duration-seconds 604800 \
  --health-url 'https://<现有公网域名>/health' \
  --data-path / \
  --data-path /var/lib/docker
/root/sub2api-capacity-report /root/sub2api-ops/capacity-baseline.jsonl \
  > /root/sub2api-ops/capacity-baseline-report.json
```

将健康探针 URL 指向现有公网入口可把反向代理和公网路径纳入采样；若只能使用 `http://127.0.0.1:8080/health`，报告仅代表本机应用入口。`--data-path` 应覆盖实际 Docker 数据目录和应用数据目录。报告只有在观测跨度达到七天、采样间隔没有超过两倍的缺口且采集器无错误时才开放容量评审门；可通过 `--expected-interval-seconds` 调整报告检查间隔。若 PostgreSQL 未预先启用 `pg_stat_statements`，报告仍包含长时间运行查询采样，并标明扩展统计不可用；采集器不会启用或修改扩展。

容量决策按下列次序完成：

1. 按业务高峰对齐应用延迟、CPU/内存、宿主机 I/O wait 与设备忙碌时间、数据库连接等待/慢查询、Redis 延迟/内存及 OOM/重启。先定位高延迟发生时的饱和资源，不以平均值或单次尖峰定机器规格。
2. 用观测到的峰值构造至少 30 分钟、达到观测峰值 1.5 倍的模拟上游负载。记录与基线同口径的 p50/p95/p99 延迟、错误率、数据库锁和连接等待、Redis 延迟、容器内存和 OOM；未达 SLO 或仍有 OOM 时不进入上线。
3. 先增加宿主机 CPU、内存或磁盘 IOPS/容量中被证实不足的资源，再依据 1.5 倍负载下的容器工作集设置资源 reservation/limit。应用、PostgreSQL、Redis 与系统服务合计至少保留宿主机 20% 的 CPU 和内存余量；磁盘容量另须覆盖数据库、媒体文件、日志及备份增长。
4. 按单个应用实例的 `DATABASE_MAX_OPEN_CONNS` 总预算设置 PostgreSQL `max_connections`，另外扣除迁移、管理及其他数据库客户端的连接预留。不要为了消除连接等待盲目扩大连接池；先检查查询耗时、锁等待和连接占用。结合实测内存预算选择 `shared_buffers`、`effective_cache_size` 和 `maintenance_work_mem`，确保 PostgreSQL 后台、并发查询和系统缓存同时落在容器/宿主机内存预算内。
5. 按该应用实例的 `REDIS_POOL_SIZE`、运维探针及其他客户端总数协调 Redis `maxclients`。根据 key 的持久性和业务语义确定 `maxmemory-policy`；资源覆盖要求显式设置策略和 `maxmemory`，不会替用户选择。Redis `maxmemory` 必须低于容器内存限制并给复制/AOF 缓冲和进程开销留出余量。核实淘汰、拒绝写入和 AOF/RDB 持久化状态。

扩容前执行 `backup` 和 `verify <备份ID>`，再在隔离恢复副本上验证本次数据库/Redis 备份及关键业务数据。取得实测值并通过 1.5 倍负载门禁后，将 `CAPACITY_*` 值追加到 `/root/sub2api-deploy/.env`，保留原有变量并维持 root-only 权限；不要用容量文件替换现有 `.env`。资源限制模板位于发布管理包中的 `docker-compose.capacity.yml`。将它复制为 `/root/sub2api-ops/state/capacity-compose.yml`，权限设为 `0600`。管理器检测到该文件后，会在每次 Compose 调用中将它追加到基础文件、可选 `docker-compose.override.yml` 和当前 `/root/sub2api-ops/state/active.json` 之后，使容量策略在后续应用升级时继续生效。

应用容量配置前，用生产项目目录与 `.env` 检查最终合成文件，顺序必须是基础 Compose、可选本地 override、`active.json`、`capacity-compose.yml`；这样活动镜像、恢复库连接和应用配置先被恢复，再由容量策略覆盖对应资源与池参数。验证时用 `docker compose ... config --quiet`，避免将展开后的密钥打印到终端或日志。模板中的所有 `CAPACITY_*` 参数均为必填，Redis `maxclients` 必须与应用 `REDIS_POOL_SIZE` 及额外客户端总数协调；不得直接套用 `.env.example` 或本地示例中的高连接数。

完成七天容量评审、1.5 倍负载测试、备份和隔离恢复后，确认没有部署管理任务正在运行，并在低峰时段停止新流量、等待在途请求排空。下面的命令先验证合成配置，再重建应用、PostgreSQL 和 Redis 容器以应用资源限制及参数；数据库和 Redis 会短暂断连，不要在门禁完成前执行：

```bash
cd /root/sub2api-deploy
compose=(docker compose --project-name sub2api-deploy --project-directory /root/sub2api-deploy --env-file /root/sub2api-deploy/.env -f /root/sub2api-deploy/docker-compose.yml)
if [ -f /root/sub2api-deploy/docker-compose.override.yml ]; then
  compose+=(-f /root/sub2api-deploy/docker-compose.override.yml)
fi
compose+=(-f /root/sub2api-ops/state/active.json -f /root/sub2api-ops/state/capacity-compose.yml)
"${compose[@]}" config --quiet
"${compose[@]}" up -d --no-deps --pull never sub2api postgres redis
```

PostgreSQL 参数、Redis 启动命令或容器资源限制变化会重建相应容器并造成短时连接中断。上线时选低峰，先停止新流量并确认在途请求已排空；一次只应用经恢复验证的容量配置，随后检查 Compose 最终配置、应用/数据库/Redis 健康、PostgreSQL 与 Redis 持久化、外部 API 关键业务请求和账务/媒体任务一致性。对比同一负载和时间窗口下的延迟、错误率、连接等待、I/O 与内存。失败时按变更前记录恢复容器参数和资源配置，不恢复旧业务数据库覆盖新写入。容量观测、隔离恢复和 1.5 倍负载门禁完成前，不进行生产资源调整。

## 媒体账号并发诊断字段

实现账号级媒体并发后，正式 Release 还应包含 `252_media_account_concurrency_diagnostics.sql` 及 checksum。该迁移只向 `ops_error_logs` 增加账号并发、在途、排队和人工核对占用的可空快照列，不修改业务数据；管理器仅接受固定 `ALTER TABLE` 与列注释语句，并要求先完成隔离副本演练，再按同一 Release 的 `prepare` / `upgrade` 流程应用。

## v1.2.4 媒体任务分页索引

`247_media_task_list_indexes.sql` 仅增加 `media_tasks` 的全局及用户时间/任务 ID 索引。管理器只接受这两条固定 CREATE INDEX、对应的原始 checksum 和唯一新增迁移；其他 schema 差异仍拒绝。正式 Release 包含该 SQL 及校验值，旧 worker 下载后新管理器可从同一 Release 的已验证 assets 读取。

生产切换前先完整备份、恢复隔离副本，比较原业务、媒体任务、账务及制品表指纹，再验证新旧实际程序能启动；通过后才用 3 秒锁等待和 60 秒语句超时的事务创建生产索引并登记迁移。失败不覆盖数据库，也不删旧表或账务记录。

## 文档草稿的同源预览

文档专用 `/docs/` location 使用 `X-Frame-Options: SAMEORIGIN` 和 `frame-ancestors 'self'`，使后台可在 iframe 中显示未发布草稿；其他路径继续继承原有禁止嵌入策略。location 同时显式保留 HSTS 和 nosniff。管理器只接受原始片段或已审阅的新片段，未知配置仍拒绝覆盖；Nginx 校验或平滑重载失败时恢复原文件。更新该配置不重启 API、文档或数据容器。


## v1.3.0 公开能力与立即发布

本版本没有新增数据库迁移。先从公开正式 Release 下载、校验并登记清单、管理工具和两个固定摘要镜像，安装同一提交的管理代码。用户已要求本次立即切换：

```sh
/root/sub2api-manage upgrade v1.3.0 --immediate --confirm UPGRADE
```

该模式仍执行完整备份、隔离恢复与新旧程序验证，只跳过空闲等待和排空；在途连接可能短暂中断。必须检查任务最终 `succeeded`、应用及文档摘要、三个入口的版本与健康，并核对原 PostgreSQL/Redis 容器标识和启动时间。失败按该任务捕获的程序和配置恢复，不覆盖业务数据库。

管理脚本识别 v1.2.19 的现有文档代理片段，新增用户播放路由并关闭该路由访问日志，避免短期票据进入 Nginx 日志；保留原管理员媒体读取路由与同源文档嵌入头。

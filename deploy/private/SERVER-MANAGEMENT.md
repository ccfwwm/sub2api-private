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

## 维护边界

应用数据归档排除持续写入的 `data/logs` 运行日志；这些不是业务恢复输入。其余配置、业务文件均保留。数据库仅排除已约定的 `ops_system_logs` 数据行。

- 只管理 Sub2API 应用和本工具创建的恢复环境，不重启原 PostgreSQL、Redis、定价服务，不执行 Compose down、删卷或全局 prune。
- 运行镜像、备份、日志可能含密钥，禁止上传 GitHub或发送给他人。备份和应急镜像不自动删除。
- 当前镜像/恢复库覆盖配置保存在 `/root/sub2api-ops/state/active.json`。以后使用 Compose 维护时必须最后叠加该文件，否则可能切回旧官方镜像和原数据库。日常操作请统一使用此脚本。
- 此脚本与 `/opt/sub2api-deployer` Web 控制器互斥，避免两套部署入口同时修改应用。当前以命令行脚本为生产入口，后台镜像面板尚未连接控制器。
- 后续版本须先发布、下载并验证镜像，把对应的 `deployment.json` 和 `image-inspect.json` 放在 `/root/sub2api-ops/state/releases/<版本>/`，再调用 upgrade。脚本拒绝未登记的版本，当前已备好的版本无需联网下载。


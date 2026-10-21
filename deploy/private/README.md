# Sub2API 私有版本发布与生产交接

源码仓库：`ccfwwm/sub2api-private`（私有）。生产镜像：`ghcr.io/ccfwwm/sub2api-private`（私有）。
上游同步版本 `v0.2.15` 保持已有发布；本次私有多模态功能版本为 `v1.1.0`，上游基线仍为 `v0.2.15`。旧 `v1.0.0`–`v1.0.6` 保留历史恢复用途。上游基线及私有版本说明记录在 `deploy/private-release.json`。

## 发布不等于部署

推送 main 只运行 CI。推送正式标签或手动运行 **Private Release** 时，使用同一提交完成测试、镜像构建、隔离启动检查，再发布带有提交、digest 和迁移 SHA-256 清单的 `deployment.json`。发布流程不调用生产部署工作流；生产切换必须独立执行 **Manual Production Deployment** 或已授权的宿主机管理命令。
任何构建或发布流程都不连接生产。**Manual Production Deployment** 仅支持 `workflow_dispatch`，保留请求排空、备份、恢复演练和精确迁移门禁；不采用 `immediate` 切换。

### v1.2.15 的受审迁移

249 只扩展媒体配置类型约束并新增制品文件名列；250 会修改明确保存的旧媒体协议配置，不属于“所有数据不变”的纯加列迁移。250 根据已保存的 `request_format` / `adapter` 选择等价模板，保留有效已有模板引用；无法确认的模型保留数据并禁用，标记待配置。

管理器只接受精确的 249 DDL 和固定受审的 250 SQL 正文；Release 自报的新 checksum 不能放宽该授权。已应用迁移的 checksum 不得变化。隔离恢复库先计算完整账号行的预期指纹，再执行迁移，要求仅批准的 `extra.media_models` 变换发生，其他账号字段、其他受保护业务表、历史任务快照、账本、制品旧列和配置版本均保持原值。重复变换必须幂等，已应用的版本再次准备不会执行 SQL。

候选应用和捕获的真实旧运行时都必须在迁移后的隔离恢复库成功启动，才允许切换。新建备份保留迁移前数据及真实旧应用；失败时回退应用与原绑定配置，不覆盖生产数据库。普通 `rollback` 仍拒绝迁移清单不匹配，不能用它绕过恢复兼容证明；使用本次成功演练的 rescue 目标。

当前实施边界：仓库、代码和 GitHub 构建可以直接完成；以下操作必须先向用户说明影响并获得同意：

1. 宿主机安装控制器、配置凭据和受限 SSH 入口：修改系统配置，不重启业务容器。
2. `prepare`：读取生产数据库、制作本机应急镜像、备份文件和 Redis、运行隔离恢复。会占用 CPU、磁盘、网络，Redis 导出可能触发快照资源开销。
3. `deploy` / `rollback`：更换应用容器，存在短暂停服。第一次从官方程序切换时没有应用排空接口，需选择低峰窗口，不能承诺已有流式请求不断开。

## 经同意后的宿主机安装

将本目录从审核过的发布提交复制到服务器的独立暂存目录，然后运行：

```sh
sudo sh install.sh --confirm-host-install
```

安装只新增 `/opt/sub2api-deployer`、`/etc/sub2api-deployer`、`/var/lib/sub2api-deployer` 和 systemd 服务；默认 `enabled=false`。不修改 `/root/sub2api-deploy` 的现有 Compose/.env，不替换生产镜像。

配置独立凭据（不复制个人广权限 gh 登录）：

- `/etc/sub2api-deployer/github-token`：读取指定私有仓库 Release 的凭据，root:root 0600。
- Docker credential store：可读取私有 GHCR 包的专用凭据，通过 `docker login --password-stdin` 配置。GitHub 仓库访问权限与 Packages 权限分别验证。
- `/etc/sub2api-deployer/release-token`：供应用读取版本元数据的仓库只读凭据，root:1000 0440；只读挂载到应用。实际容器 GID 若有调整，修改 `socket_gid` 和文件组权限。

读取代码包和凭据都使用受限目录；禁止将 .env、数据库备份、当前容器快照或密钥上传到 GitHub。`docker commit --pause=false` 捕获的应急镜像可能含运行时环境机密，**只能留在服务器本机，禁止推送**。

Actions 使用独立 Unix 用户 `sub2api-deploy`，仅加入部署 socket 所属组，不能加入 docker/sudo 组。其专用 authorized_keys 条目使用：

```text
restrict,command="/usr/bin/python3 /opt/sub2api-deployer/controller.py ssh" ssh-ed25519 <专用公钥>
```

在 GitHub `production` Environment 设置 `DEPLOY_SSH_KEY`、`DEPLOY_KNOWN_HOSTS` Secrets，以及 `DEPLOY_HOST`、`DEPLOY_PORT`、`DEPLOY_USER` Variables。known_hosts 必须使用核验过的服务器指纹，不能在工作流中临时信任 ssh-keyscan。可在账户方案允许时增加 Environment required reviewer；无此功能也必须保留手动触发和控制器显式确认。

用户同意启用后，在 root 配置中设置 `enabled=true` 并重启**控制器服务**。不重启 Sub2API。新应用首次切换时才获得 socket 和只读 Release 凭据挂载。

## 两阶段操作

GitHub Actions 手动填写 `action=prepare`、`target=v1.0.0`、`confirmation=CONFIRM`。控制器异步返回 job ID，随后用 `action=status` 查询；“请求提交成功”不是“部署成功”。后台私有版本面板使用同样的机制。

准备阶段：

- 校验正式 Release 的 manifest、固定仓库、提交、架构和不可变镜像 digest。
- 要求生产迁移记录与发布清单完全一致。普通更新不执行新的迁移；存在增删或校验和变化时停止。
- 用运行容器创建本机应急镜像，核对实际二进制 SHA-256，避免基础镜像和已热更新程序版本不一致。
- PostgreSQL 导出一致性快照：排除 `public.ops_system_logs` 的数据，保留表结构和所有其他表；同一快照生成核心业务表全行指纹。
- 保存角色、应用文件、配置、定价覆盖及 Redis RDB，备份保存在 `/var/lib/sub2api-deployer/backups/<job>`。
- 在独立内部 Docker 网络和专属数据库目录实际恢复，比较用户、账户、API Key、分组、设置、订阅、支付、定价和密钥表的快照指纹；测试目标镜像和实际旧程序启动。
- 演练应用没有生产网络、数据挂载、公开端口或外部网络出口。克隆数据和业务凭据只在受限服务器目录使用；演练结束仅清理本次专属容器、网络和恢复目录，保留备份及应急镜像。

状态成为 `ready` 后，一小时内由用户再次确认：`action=deploy`、`target=<job ID>`、`confirmation=CONFIRM`。这一步才会影响线上应用。

切换阶段再次核对原容器、程序 SHA、Compose/.env 和迁移状态并做最新备份。私有应用支持阻止新请求、统计在途 HTTP/SSE/WebSocket handler，120 秒内未排空就取消切换。数据库和 Redis 容器不重启。

镜像覆盖文件保存在 `/var/lib/sub2api-deployer/active-image.json`，控制器以现有 Compose 文件叠加该覆盖文件，仅执行 `up -d --no-deps --pull never sub2api`。不执行 Compose down、删卷或数据库重建。

**以后不能直接使用旧 Compose 的官方 latest 镜像重建应用。** 手工检查或维护应用时，必须使用同样的 Compose 叠加参数；覆盖文件必须最后出现：

```sh
docker compose --project-name sub2api-deploy --project-directory /root/sub2api-deploy \
  --env-file /root/sub2api-deploy/.env \
  -f /root/sub2api-deploy/docker-compose.yml \
  -f /root/sub2api-deploy/docker-compose.override.yml \
  -f /var/lib/sub2api-deployer/active-image.json ps
```

失败时在确认数据库结构兼容的前提下恢复本次捕获的应急镜像；不恢复覆盖生产数据库。需要人工回滚时使用 `action=rollback` 和成功任务的 job ID。回滚也会备份、排空和短暂停服。

控制器重启后，不会自动继续未完成的切换，任务标记为 `interrupted`。管理员检查当前镜像、数据库迁移和健康状态后再处理；如遗留 `/run/sub2api-deployer/drain`，确认现有应用可继续服务后才能移除。不要将进程重启或工作流提交成功视为上线验收。

## 验证命令与边界

### 仅预置镜像（不切换应用）

`Export Validated Private Image` 工作流只接受已发布且带 `deployment.json` 的正式版本，按 digest 拉取镜像、核对源码提交并导出离线包。它不连接生产服务器。下载私有 Actions artifact 后，先核对 `offline-checksums.txt`，再传到服务器受限目录并重新校验，最后执行 `docker load`。同时核对导入镜像的源码标签、架构、全部 RootFS 层摘要和原始配置摘要。

经典 Docker 存储显示的 image ID 是配置摘要，containerd 存储可能显示 manifest 摘要，二者不能直接比较。若 ID 不同，必须对导入镜像执行只读 `docker image save`，确认导出配置 blob 的 SHA-256 与 CI 记录的原始配置摘要一致，并核对全部层摘要；不能只凭版本标签判断相同。

`docker load` 只预置镜像，不创建或重启容器，不更改数据挂载。离线导入可能不保留 registry RepoDigests，因此不能把本地版本标签当作控制器的 digest 拉取凭据；正式准备和切换前仍需配置专用 GHCR 只读凭据、控制器，并验证实际生产备份恢复。禁止在镜像导入后直接用默认 `docker run` 参数替换生产应用。

```sh
python3 -m unittest discover -s deploy/private -p 'test_*.py' -v
go -C backend test -tags=unit ./internal/service ./internal/handler/admin ./internal/server/middleware ./internal/repository
pnpm --dir frontend exec vitest run src/components/common/__tests__/ImageDeploymentPanel.spec.ts
pnpm --dir frontend run build
```

生产验收应包括公网健康、当前镜像 digest、版本、原数据库/Redis/定价容器启动时间、登录、模型列表、定价、业务调用方访问及必要的受控流式请求。上线前未运行生产备份或恢复时，必须明确标记“待授权和执行”，不能用 CI 空库启动检查替代真实数据恢复验证。

发布流程还会在 GitHub 临时 runner 上创建模拟生产栈，实际验证一致性快照、恢复指纹、两阶段切换和当前容器写入层回滚。它不读取真实生产数据；真实备份恢复仍需取得用户同意。宿主机命令错误详情保存在状态目录的 `command-errors.log`（0600），不要将该文件原样上传或发给非管理员。

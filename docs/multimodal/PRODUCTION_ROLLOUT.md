# 多模态生产发布记录

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

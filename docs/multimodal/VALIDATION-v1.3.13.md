# v1.3.13 验证记录

## 本地回归

- `go -C backend test -tags=unit ./...` 全量通过，包含 `TestMediaGenerationSendsMappedModelExactlyOnce`：按公开模型解析账号能力后，向模拟上游发送配置的映射模型且只提交一次。
- 前端 lint、typecheck 和生产构建通过；全量 381 个测试文件、2,908 项测试通过。同步修正 CC Switch Antigravity 测试中已过期的 Claude 默认模型断言，未修改其运行实现。
- `TestCurrentTemplateEditsWithoutBackupsAndDetachesOnDelete` 在 PostgreSQL 18 隔离数据库通过：模板更新同步账号和媒体监控当前引用，不创建备份；删除后停用账号模型并把媒体监控切换为连接检查；既有任务快照不变。
- 部署控制器及版本迁移测试 73 项成功（其中 1 项按条件跳过）；媒体 OpenAPI/请求规范与 Python/Node 公共客户端校验 14 组通过，公共客户端单测另有 4 项通过。
- 浏览器真实组件 fixture：模板新增、名称/介绍编辑、保存、复制和删除均通过；1440px 桌面下弹窗宽 1408px，390px 手机下内容宽与滚动宽均为 372px，无横向溢出。文档模型选择及 2 张图片、2 段视频、2 段音频的预览/估价请求通过，桌面和手机均无横向溢出，未产生页面异常。素材与供应商响应为 fixture，不是供应商生成验收。
- 发布前只读核验生产 v1.3.11 的既有已完成视频：任务查询 HTTP 200，公共带认证媒体入口 Range 请求 HTTP 206，返回 32 字节 MP4 文件头，任务快照 MD5 不变。生产数据库与 Redis 容器基线仅保留在服务器运维目录。

## 发布与生产

- 正式 [Release v1.3.13](https://github.com/ccfwwm/sub2api-private/releases/tag/v1.3.13) 与 [发布 CI](https://github.com/ccfwwm/sub2api-private/actions/runs/40929813369) 全部成功，包含 PostgreSQL/Redis 镜像检查、匿名拉取、备份恢复、切换/回滚/rescue 演练。
- 运行源码：`94be784f282c65489124aa8cb44ffb0b9c13fc80`。应用 digest：`sha256:8bff64588b9e1cf0aac690a25abee2fc44f47c507a3fbfcfe0aefdcc8c914b34`；文档 digest：`sha256:21d5e48ee486b05c89c8e396bbbfbf57d17c763baddd00656fc4f906f04fb739`。
- 按用户已授权的立即切换方式，使用正式 Release 清单下载并校验管理器与固定镜像，执行 `/root/sub2api-manage upgrade v1.3.13 --immediate --confirm UPGRADE`。任务 `fd23df2acc9f41f394f20288216dd958` 于 2026-10-29 19:02:13 UTC 最终 `succeeded`；备份 `6c5f8413ba744d1c9d5129c126bb7b28` 保留在服务器。完整备份和隔离恢复演练后才切换，未跳过检查。
- 独立上线核验：`code.aicodeme.xyz`、`code.aicodeme.cn`、`hkcode.aicodeme.xyz` 均返回 v1.3.13，健康和文档 HTTP 200，新弹窗 CSS 已加载。PostgreSQL/Redis 容器 ID 和启动时间未变；部署前 6 个未结算任务的配置快照未变，已有视频仍通过公共 Range HTTP 206 下载。
- 生产浏览器验收：文档视频分类可选 7 个公开模型，图片/视频/音频均有 `multiple` 上传入口；1440px 桌面及 390px 手机无横向溢出或页面异常。公开目录验收未输入真实 Key、未提交生成。
- 真实供应商生成：账号 2142，公开模型 `sd-2.5-满血全参数-720p`，配置上游模型 `高质量快速sd2.5(满血30-10-10/720P)`。生产模型目录确认包含该映射值；经真实管理测试接口仅提交一次，SSE 与持久测试记录均确认映射。测试 `mt_96eb6b75fc804a6cb0cad79f632d9da5` 于 2026-10-29 19:18:27 UTC 最终 `completed`，生成 1 个视频，认证成品 Range 下载 HTTP 206，MP4 文件头有效，配置未变化。观察约 13 分钟，上游自行完成，期间仅查询原任务，无重复生成。
- 去敏生产验收回执见 [PRODUCTION-v1.3.13.json](PRODUCTION-v1.3.13.json)。发布标签及镜像保持不变；本次上线后的证据补充只更新 main 分支文档。

# imagevideo 文档与媒体任务改造验证记录

验证日期：2026-10-14。以下为初次本地交付的验证记录，当时未部署、未创建发布标签、未调用收费生成接口、未修改生产数据库。用户随后授权部署生产及合并 `main`，生产发布结果另附记录，不将本地模拟当作生产验收。

当前生产快照：`v1.2.6`，提交 `cecaf2f8e1af01d94dfbea3a472974d75826cf4e`，固定镜像摘要 `sha256:be1a6ee34c58a4c32e9dc548544cb64a650a8f1f57fc5faf9fd73173e4ba41e8`。最新公网与数据回归记录见 [v1.2.6 生产验收](IMAGEVIDEO_PRODUCTION_V126.md)；下方 `v1.2.5` 内容保留为历史记录。

## 交付内容

- 免登录页面 `/docs/imagevideo`，标题为“词元互联图片、视频与音乐接口文档”。首页默认版、简洁版及登录后侧栏均提供入口；后台模式的路由守卫允许匿名访问该页面。
- 深色文档页面包含固定目录、手机折叠目录、参数表、cURL、响应结构、代码及 Markdown 复制功能。所有公开请求地址使用 `https://code.aicodeme.xyz`，分别展示 `/v1` 和 `/v1beta`。
- 模型快照为 26 个精确 ID，保留大小写和别名。以当前配置和实际校验代码说明支持范围，将历史完成记录与当前场景验证区分。未配置素材能力的模型不承诺编辑、首尾帧或参考输入成功。
- Python 和 JavaScript 完整客户端位于 `frontend/public/examples/`：一次提交、保存状态、恢复轮询、鉴权下载、按实际 MIME 保存文件。同步结果、异步结果和同步回退 HTTP 202 均通过任务 ID 进入通用查询流程；未知状态停止自动处理，禁止向外域发送 Key。
- 管理员 `/admin/usage`、用户 `/usage` 均将媒体记录放入使用记录卡片的“媒体任务”Tab，首次打开加载，切换保留页码及筛选。复用表格、分页、每页条数偏好及列设置风格；手机默认折叠筛选区。
- 支持日期、任务 ID、API Key、模型、分组、媒体类型、任务状态、账务状态筛选和创建时间排序。管理员另支持用户和账号筛选，以及成本、毛利可选列。
- 任务详情包含预留、受理费、结算、释放、退款、计费数量、价格版本、倍率、产物大小/时长和诊断；管理员保留退款、成本核实、受理核对及 `expected_version` 冲突保护。

## 接口与兼容性

沿用现有用户、管理员媒体列表路径。传入 `page` 或 `page_size` 启用分页，标准成功响应的 `data` 包含：

```json
{"items":[],"total":0,"page":1,"page_size":20,"pages":0}
```

默认页码 1、每页 20；页码上限 1000000、每页上限 1000。未传分页参数的旧请求仍返回数组，支持原有 `before` 游标；分页与 `before` 混用返回 400。按 `created_at,id` 同方向排序，同时间记录稳定排序。新筛选用于分页查询，旧请求保留原游标行为。

用户身份由认证上下文强制限定；用户请求不能选择其他用户或账号。用户响应不返回用户、账号、供应商成本及毛利。关联名称、产物和计费明细批量读取，不读取凭证及原始请求素材。

新增迁移 `backend/migrations/247_media_task_list_indexes.sql`，包含全局及用户维度的时间/ID 索引。只在本地临时数据库应用，没有执行生产迁移或改写历史账务数据。

## 内容来源与发布快照

公开页面、章节 Markdown、全文复制和导出使用同一结构化源：`frontend/src/data/imagevideoDocs.ts`。客户端程序也从真实下载文件导入，避免示例与文件分叉。

内部参考核验延续 [RECEIPTS.md](RECEIPTS.md)，参考页面为 `https://doc.yunfei.best` 和 `https://www.v6k.top/docs?category=video`。公开内容不包含这些站点的名称、介绍或外链。

当前分组/模型清单来自本轮只读配置核对。历史渠道完成记录仅证明对应历史请求，不能证明全部当前参数与素材场景。特别是 GPT 编辑、Gemini 参考素材、Omni/Veo 参考图以及未设置素材范围的其他视频模型，公开文档明确限制成功承诺；Lyria SSE 按文字事件读取，不描述为音频流。

全文 Markdown 已导出到 [接口调用文档](../../output/imagevideo/接口调用文档.md)。重新导出：

```powershell
Set-Location frontend
node scripts/export-imagevideo-docs.mjs
```

导出过程使用独立 Vite 缓存及未监听端口的 HMR server，可与本地预览并行；生成文件和截图位于忽略的 `output/imagevideo/`，结构化源与导出工具随代码交付。

## 自动验证

下列检查均已通过。命令以各自注明的目录执行。

| 检查 | 命令 / 范围 | 结果 |
|---|---|---|
| 后端单元与 handler 回归 | `go test ./internal/media ./internal/handler -count=1`，目录 `backend` | 通过 |
| PostgreSQL 集成 | `go test -tags integration ./internal/media -count=1`，目录 `backend` | 通过；真实本地 PostgreSQL |
| 前端相关回归 | `vitest run src/components/media/__tests__ src/components/admin/usage/__tests__ src/views/__tests__/ImageVideoDocs.spec.ts src/views/__tests__/HomeView.compact.spec.ts src/i18n/__tests__`，目录 `frontend` | 19 个文件、116 项通过 |
| 最终文档校验 | `vitest run src/views/__tests__/ImageVideoDocs.spec.ts` | 2 项通过；JSON、cURL、26 模型、地址、来源清理及共用复制内容 |
| 类型检查 | `pnpm typecheck` | 通过 |
| 前端构建及国际化 | `pnpm build` | 通过；包含最终产品代码与文档内容 |
| ESLint | 本轮新增/修改的 Vue、TypeScript、导出脚本 | 通过；使用本地 `node_modules/.bin/eslint.cmd` |
| 客户端本地契约测试 | `python docs/multimodal/test_public_clients.py`，仓库根目录 | 3 个测试方法、10 个参数化场景通过 |
| 格式 | `gofmt -l` 本轮 Go 文件；`git diff --check` | 无格式问题 |
| Markdown 导出 | `node scripts/export-imagevideo-docs.mjs` | 通过 |

分页集成测试创建 45 条任务并遍历三页，验证同时间排序、空页、组合筛选、跨用户隔离、旧游标及外用户游标、索引和账务操作的版本冲突。handler 测试覆盖分页/旧响应形状、非法页码/状态/日期/排序及用户身份校验；原有账务测试同时运行。

客户端测试只连接本地 HTTP 模拟服务，覆盖 HTTP 202、同步 HTTP 200、同步回退 202、状态恢复、仅一次 POST、真实 MIME 扩展名、原 Key 鉴权、unknown 停止、外域制品拒绝及异常响应，不产生外部生成费用。

集成测试环境变量：

```powershell
$env:GOCACHE='C:\softworks\gpt-tools\sub2api\output\imagevideo\go-cache'
$env:MEDIA_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55479/media_test?sslmode=disable'
```

测试数据库运行在专用、仅绑定本机端口的 `postgres:18-alpine` 临时容器 `sub2api-imagevideo-test-20261014`，设置 `--rm`，不复用应用或生产数据库。没有挂载工作区或现有数据卷；镜像自动创建的匿名 PostgreSQL 卷随 `--rm` 容器一起删除。测试结束后已关闭容器，并通过名称筛选分别确认容器及其匿名卷均已移除。

## 浏览器验证与截图

本地 Vite 预览：`http://127.0.0.1:5179/docs/imagevideo`。浏览器的应用 API 由本地模拟数据拦截，45 条媒体记录、假用户、假 Key 和账务金额仅用于界面验证，截图不代表真实用户历史或实测生成结果。

- 匿名访问、后台模式公开文档、完整 Markdown 复制、手机目录与音乐锚点均通过；正文及复制内容没有参考站信息。
- 管理员/用户媒体 Tab 首次加载、切换保留第三页、筛选回第一页、每页条数、详情、管理员可选成本/毛利列及列设置保存均通过。
- 普通用户不调用管理员搜索接口，列表及详情不显示管理员成本/账号或账务操作。管理员详情保留原有三类操作。
- 用户错误请求开关开启时，用量明细、错误请求、媒体任务三个 Tab 同时可用；管理员另有用户排行。
- 桌面浅色、深色及 390px 手机视口均已人工检查，页面无横向溢出，手机以原表格组件的卡片布局呈现。

| 界面 | 截图 |
|---|---|
| 文档桌面 | [docs-desktop.png](../../output/imagevideo/screenshots/docs-desktop.png) |
| 文档手机 | [docs-mobile.png](../../output/imagevideo/screenshots/docs-mobile.png) |
| 管理员浅色 / 深色 | [浅色](../../output/imagevideo/screenshots/admin-media-light.png) · [深色](../../output/imagevideo/screenshots/admin-media-dark.png) |
| 管理员详情 / 手机 | [详情](../../output/imagevideo/screenshots/admin-media-detail.png) · [手机](../../output/imagevideo/screenshots/admin-media-mobile.png) |
| 用户浅色 / 深色 | [浅色](../../output/imagevideo/screenshots/user-media-light.png) · [深色](../../output/imagevideo/screenshots/user-media-dark.png) |
| 用户详情 / 手机 | [详情](../../output/imagevideo/screenshots/user-media-detail.png) · [手机](../../output/imagevideo/screenshots/user-media-mobile.png) |

最终页面加载没有控制台错误。现有 `AdminComplianceDialog` 在精简本地设置 fixture 下有一条 `BaseDialog.show` 为 undefined 的警告；不归因于媒体任务或文档。首次建立浏览器拦截前的 404，以及参考站加载错误不计入最终页面结果。构建保留仓库既有 Browserslist 数据过期、包体积、混合静态/动态导入及 Node 弃用警告。

## 实施阶段的上线边界

初始实施阶段不发起收费生成、不执行生产发布或生产索引迁移。用户随后授权发布，现已完成 v1.2.5 生产部署、索引迁移与独立只读验收，见 [生产验收记录](IMAGEVIDEO_PRODUCTION_V125.md)。尚未逐模型执行全部当前素材/参数组合的真实渠道验收；文档仅公布配置可支持范围及已有历史证据，本地 UI 模拟不代表生产真实生成。

## 获授权后的 v1.2.4 发布准备

用户已授权正式生产部署，并要求后续直接在私有 `main` 开发。现有 Release 和宿主机双重校验要求发布提交已经在 `main`，因此先合并 `origin/main` 再触发正式 Release。仅推送普通提交不会切换生产。

补齐迁移 247 的精确 SQL、checksum、单一新增迁移白名单及旧 worker 的受校验资产兼容；不放宽其他 schema 变更。恢复演练增加媒体任务、账务、成本及制品指纹核对，CI 隔离环境额外覆盖索引创建及应急切回。管理工具 40 项测试通过；完整规格和原客户端模拟 14 组通过，公开客户端 3 个方法通过。新媒体组件、文档和首页入口测试已加入发布必跑列表。

生产基线已保存在 `/root/sub2api-ops/state/imagevideo-v124/before.json`：版本 `1.2.3`，296 条已应用迁移，8 条媒体任务、7 个制品、22 条账务分录、7 条成本记录。记录只在服务器保留实际容器挂载与文件哈希，不包含凭证。本次没有发起收费生成。

## 正式发布检查修复（v1.2.5）

v1.2.4 的源代码单元、集成、前端、客户端规格及管理工具检查通过，但 golangci-lint 阻断了 3 处 sqlmock 关闭返回值和 3 处错误消息大写。镜像打包和生产部署未执行。修复关闭返回值处理及错误消息格式后使用 v1.2.5 新标签发布，保留 v1.2.4 标签，不重写已推送的版本。

## 已完成的生产发布

v1.2.5 源码 CI 与正式发布源码测试全部通过。GitHub 付款或额度限制阻止镜像作业启动后，使用原 Dockerfile 在本地专用隔离 Docker 中构建同一提交，并完成 smoke、备份恢复、固定镜像切换、回滚、索引 247 与紧急切回演练。私有 GHCR 摘要与已演练镜像一致；现有生产管理器完成最终备份及恢复演练后部署成功。

生产验收确认在线版本、公开文档、媒体列表分页/筛选/旧游标、索引、旧 checksum 和历史媒体文件完整性。在线文档桌面、手机及完整 Markdown 复制验证通过，未发起收费生成。具体证据、测试边界及 GitHub 后续自动发布限制见 [生产验收记录](IMAGEVIDEO_PRODUCTION_V125.md)。

# v1.3.18 验证记录

## 请求契约

- 失败任务 `mt_e88c14b7-3028-4f51-96cd-9cc474bc4a25` 属于账号 2150、分组 66、模型 `gemini-omni-flash`、Foxrouter。任务快照固定使用 `video-reference-arrays@3`，图片规则为 `shape=preserve`、`encoding=preserve`；入站图片是一个对象，错误明确指出该对象无法解码为上游 `Alias.images` 字符串元素。本站已释放该任务的预留额度。
- 同一账号同一模型的不带图任务 `mt_0a792919-a218-44bd-bb09-ab74c156f2d1` 和 `mt_71338591-1923-4660-880b-0f2e36b8206d` 成功；这只能验证无图路径，不能证明图片结构兼容。另一个上游的成功任务不作为 Foxrouter 模板依据。
- Foxrouter 上游验证：`images` 单个 Base64 Data URL 字符串返回 HTTP 400 并要求 `[]string`；单元素和双元素 Base64 Data URL 字符串数组均返回 HTTP 200 与任务 ID。测试复用已提交任务查询状态，没有重复 POST。
- 本地专用模板将图片对象规范为 Data URL 字符串，并映射到嵌套 `Alias.images`。单图/双图序列化测试及内置模板 HTTP 提交、查询和成品路径合同测试覆盖此形状；模板和绑定均与其他上游隔离。

## 界面与文档

- 媒体联调组件覆盖无需免费预估直接提交、预估未完成时仍可提交、幂等恢复和结果未知时禁止新测试。
- 文档侧栏徽标覆盖当前站点版本读取、直连接口回退、非法版本和网络失败。
- `pnpm run test:run`：381 个测试文件、2,919 项通过；其中媒体联调组件 11 项通过，覆盖无需免费预估直接提交及预估并行提交。
- `pnpm run typecheck`、`pnpm run lint:check` 和 `pnpm run build`：通过。构建保留了现存的 Browserslist 数据过旧、混合动态/静态导入和大 chunk 提示。
- `go test -tags=unit ./internal/media ./internal/handler`：通过；新增模板的单图/双图映射及原生 HTTP 合同覆盖通过。
- `node --test docs-web/app.test.mjs`：13 项通过，包含当前入口徽标和直连回退验证；`git diff --check`：通过。

## 生产

- 后续复核纠正：本文件此前将错误中的 Go 内部类型路径 `Alias.images` 误写为 JSON 嵌套路径。原始直连成功受理的请求实际使用顶层 `images: []string`。修正后的源码回归和生产专属模板验收见 `VALIDATION-v1.3.19.md`；本版本没有切换生产。

- 正式 Release 与生产部署：待记录。
- 生产账号 2150 的 `gemini-omni-flash` 绑定：待记录。
- 三个生产入口版本、任务请求映射、标准状态与成品查询：待记录。
- 数据库和 Redis 容器保护状态：待记录。

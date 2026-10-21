# v1.2.15 候选状态与验收边界

本文件记录 2026-10-21 候选实现与可复核证据，不作为已部署或公网生成成功的声明。桌面、插件和 Sub2API 分别版本化；本轮不重打 ZeroWall 桌面安装器。

## 候选行为

- 通用视频入口保留 URL、MIME Data URL 与校验后的裸 Base64、素材顺序、角色和扩展属性；动态模板及绑定明确指定协议版本和上游模型。
- 规范输入、语义覆盖、计价与发送编译共用有效请求；未知可选参数按规则透传或显式过滤，时长目录提示不代替正整数、大小、模型身份及账务校验。
- 模板、绑定草稿/发布、历史和回滚使用现有配置版本存储；管理界面新建时先保存草稿，再发布，发布失败保留草稿版本供重试。
- 管理员有效配置预览使用真实 API Key、分组、账号候选和已发布模板/绑定；返回覆盖来源、请求与素材摘要及真实估价。离线 URL 明确标记待测量；显式在线预览仅允许素材 GET，不创建任务、不预留费用、不调用生成。管理员付费测试同样拒绝过期、额度耗尽、停用的 Key/用户/分组。
- 新监控生成依赖显式能力和已发布模板，移除按模型名/域名选择协议的回退。历史任务先读 journal，冻结快照用于只读查询与恢复，当前模板变更或缺失不会促成重复 POST。
- 内置模板覆盖同步 JSON、媒体二进制及异步任务，包含独立 Gemini 音频与语音合成模板。原生 HTTP 矩阵直接执行内置路径、认证及解析配置，核验模型发现、提交、查询和实际下载；PCM 转 WAV 仅由显式模板配置启用，保留转换前后 MIME、大小和摘要。
- 公共调试文档统一发送 `images`、`videos`、`audios`，目录未设置素材数量时仍显示输入。五素材浏览器模拟核对 4 张图片、1 个视频完整进入一次提交；文档页面、公开客户端和插件不再选择供应商协议。

## 已有自动化证据

证据目录为 `output/media-templates-v1215/verification/`；本地输出不会随源码自动发布，发布收据应另行归档。

| 检查 | 结果与证据 |
| --- | --- |
| 媒体与 handler 单测 | `media-handler-unit-go1273-20261021.log`：Go 1.27.3 下两个包均通过，分别 13.969 秒、53.650 秒 |
| 有效配置预览与测试身份 | `effective-preview-media-handler-final-20261021.log`：媒体与 handler 定向通过，分别 1.328 秒、6.190 秒；覆盖只读订阅查询、白名单、Key 有效期/额度、在线 GET 实际内容证据及重定向；对应 `go vet` 通过 |
| 媒体隔离 PostgreSQL 集成 | `media-integration-releasefinal-20261021.log`：86 个顶层测试通过、零跳过，包含配置版本、重启/回滚、继承规则发布门禁与旧视频格式流程 |
| 迁移 249/250 | `migrations249250-integration-releasefinal-20261021.log`：三个幂等/迁移用例通过 |
| Go 可达漏洞扫描 | `govulncheck-go1273-20261021.log` 与 `govulncheck-go1273-linux-amd64-20261021.log`：Go 1.27.3、govulncheck v1.9.0，Windows 与生产 Linux/amd64 目标均退出 0，可达漏洞为 0 |
| 受理结果丢失的只读恢复 | `recovery-integration-final-20261021.log`：三个恢复/禁止重投用例通过 |
| service 完整单测 | `go test -tags unit ./internal/service -count=1 -timeout 600s`：通过，216.681 秒；`service-unit-final-20261021.log` |
| service 定向回归 | `service-targeted-final-20261021.log`：监控、映射、时长参数和多账号选择通过，含跨重启、模板不可用及零 POST 门禁 |
| service 静态检查 | `go vet -tags unit ./internal/service`：通过；`service-vet-final-20261021.log` |
| 冻结源码后的媒体与 handler | `media-handler-sourcefreeze-final.log`：两个包通过，2.880 秒、46.343 秒；`media-handler-sourcefreeze-vet.log` 静态检查通过 |
| 冻结源码后的 PostgreSQL 集成 | `media-integration-pcm-sourcefreeze-20261021T130131.jsonl`：102 个顶层测试、含子测试 287 项通过，48.660 秒，零跳过；迁移 249/250 三项通过，23.146 秒 |
| 部署管理器及 SQL 门禁 | `deploy-python-with-pg-pcm-sourcefreeze-20261021T130131.log`：67 项通过，含真实 PostgreSQL SQL 门禁，14.232 秒，零跳过；临时库已精确删除并核验 |
| 公开规范与客户端 | `spec-sourcefreeze-final.log` 的 14 组模拟通过，`clients-sourcefreeze-final.log` 四项客户端测试通过 |

最终前端定向测试为 7 个文件、25 个测试通过，lint、类型检查、i18n 及生产构建通过；公开文档源码测试 5 项通过。Go 1.27.3 完整后端 unit 检查已通过，新增收尾代码另跑媒体与 handler、隔离 PostgreSQL 和迁移门禁。冻结源码后的精确日志与候选镜像隔离验收由构建收据补充。早期失败日志保留，不将中间编译态或旧断言失败抹去，也不将一次单测成功当成完整发布验收。

此前 Go 1.27.2 扫描发现标准库 `crypto/x509` 的可达漏洞 `GO-2026-6809`。本轮仅将 `backend/go.mod`、三个 Go 构建镜像入口和 CI 版本断言统一升级至修复版本 1.27.3，依赖及 `go.sum` 未变。工具链经 `goproxy.cn` 下载，保留 `sum.golang.org` 校验；扫描数据库更新时间为 2026-10-21 00:27:29 UTC。两次最终扫描仍各报告 2 项仅出现在所需模块中、未被调用的漏洞，未将其计作可达漏洞。候选生产镜像仍须实际使用 Go 1.27.3 构建并在发布收据中核验。

## 直连证据与预算

当前审计源为 `output/media-templates-v1215/ledger.json`，SHA-256 为 `dd12e47a645e241481df1fd3eeed382ef0d1af3f17576a7728b7dea227667729`。详情见 `CHANNEL_VERIFICATION_CURRENT.json` 和 `output/media-templates-v1215/audit-20261021/REPORT.md`。

- 共 12 次新尝试：6 completed、1 failed、2 uncertain、3 rejected；3 条历史 reused 不计新提交。总上限 16 次，保守剩余 4 次。
- 6 个 MP4 均通过大小/SHA-256、ffprobe、完整解码和 1/4/7 秒抽帧检查。它们是直连 Veo 的 5 种成功 Base64 形式加 URL 对照，均为 8 秒。
- Veo 的 Data URL `b64_json` 对象用例收到 HTTP 503；不声称六种 Base64 形式全部成功。
- XX 保持五素材、5 秒，本轮没有已核验的可下载成品。两个结果不确定的传输失败没有 task ID，也没有重新提交；历史 URL 记录的下载超时仍未转为成品验收。
- 所有费用均为 `unverified`，直连证据不代表中转模板、网关鉴权、预留/结算/退款或供应商实际费用已验证。

## 仍待发布协调者完成

候选镜像、固定 digest、Git/Release 和生产切换由主协调者在最终门禁后处理。部署后仍需在剩余授权预算内核验 Veo、XX、Grok 的真实网关路径、最终任务、费用和可下载制品；每个新用例至多一次提交，不自动重试，不测 TT，不改模型/时长/素材，不将多参考降为单图。保留备份、原应用制品和规则指针用于回退。

本文件未产生新的安装器、镜像、build ID、公开 URL 或付费请求。发布后的实际路径、大小、SHA-256、镜像 digest 与公网证据须由发布收据提供。

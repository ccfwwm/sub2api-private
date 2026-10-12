# RECEIPTS — 多模态兼容与计费实施规格

核验日期：2026-10-12。使用 `paper-download:citation-receipts` 的 read-before-cite 原则：读取对应正文后逐项核验。本文检查**来源是否声明该能力**；本项目已另外完成收费生成测试，结果见 [真实渠道验收](REAL_CHANNEL_VALIDATION.md)。此处的来源判定不代表真实调用通过。

源码基线：`backend/cmd/server/VERSION=0.2.15`，提交 `603d4c72cc1bf2b2519ded3f7bfaa62a5b271960`。网页正文来自云飞 HTML、V6K 浏览器渲染正文；V6K OpenAPI 自述版本 `1.1.2`、规范版本 `3.1.0`。提取证据的 SHA-256 与核验摘录见 [source-evidence.json](source-evidence.json)，hash覆盖记录中的正文/规范JSON，不代表HTTP原始字节。摘录及模型配置不含API凭证。

VALID：引述与来源一致；ADJUST：来源存在但承诺/归属需收窄；INVALID：不能按原说法使用；UNVERIFIABLE：未得到所需范围/行为证据。产品状态另用“源码已有基础 / 参考服务声明验证 / 待开发 / 待渠道验证 / 本项目实测通过”。来源核验与运行时验收分开：本轮已新增本项目真实生成记录，详见 REAL_CHANNEL_VALIDATION.md；来源声明不自动升级为实测通过。

## 1. 云飞 19 个章节

来源根：[云飞接口文档](https://doc.yunfei.best/#top)。各行的 fragment 是实际 HTML article ID。引文保留关键字段和值，省略示例中占位URL。

| 证据ID/章节 | 来源原文关键摘录 | 对应规格/字段 | 判定及本项目状态 |
|---|---|---|---|
| Y01 [gpt-text](https://doc.yunfei.best/#gpt-text) | “n 是 请固定传 1”；“url（默认）或 b64_json”；“网址默认保存 15 分钟” | GPT JSON、n=1、response_format | VALID；保存15分钟是参考服务约定，不承诺所有渠道；待渠道验证 |
| Y02 [gpt-edit](https://doc.yunfei.best/#gpt-edit) | “image / image[] … PNG / JPEG / WebP 文件，可重复传入” | multipart单图/多图编辑、顺序 | VALID；max_count未公布，需配置；源码已有基础、待补齐 |
| Y03 [banana-text](https://doc.yunfei.best/#banana-text) | “需要自动比例时省略，不要传 auto”；“1K / 2K / 4K，K 必须大写” | Gemini imageConfig、自动比例 | VALID；TEXT返回URL属渠道扩展；待渠道验证 |
| Y04 [banana-edit](https://doc.yunfei.best/#banana-edit) | “多个 inline_data / file_data 可以混用，并按它们在 contents[].parts 中的顺序处理” | Gemini混合参考图 | VALID；按字段遍历结果，不依赖文档固定下标；待渠道验证 |
| Y05 [grok-video-submit](https://doc.yunfei.best/#grok-video-submit) | “1～15 秒，默认 8；兼容 seconds”；“1.5 额外支持 1080p” | Grok两个ID、duration、JSON/multipart | VALID；源码已有Grok基础；参考服务声明验证、本项目待渠道验证 |
| Y06 [grok-video-image](https://doc.yunfei.best/#grok-video-image) | “JSON 里写 {\"url\": ...}”；“字段会被忽略…纯文生视频…照常计费”；“edits…extensions 当前未开放…503” | image.url、input_reference、拒绝错字段 | ADJUST；本项目将明显错误字段改为400；不把参考服务503能力标已开放 |
| Y07 [grok-video-result](https://doc.yunfei.best/#grok-video-result) | “grok 的轮询响应里没有 url 字段…/content 取”；“按渠道 base URL 取回” | 查询/MP4、制品、相对URL | VALID；新制品接口待开发；链接的access参数不得写日志 |
| Y08 [omni-video-submit](https://doc.yunfei.best/#omni-video-submit) | “omni-flash 支持 4 / 6 / 8 / 10（默认 10）”；“veo-3.1-* 只支持 8”；“不生效字段” | Omni/fast/lite/quality不同档位 | VALID；本项目不将静默忽略作为兼容承诺；未知mode不开放 |
| Y09 [omni-video-image](https://doc.yunfei.best/#omni-video-image) | “image … 字符串”；“images … []string”；“image_url 不会被识别” | image/images与错误字段保护 | VALID；多图上限未公布；字段误写本项目拒绝；待渠道验证 |
| Y10 [omni-video-result](https://doc.yunfei.best/#omni-video-result) | “url、download_url、video_url、videos[0]、data[0].url 都是同一个地址”；“无需把网关 API Key 发送给视频文件域名” | 制品去重、跨域凭证隔离 | VALID；多个链接不按多条收费；查询超时不重提 |
| Y11 [veo-text](https://doc.yunfei.best/#veo-text) | “4 / 6 / 8 秒”；“16:9 或 9:16”；“输出分辨率 720p” | generate-preview系列能力 | VALID；不与veo-lite合并；参考服务的失败不扣费不代替本项目账务实现 |
| Y12 [veo-image](https://doc.yunfei.best/#veo-image) | “image_url”；“公网直链、dataURL 或裸 base64” | Veo单图与对象格式拒绝 | VALID；待渠道验证 |
| Y13 [veo-frames](https://doc.yunfei.best/#veo-frames) | “第 1 张为首帧，第 2 张为尾帧” | 有序两图首尾帧 | VALID；恰好两图并保留角色；待渠道验证 |
| Y14 [veo-ref](https://doc.yunfei.best/#veo-ref) | “带图时固定为 8 秒”；“固定为 16:9” | ref带图条件、无图文生条件 | VALID；3图示例不代表最大3图；最大图数待验证 |
| Y15 [audio-chat](https://doc.yunfei.best/#audio-chat) | “非流式…最小格式已通过云飞验证”；“SSE…扩展参数…尚未验证”；“返回文字消息，不是…音频流” | Chat音乐、data[]、SSE | ADJUST；仅**参考服务声明验证**非流式纯音乐，不能称本项目实测 |
| Y16 [audio-submit](https://doc.yunfei.best/#audio-submit) | “当前未在云飞开放（404）”；“HTTP 202…接入后的预期提交响应” | POST audio/generations | ADJUST；是目标协议而非可用端点，本项目待开发且关闭 |
| Y17 [audio-compat](https://doc.yunfei.best/#audio-compat) | “当前返回 404，尚不可用” | music/generations别名 | ADJUST；待开发、待渠道验证，不能对外承诺当前已支持 |
| Y18 [audio-models](https://doc.yunfei.best/#audio-models) | “5–300 秒是输入校验范围…不是输出长度限制”；“时长…可能为 null” | 参数hint、真实时长计量、预留上限 | VALID；不能拿duration=300作为实际最长输出保证；扩展待验证 |
| Y19 [audio-endpoints](https://doc.yunfei.best/#audio-endpoints) | “非流式音乐生成已验证；SSE / 扩展参数待验证”；两个专用路径“404” | 入口状态表 | VALID；两个查询音乐路径是本项目新增，不能归因给云飞 |

Y20：云飞“香蕉&香蕉2”总览表（位于banana-text之前）明确列出两个Gemini ID与共同的1:1、16:9、9:16、4:3、3:4、3:2、2:3、5:4、4:5、21:9，以及“仅香蕉2支持超宽长条”：8:1、4:1、1:4、1:8。判定VALID。能力模板按模型分开保存来源声明枚举，但仍标记待渠道验证、全部关闭；实际开放需逐渠道核实。总览摘录见source-evidence的overview。图像组1K/4K及quality权限依赖分组，不能将尺寸/质量笛卡尔积都当已开通。

## 2. V6K 文档与 OpenAPI

来源：[视频文档](https://www.v6k.top/docs#video)，[OpenAPI 1.1.2](https://www.v6k.top/docs/openapi.json)。React shell不能直接当正文，核验使用渲染后的模型表、参数表、错误与下载段落。

| 证据ID/定位 | 原文/Schema摘录 | 判定、差异与开发要求 |
|---|---|---|
| V01 模型表：00sd2.0 | “5、10、15 秒；默认 5 秒”；“最多 10 张” | VALID；images为公网URL对象，省略resolution；不推断官方名称 |
| V02 模型表：00sd2.5 | “固定 30 秒；默认 30 秒” | VALID；模板固定30，计费不得复用15秒截断；待渠道验证 |
| V03 模型表：XX系列/wan | “seconds 必填；具体可选时长按模型选择” | VALID；未公布枚举标UNVERIFIABLE，不依据通用3600上限开放 |
| V04 模型表：mini/oplu/AM/dola | “分辨率需与模型匹配” | VALID；ID里的480/720不是独立能力证据，分辨率待配置 |
| V05 模型表：video-QY | “视频编辑：需要提供参考视频” | VALID；必需video、其它约束未公布 |
| V06 模型表：doubao四ID | “metadata.content 可传原生素材结构” | VALID；包含`doubao-seedance-2-5/65Z`，保持斜杠；metadata不能覆盖本地价格/身份 |
| V07 模型表：JSadanco2.0 | “seconds 可选”；“最多 9 张” | VALID；URL/URL对象/Data URL；默认时长未公布，不猜测 |
| V08 LLseadanco2.5及示例 | “固定30秒；分辨率720p”；“最多30张”；“单张不超过30 MiB” | VALID；JSON+multipart、URL/对象/Base64/Data URL；单图限制不是总请求大小 |
| V09 F/FF四模型 | “兼容 reference_*_urls 字段；仅接受 JSON” | VALID；具体时长/素材上限未知，默认关闭 |
| V10 MinimaxH3-2K | “4–15秒，默认15秒”；“2K或768p”；“最多9张…3条…3条” | VALID；JSON+multipart；输入视频/音频时长与大小限制待核实 |
| V11 参数表/VideoRequest | seconds整数或字符串；“正整数并不代表该模型一定支持” | VALID；通用Schema校验与逐模型能力校验并行；别名冲突400 |
| V12 状态表/Video.status | 网页四种状态；OpenAPI枚举含unknown | ADJUST；本项目公开五态，未知受理结果不能当失败或自动重提 |
| V13 下载段/GET+HEAD content | “HEAD 和 Range 支持情况取决于上游” | ADJUST；206/416/304不承诺所有模型支持；来源spec列响应不等于已实际返回 |
| V14 错误处理 | “HTTP为200，HTML…提交结果待确认”；“超时…不要自动重复提交” | VALID；客户端先保存state、单次POST、未知停下，无ID不轮询 |
| V15 查询/退款 | “每20秒”；“任务…失败也可能返回HTTP200”；“进入失败终态…按网关流程退款” | ADJUST；20秒适合客户端，不强制所有渠道；来源退款不证明本项目新引擎已实现 |
| V16 完整示例 | 页面Node在created赋值处、Python在seconds字段处结束 | VALID；页面展示的示例不完整，本项目交付完整独立脚本，不复制截断代码 |
| V17 OpenAPI info/paths | openapi=3.1.0，version=1.1.2；models/videos/查询/content/chat/images六路径 | VALID；本项目新增音乐、制品、估价、管理路径独立标proposed，不冒充来源API |
| V18 模型发现 | “出现在列表中表示有访问权限，不等于已通过实际生成测试” | VALID；24模型配置覆盖不等于24模型实测完成 |

24 ID的机械覆盖列表由能力模板保存并由verify_spec检查，保持大小写与拼写。不纠正供应商`sedanco/seadanco`等别名拼写。

## 3. 源码证据

这里的文件位置属于当前工作树，源码观察可复查；网页协议不是源码实现证明。

| 证据ID | 文件/符号 | 结论与判定 |
|---|---|---|
| S01 | `backend/internal/server/routes/gateway.go`，images/videos/chat/Gemini/Ark注册 | VALID；源码已有基础，不是通用38模型已实现 |
| S02 | `backend/internal/service/video_billing_resolution.go`，VideoBillingMaxDurationSeconds、NormalizeVideoBillingDurationSecondsOrDefault | VALID；最大15、默认8；新SD2.5必须独立校验/计量 |
| S03 | 同文件，LookupVideoBillingResolution/OrDefault | VALID；480/720/1080，运行时未知默认480；新2K/768p不能使用此fallback |
| S04 | `backend/internal/service/openai_gateway_usage.go`，calculateOpenAIVideoCost、UsageUnits | VALID；video分支使用videoCount×duration，按次使用videoCount；不能笼统称video都按次 |
| S05 | `backend/internal/service/billing_service.go`，calculatePerRequestCost | VALID；unitPrice==0回退；新配置需显式free而非零即缺失 |
| S06 | `backend/internal/service/grok_media.go`，GrokVideoPendingBilling、任务绑定/计费claim；`service/image_task.go` | VALID；Redis状态/快照基础；不能宣称新DB任务引擎已经完成 |
| S07 | `backend/internal/repository/usage_billing_repo.go`，batch image reserve/capture/release与hold claim | VALID；已有事务扣余额加冻结/去重模式；新额度窗口和任务hold仍待开发 |
| S08 | `backend/internal/service/account_stats_pricing.go`，resolveAccountStatsCost | VALID；可采用用户倍率前费用，不可称可靠供应商实际成本；新媒体未知不回退售价 |
| S09 | `backend/internal/service/seedance.go`，Ark原生任务/用量 | VALID；既有基础需保留；实际token不是Grok按秒的替代别名 |

新价卡/计算器/任务/管理接口/音乐/制品/估价的数据结构与规则为本项目设计，不对外部文档作虚假归因。模板test_only金额为账务样本，供应商实际价格**UNVERIFIABLE**，不自动填生产。

## 4. 尚需渠道实测的证据

每个开放模型：精确模型ID、adapter与能力版本；已脱敏请求hash；提交本地ID/上游ID；真实生成状态；恢复/查询日志；实际制品hash/MIME/大小/时长；费用与供应商cost来源核对；验证者/日期/预算。不得在证据文件保存Key、带access参数的下载链接、大Base64或用户私有素材。

没有完整生成、有效下载与账务核对时，`validation_status`仍为待渠道验证。音乐SSE、专用接口404、Flash额外比例、未公布素材/时长上限、HEAD/Range/地址刷新，需单独验证。模型发现和mock不作为此项证据。


## 本轮运行时核验

VALID：用户账户管理令牌完成两平台认证，专用限额推理 Key 完成模型发现与真实生成。所有结算、下载、HEAD/Range 和费用差值依据隔离台账记录。未发现的 00sd2.5、LLseadanco2.5 以及未知参数模型保持待渠道验证；Minimax 403 不标兼容成功，多图 504 不自动重提。

ADJUST：专用本地音乐入口通过 Chat 音乐适配，不能描述成供应商已开放 audio/music 专用路径。SSE 返回完成消息块，不是增量音频流。配置成本是 estimated，只有独立账单对账才可标 verified。模板全关闭，mock 与模型列表均不算真实生成。

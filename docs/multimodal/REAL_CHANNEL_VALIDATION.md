# REAL_CHANNEL_VALIDATION — 真实渠道验收

## v1.1.2 统一配置补充验收（2026-10-13）

两平台管理访问令牌仍可正常认证。媒体账号、分组和渠道均使用 `imagevideo`；没有同步供应商价格。本地售价及独立账号成本的数据库保存/读回、免费分组优先、渠道回退、缺价拒绝、无价模型隐藏、模型同步不修改能力和价卡快照版本检查通过。

| 平台 | 模型 | 结果 | 本轮供应商费用 USD |
|---|---|---|---|
| 云飞 | `gpt-image-2` | 图片生成成功、settled、GET 200、HEAD 200、Range 206 | 0.01 |
| V6K | `JSadanco2.0` | 视频生成成功、settled、实际 5.088 秒、GET 200、HEAD 200、Range 206 | 0.342466 |

本轮新增 0.352466 USD，累计 5.540890 USD，低于授权 50 USD。费用依据专用推理 Key 的独立配额增量，不混同 Sub2API 本地测试售价。测试配置不导入生产。下文保留前轮详细场景证据。

核验日期：2026-10-12。环境为本地隔离 Sub2API staging（独立 PostgreSQL/Redis、独立数据目录），不是生产；测试 Key 与渠道令牌均只从本机 DPAPI 环境读取，本文不包含凭证、签名下载地址或 Base64。

## 结论

两个平台的 New API 管理接口和专用推理 Key 均返回认证成功。累计专用 Key 配额增量为 **5.178424 USD**（按平台 quota_per_unit=500000 计算），低于本次授权的 50 USD 上限。每个成功案例都验证了：生成或任务完成、一次结算、GET 下载、HEAD、Range（206）和媒体文件探测。验收价卡只存在于隔离 staging，未导入生产。

本报告的“通过”只覆盖列出的模型和输入场景。模型发现列表、规范验证、模板存在和 mock 测试不升级为真实兼容。生产能力/价格仍保持关闭，直到管理员在生产填写正式价卡并完成对应渠道验收。

## 本项目实测通过

| 平台 | 模型 | 场景 |
|---|---|---|
| 云飞 | `gpt-image-2` | 1K/2K 文生图、单图编辑、多图 multipart 编辑（2图顺序） |
| 云飞 | `gemini-3-pro-image-preview` | 文生图 |
| 云飞 | `gemini-3.1-flash-image-preview` | 文生图 |
| 云飞 | `grok-imagine-video` | 480p 视频 |
| 云飞 | `grok-imagine-video-1.5` | 1080p 视频 |
| 云飞 | `omni-flash` | 4秒视频 |
| 云飞 | `veo-3.1-fast` | 视频 |
| 云飞 | `veo-3.1-lite` | 视频 |
| 云飞 | `veo-3.1-quality` | 视频 |
| 云飞 | `lyria-3.5` | Chat 音乐、别名 `/v1/music/generations`、SSE结果字段 |
| 云飞 | `lyria-3-pro` | Chat 音乐 |
| V6K | `JSadanco2.0` | 5秒视频 |
| V6K | `XXsedanco2.0S` | 5秒视频 |
| V6K | `XXsedanco2.5S` | 5秒视频 |

V6K 两个成功案例还验证了实际输出约 5.056 秒、固定/请求时长数量和一次结算。`gpt-image-2` 多图编辑案例确认接受的输入图片数量为 2；Lyria 别名案例记录了约 178.389 秒实际音频时长。

## 失败、待核对和未开放

- `gpt-edit-multiple-v1`：提交 HTTP 504，受理不明，进入 `unknown/manual_review`；没有上游 ID，不重提、不自动释放。
- `gemini-mixed-input-v1`：提交响应不完整，进入 `unknown/manual_review`；不重提、不自动释放。
- `MinimaxH3-2K`：上游 HTTP 403，明确失败，预留释放。
- `XXsedanco2.0`：上游 HTTP 400 参数错误，明确失败，预留释放。
- `seedance-mini`：上游明确失败，预留释放。
- `00sd2.0`、`00sd2.5`、`LLseadanco2.5`：本次账号没有对应访问权限，未调用；不能把 `LLseadanco2.5X` 当成同一模型。
- V6K 文档中公布但当前账号未发现的 Veo generate-preview ID：未调用。

## 账务和恢复验收

隔离 Go integration 测试覆盖：持久任务、余额冻结与释放、旧 HTTP 在途预留竞争、Key/套餐/平台额度、幂等键、并发预留、worker 租约与 fencing、进程重启恢复、失败/受理不明、实际时长/成本快照、退款与成本核对。失败默认全额释放；受理不明任务保留人工核对状态。

浏览器验收覆盖：390px 手机和桌面布局、后台价卡草稿/发布/停用、精确十进制单价、复制配置版本冲突保护、敏感操作二次验证取消路径，以及用户使用记录。二次验证的真实 TOTP grant 未在 staging 伪造，取消/403 路径已验证。

## 状态边界

模型能力发现返回 38 个模板，但模板默认关闭。`本项目实测通过` 只用于上表；`待渠道验证`、缺少有限预留上限、缺少正式价卡或未发现访问权限的模型继续关闭。本报告记录隔离渠道测试。不可变镜像发布、生产迁移演练与切换已另行完成，见 [生产发布记录](PRODUCTION_ROLLOUT.md)；生产未导入上述测试配置。

## 脱敏逐例证据

| 案例 | 模型 | 状态/账务 | GET/HEAD/Range | 下载 SHA-256 |
|---|---|---|---|---|
| gpt-image-1k-v1 | `gpt-image-2` | completed / settled | 200/200/206 | 89247d00ebf3d5359a1579660e8167ea3b183071a4d6bb39c240a4e1428e2ff3 |
| minimax-768p-v1 | `MinimaxH3-2K` | failed / released | —/—/— | — |
| grok-480p-v1 | `grok-imagine-video` | completed / settled | 200/200/206 | 6897064b5c7430e1bb43e68c1c8a3b684305150df69e7c61b48f0580e8ff924c |
| gemini-flash-1k-v1 | `gemini-3.1-flash-image-preview` | completed / settled | 200/200/206 | 10a07d6098d44ae5384d2b1a9dc79433bc9c382d055eb0afb523fdc9b9e7b16d |
| omni-4s-v1 | `omni-flash` | completed / settled | 200/200/206 | 572d67495e9dd742f6b85e85d6999974564aa71a2a5a4ddb531bcc4844108a30 |
| lyria35-v1 | `lyria-3.5` | completed / settled | 200/200/206 | b8ee733f80ecd2b5706629e1269cc98a6cda29f2125db088da5b577fab67599c |
| jsadanco-5s-v1 | `JSadanco2.0` | completed / settled | 200/200/206 | 087c0d858a0d6a10336d98dc4eac32f61badf402b12592877a9300391f12c969 |
| gemini-pro-1k-v1 | `gemini-3-pro-image-preview` | completed / settled | 200/200/206 | f3446ee8107aa4a4f714f579191404ea41813341e43301db49790bbebed78419 |
| grok15-1080p-v1 | `grok-imagine-video-1.5` | completed / settled | 200/200/206 | 588c1ec1b6ec7a5d73dcda86e07bf4328ee3449fe310f05ba1965daf20c23014 |
| veo-lite-v1 | `veo-3.1-lite` | completed / settled | 200/200/206 | d04770b3e062f90a59d6725a4d661c94d9e82b44dffaf91a1796ef9043293cb1 |
| veo-fast-v1 | `veo-3.1-fast` | completed / settled | 200/200/206 | ab102a7c87db329bd20e1c9fb4e4d1b9d49a05f20545e461bf36c6cbb3ee7d3e |
| veo-quality-v1 | `veo-3.1-quality` | completed / settled | 200/200/206 | 0b30c08ad2f7d854ab25bb6ab41ba442c16fd24e111fdb3606183088e64404a5 |
| lyria-pro-v1 | `lyria-3-pro` | completed / settled | 200/200/206 | d1b8c97facf832b74f1cdf06789ae0f2b5ebde77ac5a48647c2f231933d5b825 |
| gpt-image-2k-v1 | `gpt-image-2` | rejected / — | —/—/— | — |
| gpt-image-2k-v2 | `gpt-image-2` | completed / settled | 200/200/206 | 4a0340e8f91b9828189aac8110591a62f4bda753671cdf8e4321d16edd9e6961 |
| gpt-edit-single-v1 | `gpt-image-2` | completed / settled | 200/200/206 | c93ad3ba06366f504fd74b6355cc345d6b588c9011ad3b85596b2f2f7e5434a1 |
| gpt-edit-multiple-v1 | `gpt-image-2` | unknown / manual_review | —/—/— | — |
| lyria-sse-v1 | `lyria-3.5` | completed / settled | 200/200/206 | c8c7fd271257743d432cea65cfdf92e176dd731bc4745cff91a0a09ca03cf1df |
| gpt-edit-multiple-independent-v2 | `gpt-image-2` | completed / settled | 200/200/206 | 87b3b54f7ad5252294c60a2e2103c7d25724a23b08dad3fb65346d09afa03119 |
| gemini-mixed-input-v1 | `gemini-3.1-flash-image-preview` | unknown / manual_review | —/—/— | — |
| lyria-music-alias-v1 | `lyria-3.5` | completed / settled | 200/200/206 | 9bb946f4092bdad7f11a3d9d1a3fbd4321c8effb8f6468720e167afefd46a5b4 |
| XXsedanco2.0-5s-v1 | `XXsedanco2.0` | failed / released | —/—/— | — |
| XXsedanco2.0S-5s-v1 | `XXsedanco2.0S` | completed / settled | 200/200/206 | 6aaf4be917170fe53725410a56ba6de5aaf9757c6b8ea5ea2b9e8432800352c7 |
| XXsedanco2.5S-5s-v1 | `XXsedanco2.5S` | completed / settled | 200/200/206 | 50d214a9f84afeed778189940e99a5912283eb920a4d9a90218ee3cf5df182f6 |
| seedance-mini-5s-v1 | `seedance-mini` | failed / released | —/—/— | — |

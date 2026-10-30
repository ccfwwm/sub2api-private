# v1.3.19 验证记录

## 发布前

- 代码基于 `42a077466`，保留 `.playwright-mcp/` 本地目录，不纳入发布。
- Foxrouter `gemini-omni-flash` 真实契约已验证：顶层 `images` 接受一个或两个 Base64 Data URL 字符串数组；裸字符串会被 HTTP 400 拒绝。错误消息中的 `Alias` 是 Go 内部类型名，不是 JSON 嵌套字段。凭证未写入仓库。
- 本地媒体模板、原生 HTTP 合同、媒体诊断界面和文档脚本回归已通过，详见 `VALIDATION-v1.3.18.md` 的测试清单。
- 最终复核原始直连请求后，修正模板目标为顶层 `images`；以嵌入 Go Alias 类型的解码器验证单图/双图，再运行 `go test -tags=unit ./internal/media -run 'TestFoxrouterTemplateMapsBase64ImageObjectsToStringArray|TestBuiltinTemplate' -count=1`，通过。

## 生产

- 正式 Release Action `41093970458` 已全部成功，版本 `v1.3.19`，发布提交 `b4ab17e61afb4fb213e16157882e840deb361a31`。Manual Production Deployment Action `41099516923` 发起 `immediate=true`；宿主机自动部署任务 `b7d487f997df49d9a3d6c6bb0f42aca7` 返回 `succeeded / online_verified`，管理器任务 `9588e292f20f4b8b81cc80db1b8bd9ac` 返回 `succeeded / cutover_mode=immediate`。备份 `4eb9eaa0b5da46e896fa2df6b73129be` 完成并通过隔离恢复演练。
- 应用 digest `sha256:49833e6214f1bfbe4917a0da3f62383a86c4ad92f9d0aa6a1dfab02a95791b14`；文档 digest `sha256:85c41cbd49e2dfa2ac88e155d0f973de74d2c05412c7a33591c6e6c2a3d0f76f`，两个运行容器的 revision 均与发布提交一致。标签与 Release 产物保持不可变；顶层图片字段修正通过独立发布的生产模板配置生效，并同步修正 main 的内置模板源文件与测试。
- 生产账号 2150、分组 66、`gemini-omni-flash`：通过正式管理员配置接口发布 `foxrouter-video-base64-image-strings@1`，建立精确绑定 `foxrouter-gemini-omni-flash-a2150-g66@1`。在 v1.3.17 运行时先发布正确配置，v1.3.19 启动只补缺失内置模板，不覆盖已发布配置；共享 `video-reference-arrays@3` MD5 保持不变。
- 未运行预估，直接通过公开标准 `POST /v1/videos` 提交单图/双图各一次：`mt_8a2e7c8d-0ebc-4792-8fef-b063908842a3`（414.8 ms）、`mt_f39c1791-a04a-45eb-ad0b-4b285e664b22`（387.2 ms），均 HTTP 202 / queued / local_pending，后续 GET HTTP 200 / upstream_accepted。出站证据分别为顶层 1 个、2 个 Data URL 字符串，没有 Alias 嵌套，也没有重复 POST。
- 两项任务最终均被 Foxrouter 的生成后台以 `upstream_error: AI Studio CreateInteractionStream 协议位置 $delta[1][4][0]: 期望整数` 标记失败；此前直接调用 Foxrouter 的两项任务只读复核也得到相同错误。该结果验证了图片解析/受理和失败账务链路，不能视为成功出图。每项本站预留 USD 0.85 均全部释放，结算 USD 0；没有新生成成品可下载。
- 两项供应商任务 ID 分别是 `task_U8OiZhEbC8vG2Zcnk9i9pNzGre7ki6jU`、`task_K4nV0kC0Jxo59EUdfNwOhYDrZGbYajEG`。原始直连的 `task_PyuEirXMDcLwdgSWHxhbrA0eKhKFYfgf`、`task_jj64JJZJNbP9edwCd4nRBGel1QV35UnL` 同样失败；只读查询复用了这些任务，没有重复生成。
- 对照任务 `mt_2f41a722-b5a1-484a-b566-2d1dfc88f20e` 实属账号 2114 的另一个模型，当前 `unknown / manual_review`，上游提交 HTTP 500 / `fail_to_fetch_task`，没有上游任务 ID；其请求编号为 `req_1793373205616647108_38890`。本站未重投，也未把不确定受理当作生成成功。
- 既有 Foxrouter 无图完成任务 `mt_71338591-1923-4660-880b-0f2e36b8206d` 的标准查询返回 HTTP 200 / completed；`/v1/videos/{id}/content` Range 返回 HTTP 206 / video/mp4 / 32 字节，MP4 文件头有效。该只读回归不代表本轮带图生成成功。
- 三个生产入口 `https://code.aicodeme.cn`、`https://code.aicodeme.xyz`、`https://hkcode.aicodeme.xyz` 的健康检查、文档页、文档脚本、公开文档 API 和公开设置均 HTTP 200，版本均 `1.3.19`。浏览器实际显示 `MEDIA API · v1.3.19`，原始证据默认没有展开，控制台无错误；徽标截图见 `output/media-v1319/docs-v1319.png`。
- 免费预估 HTTP 200，284.1 ms；同一 Key 的任务总数前后均为 348，没有提交生成。付费任务此前在未运行预估的情况下直接提交，证明两个入口互不依赖。
- 切换后再次核对有效绑定与图片目标，仍为账号 2150、分组 66、模型 `gemini-omni-flash` 的顶层 `images` 字符串数组；两项测试幂等键各对应唯一任务，没有重复创建。共享模板 MD5 与部署前一致。
- PostgreSQL 和 Redis 的容器 ID、启动时间均与切换前完全相同；未被替换或重启。应用和文档容器在 2026-10-30 17:39 UTC 完成切换。最终独立检查时间 2026-10-30 17:43 UTC。
- GitHub 部署 Action 的状态回传在上述独立检查时仍显示运行中；宿主机受限 `status` 接口已明确返回成功。没有为重取状态再次执行部署。

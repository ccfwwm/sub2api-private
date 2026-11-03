# 图片参数适配与渠道探测

## 账号图片参数适配

创建或编辑 OpenAI API Key 账号时，可在「图片请求参数适配」中填写：

- 删除字段：例如 `response_format`。
- 参数改名：每行填写 `原字段=目标字段`，例如 `size=resolution`。

配置保存在账号 Extra 中：

```json
{
  "openai_images_request_params": {
    "drop": ["response_format"],
    "rename": {"size": "resolution"}
  }
}
```

规则用于 OpenAI API Key 的图片生成、编辑转发及账号生图测试，支持 JSON 顶层字段和 multipart 字段（包括 `image[]`）。先删除，再对剩余字段同时改名，值和文件内容保持原样。多个源字段映射到同一目标，或目标与未改名字段冲突时，拒绝配置或请求，避免覆盖内容。留空不修改请求；聊天、Responses、OAuth/Codex 请求不使用这些规则。

删除 `response_format` 后，账号测试支持上游返回 `data[].b64_json` 或 `data[].url`。

## 渠道探测

现有主动探活表单和请求模板增加「探测类型」：

| 类型 | 请求与成功判定 |
| --- | --- |
| 文本 | 保留现有平台预设与校验 |
| 图片生成 | OpenAI/Grok 使用 `/v1/images/generations`；Gemini 使用 `/v1beta/models/{model}:generateContent`，要求 HTTP 2xx 且返回图片字段 |
| 自定义接口 | 自选 HTTP 方法、相对路径、JSON 对象请求体、自定义 headers 与响应提取路径 |

图片生成需要填写上游支持的生图模型。预设请求不发送 `response_format`；高级请求配置仍支持合并或替换请求体。Gemini 预设使用 `responseModalities: ["IMAGE"]`，识别 `inlineData` 或图片类型的 `fileData`。

图片与自定义检测默认仅手动运行。勾选「启用周期检测」后才会进入定时任务；切换探测类型会取消此勾选，需要再次明确启用。启用表示接受该请求可能产生的上游费用。文本和已有媒体任务监控保留原调度行为。

## 自定义接口示例

Endpoint 填写 `https://api.example.com/tenant`，其余设置：

```json
{
  "probe_type": "custom",
  "image_probe_schedule_enabled": false,
  "custom_method": "POST",
  "custom_path": "/v1/check",
  "custom_body": {"prompt": "hello"},
  "custom_response_json_path": "data.0.text",
  "custom_response_require_non_empty": true,
  "extra_headers": {"X-Client": "monitor"}
}
```

请求目标为 `https://api.example.com/tenant/v1/check`。默认使用 API Key 的 Bearer 认证；可用自定义 headers 覆盖认证头或添加平台要求的头。HTTP 头覆盖不区分大小写，原有禁止修改的传输类 headers 仍受限制。

响应路径按点分隔，例如 `data.0.text`；留空使用整个响应体。配置路径时要求响应为有效 JSON。开启非空判定后，缺失值、null、空字符串、空数组和空对象均视为失败；关闭后允许空结果，但非 2xx 始终失败。自定义请求体不插入模型或随机文本，因此每次运行只请求一次，额外模型不重复发送同一请求。

路径只能追加到当前 endpoint，不能填写外部域名、查询参数、片段或目录跳转。Endpoint 继续要求 HTTPS，保存时和真实拨号时均检查内网及环回地址，禁止通过重定向绕过。

## 配置兼容

迁移 `255_channel_monitor_probe_config.sql` 为监控和请求模板添加快照字段。已有记录默认 `text`，周期费用开关为 false，非空判定为 true。模板应用后立即刷新对应监控调度。更新接口省略 `custom_body` 保留原值，传 `null` 清空。

主动探测继续遵循渠道监控现有运行模式开关；V2 被动监控模式不会因此启用主动请求。

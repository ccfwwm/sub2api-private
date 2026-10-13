# v1.2.2 媒体渠道监控

渠道监控 V1 新增 `imagevideo`，复用原有监控列表、定时调度、历史、复制及请求模板。账号测试和媒体业务接口保持兼容，不调整价格。

## 配置与检测

- 选择“图片 / 视频 / 音频”，填写 Base URL、API Key、主模型和附加模型。
- 默认“连接检查”：只读取一次 `/v1/models`，检查鉴权和所选上游模型是否可见，不提交生成请求。连接通过不代表生成通过。
- 显式选择“生成检测”后，按间隔为每个模型生成一个产物，上游可能收费。图片、视频、音频使用现有适配器和参数规范化；可修改提示词、恢复默认提示词，并配置尺寸、时长、分辨率等。
- 已知模型使用原媒体预设。别名可映射到上游模型；未知模型需指定接口和完整能力。高级 JSON 可覆盖时长与产物域名限制，保存时执行后端校验。
- 媒体监控不支持账号配额检测。生成检测只接受一个产物，不接受素材、流式请求或未支持参数。

## 异步与状态

异步受理任务在持久数据目录 `data/channel-monitor-tasks` 保存加密日志。应用重启后，下一检测轮继续查询原任务，不重复提交。任务提交前先持久化受理日志；失败、超时、损坏或无法确定是否受理时停止再次提交，管理员点击“重置检测任务”并保存后才能重新生成。修改密钥、参数或能力也要求显式重置。

仅获得有效图片、视频或音频产物后才记录生成通过。异步等待记为 `pending`，不计入故障率或平均检测耗时。历史仅保存状态和脱敏摘要，不输出密钥、原始产物地址或上游任务标识。

## API 兼容

继续使用原 `channel-monitors`、`channel-monitor-templates` CRUD、手动检测和历史接口，无新增 endpoint。`provider` 增加 `imagevideo`，历史状态增加 `pending`，媒体快照保存在原 `body_override` 中；`body_override_mode` 使用 `merge`（无快照时可为 `off`）。

```json
{
  "provider": "imagevideo",
  "check_mode": "probe",
  "primary_model": "omni-flash",
  "body_override_mode": "merge",
  "body_override": {
    "media_monitor": {
      "mode": "generation",
      "models": {
        "omni-flash": {
          "operation": "video",
          "parameters": {
            "prompt": "A blue ball slowly rolls across a plain white floor.",
            "seconds": 4
          }
        }
      }
    }
  }
}
```

省略 `media_monitor` 时默认连接检查。生成模式的 `capability` 可省略以使用已知模型预设；自定义模型需填写与现有 `extra.media_models` 相同的能力结构。

## 发布验证

本轮使用本地模拟上游验证接口、参数、提示词、有效产物、异步恢复、禁止隐式重投及保存读回，真实渠道生成不再重测。`CHANNEL_VERIFICATION_CURRENT.json` 明确复用 v1.2.1 原渠道验证记录和原验证时间，不声称新增实测。

迁移 `246_channel_monitor_imagevideo.sql` 只扩展三个 CHECK 枚举，不新增列、不修改配置或定价。部署仅允许这份经过固定语句和校验和审核的迁移；先真实备份、隔离恢复、比对原数据、验证旧运行程序可回滚，再原子执行迁移并立即切换应用容器。PostgreSQL、Redis、定价服务和其他生产容器保留。

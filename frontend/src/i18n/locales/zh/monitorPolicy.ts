export default {
  "title": "检测提示词与延时",
  "target": "配置对象",
  "defaults": "监控默认配置",
  "inheritance": "留空继承默认值；逐模型配置优先。",
  "prompt": "检测提示词",
  "defaultPrompt": "留空使用当前协议的默认提示词",
  "externalPrompt": "提示词由完整请求体控制，请在请求体中编辑。",
  "normal_latency_ms": "正常耗时上限（秒）",
  "request_timeout_ms": "单次请求超时（秒）",
  "observation_timeout_ms": "生成总观察期限（秒）",
  "latencyHint": "成功但超过正常耗时上限显示降级，仍计入可用率。异步生成使用总耗时；生成中不计入失败。",
  "invalid": "请检查时间范围：正常耗时上限应小于请求超时或异步观察期限；提示词最多 8192 字节。",
  "noData": "暂无数据",
  "readError": "数据读取异常",
  "stale": "结果已过期",
  "lastResult": "实际结果时间",
  "samples": "有效样本：{count}"
}

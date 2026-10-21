
export interface MediaCharge {
  name: string
  unit: string
  unit_price: string | null
  quantity_source: string
  fallback_source: string | null
  usage_field: string | null
  trigger: string
  refundable: boolean
  rounding: { mode: string; step: string; minimum: string; scope: string }
  method: string
  package_quantity: string | null
  tiers: { up_to: string | null; unit_price: string | null }[]
}
export interface MediaPricing {
  media_type: string
  rules: { priority: number; when: Record<string, unknown>; charges: MediaCharge[] }[]
  reservation: { maximum_charge: string | null; max_quantities: Record<string, string>; overrun: string }
  success_policy: string
}
export interface MediaCapability {
  profile_id: string; version: string; model: string; upstream_model: string
  request_format?: 'legacy' | 'yunfei_multimodal' | 'v6k_url' | 'unified'
  template_ref?: { id: string; version: number }
  template_rules?: Record<string, unknown>[]
  provider: string; adapter: string; media_type: string; enabled: boolean; status: string
  validation_status: string; verification_evidence: string
  operations: string[]; formats: string[]; scenarios: string[]
  duration: { mode: string; values: number[]; min: number | null; max: number | null; default: number | null; required: boolean }
  resolutions: string[]; sizes: string[]; aspect_ratios: string[]; qualities: string[]
  resolution_policy: string
  materials: Record<string, { reference_field?: string; formats: string[]; max_count: number | null; max_file_bytes: number | null; max_duration_seconds: string | null }>
  delivery: { poll_seconds: number; head: boolean; range: boolean; refresh: boolean }
  limits: { max_request_bytes: number; max_concurrent_tasks: number; observation_seconds: number }
  artifact_hosts: string[]; usage_fields: Record<string, string>
  [key: string]: unknown
}

export const mediaUnits = ['request', 'output', 'second', 'minute', 'token', 'input_character', 'input_image', 'input_video_second', 'input_audio_second']
export const quantitySources = ['input_characters', 'task_count', 'successful_outputs', 'request_duration', 'fixed_duration', 'artifact_duration', 'provider_duration', 'provider_tokens', 'accepted_input_images', 'accepted_input_video_duration', 'accepted_input_audio_duration']
export const unitNames: Record<string, [string, string]> = {
  request: ['次', 'request'], output: ['产物', 'output'], second: ['秒', 'second'], minute: ['分钟', 'minute'],
  input_character: ['输入字符', 'input character'], token: ['token', 'token'], input_image: ['参考图', 'input image'], input_video_second: ['参考视频秒', 'input video second'], input_audio_second: ['参考音频秒', 'input audio second']
}
export function newMediaCharge(unit = 'request'): MediaCharge {
  const sources: Record<string, string> = { input_character: 'input_characters', request: 'task_count', output: 'successful_outputs', token: 'provider_tokens', input_image: 'accepted_input_images', input_video_second: 'accepted_input_video_duration', input_audio_second: 'accepted_input_audio_duration' }
  return { name: 'generation', unit, unit_price: null, quantity_source: sources[unit] || 'request_duration', fallback_source: null, usage_field: unit === 'token' ? 'total_tokens' : null, trigger: 'success', refundable: true, rounding: { mode: 'none', step: '1', minimum: '0', scope: 'per_output' }, method: 'linear', package_quantity: null, tiers: [] }
}
export function newMediaPricing(): MediaPricing {
  return { media_type: 'video', rules: [{ priority: 0, when: {}, charges: [newMediaCharge()] }], reservation: { maximum_charge: null, max_quantities: {}, overrun: 'cap_user_charge' }, success_policy: 'any_output' }
}
export function newMediaCapability(model = ''): MediaCapability {
  const base: MediaCapability = { profile_id: `manual:${model || Date.now()}`, version: '1', model, upstream_model: model, provider: 'manual', adapter: '', media_type: '', enabled: false, status: 'enabled', validation_status: '待渠道验证', verification_evidence: '', operations: [], formats: ['json'], scenarios: [], duration: { mode: 'unknown', values: [], min: null, max: null, default: null, required: false }, resolutions: [], sizes: [], aspect_ratios: [], qualities: [], resolution_policy: 'optional', materials: {}, delivery: { poll_seconds: 10, head: true, range: true, refresh: false }, limits: { max_request_bytes: 33554432, max_concurrent_tasks: 2, observation_seconds: 3600 }, artifact_hosts: [], usage_fields: {} }
  return mediaDefaults(base)
}
export function mediaDefaults(c: MediaCapability): MediaCapability {
  c.upstream_model ||= c.model
  c.status = 'enabled'
  c.limits ||= {} as MediaCapability['limits']
  c.limits.max_request_bytes ??= 33554432
  c.limits.max_concurrent_tasks ??= 2
  c.limits.observation_seconds ??= 3600
  c.delivery ||= { poll_seconds: 10, head: true, range: true, refresh: false }
  c.delivery.poll_seconds ||= 10
  c.sizes ||= []
  c.materials ||= {}
  c.usage_fields ||= {}
  c.artifact_hosts ||= []
  for (const [kind, m] of Object.entries(c.materials)) {
    if (m.max_count && m.max_count > 0) {
      m.max_file_bytes ??= 33554432
      if (kind !== 'image') m.max_duration_seconds ??= '30'
    }
  }
  if (c.adapter === 'yunfei_gpt_images' && !c.sizes.length) c.sizes = ['1024x1024', '1536x1024', '1024x1536']
  return c
}
export function accountMediaCapabilities(extra: Record<string, unknown> | undefined, credentials: Record<string, unknown> | undefined): MediaCapability[] {
  const caps: MediaCapability[] = JSON.parse(JSON.stringify(extra?.media_models || []))
  if (!('media_models' in (extra || {}))) {
    for (const [model, upstream] of Object.entries((credentials?.model_mapping || {}) as Record<string, string>)) {
      const c = newMediaCapability(upstream); c.model = model; c.upstream_model = upstream; caps.push(c)
    }
  }
  return caps.map(mediaDefaults)
}
export function validateMediaPricing(value?: MediaPricing): string | null {
  if (!value || !value.rules.length) return '请配置媒体模型价格 / Configure model pricing'
  const decimal = /^\d+(?:\.\d{1,12})?$/
  for (const r of value.rules) {
    if (!r.charges.length) return '请添加收费项 / Add a charge'
    for (const c of r.charges) {
      if (c.method === 'linear' && (c.unit_price == null || !decimal.test(c.unit_price))) return '请填写有效单价，0 表示免费 / Enter a price; 0 means free'
    }
  }
  if (value.reservation.maximum_charge != null && !decimal.test(value.reservation.maximum_charge)) return '费用上限无效 / Invalid maximum charge'
  return null
}

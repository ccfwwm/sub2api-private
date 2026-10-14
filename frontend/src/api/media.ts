import { apiClient } from './client'

export type ConfigKind = 'capability' | 'price'
export interface MediaConfigRecord {
  id: string
  version: number
  published_version: number | null
  status: string
  config: Record<string, unknown>
}
export interface MediaArtifact {
  key: string
  type: string
  mime_type: string
  size_bytes?: number | null
  duration_seconds: string | null
  content_url: string
  available: boolean
}
export interface MediaTask {
  id: string
  model: string
  status: string
  created_at: number
  completed_at?: number
  object?: string
  progress?: number
  diagnostic?: string
  data: MediaArtifact[]
  billing: {
    state: string
    reserved: string
    accepted: string
    settled: string
    released: string
    refunded: string
    maximum_charge: string
    version: number
    quantities: Record<string, string[]>
    price_version: string
    multiplier: string
    time_multiplier: string
    calculation?: { total: string; uncapped: string; items: Record<string, unknown>[] }
  }
}
export interface MediaTaskRecord {
  task: MediaTask
  media_type?: string
  operation?: string
  api_key_id?: number
  api_key_name?: string
  group_id?: number
  group_name?: string
  user_email?: string
  account_name?: string
  user_id?: number
  account_id?: number
  cost_status?: string
  cost_amount?: string | null
  margin_amount?: string | null
}
export interface MediaTaskQuery {
  page?: number
  page_size?: number
  task_id?: string
  model?: string
  media_type?: string
  status?: string
  settlement_state?: string
  api_key_id?: number
  group_id?: number
  user_id?: number
  account_id?: number
  start_time?: string
  end_time?: string
  sort_by?: 'created_at'
  sort_order?: 'asc' | 'desc'
}
export interface MediaTaskPage {
  items: MediaTaskRecord[]
  total: number
  page: number
  page_size: number
  pages: number
}
export const mediaAPI = {
  async configs(kind: ConfigKind) { return (await apiClient.get<MediaConfigRecord[]>(`/admin/media/configs/${kind}`)).data },
  async history(kind: ConfigKind, id: string) { return (await apiClient.get<MediaConfigRecord[]>(`/admin/media/configs/${kind}`, { params: { id } })).data },
  async save(kind: ConfigKind, id: string, expected_version: number, config: Record<string, unknown>, publish: boolean) {
    return (await apiClient.post<MediaConfigRecord>(`/admin/media/configs/${kind}`, { id, expected_version, config, publish })).data
  },
  async disable(kind: ConfigKind, id: string, expected_version: number) { return (await apiClient.post(`/admin/media/configs/${kind}/disable`, { id, expected_version })).data },
  async simulate(input: Record<string, unknown>) { return (await apiClient.post<Record<string, unknown>>('/admin/media/simulate', input)).data },
  async tasks(admin = false, before?: string) { return (await apiClient.get<MediaTaskRecord[]>(admin ? '/admin/media/tasks' : '/media/tasks', { params: { before } })).data },
  async taskPage(admin = false, params: MediaTaskQuery = {}) {
    return (await apiClient.get<MediaTaskPage>(admin ? '/admin/media/tasks' : '/media/tasks', { params: { page: 1, page_size: 20, ...params } })).data
  },
  async verifyCost(id: string, amount: string, evidence: string, expected_version: number, key: string) {
    return (await apiClient.post(`/admin/media/tasks/${encodeURIComponent(id)}/cost`, { amount, evidence, expected_version }, { headers: { 'Idempotency-Key': key } })).data
  },
  async refund(id: string, amount: string, reason: string, expected_version: number, key: string) {
    return (await apiClient.post(`/admin/media/tasks/${encodeURIComponent(id)}/refund`, { amount, reason, expected_version }, { headers: { 'Idempotency-Key': key } })).data
  },
  async reconcile(id: string, decision: string, upstream_id: string, evidence: string, expected_version: number) {
    return (await apiClient.post(`/admin/media/tasks/${encodeURIComponent(id)}/reconcile`, { decision, upstream_id, evidence, expected_version })).data
  }
}

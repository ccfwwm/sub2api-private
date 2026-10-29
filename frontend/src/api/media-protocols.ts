import { apiClient } from './client'
import type { MediaConfigRecord } from './media'

export type ProtocolKind = 'templates' | 'bindings'
export interface ProtocolTemplate {
  id: string; name: string; version: string; status: string; schema_version: number
  media_types: string[]; operations: string[]; mode: string; poll_seconds: number
  auth: { mode: string; name?: string; prefix?: string }
  submit: { method: string; path: string; encoding?: string }
  [key: string]: unknown
}
export const mediaProtocolsAPI = {
  async updateTemplate(body: Record<string, unknown>) { return (await apiClient.post<MediaConfigRecord>('/admin/media/templates/current', body)).data },
 async effective(group_id: number) {return (await apiClient.get(`/admin/media/bindings/effective`,{params:{group_id}})).data},
  async list(kind: ProtocolKind) { return (await apiClient.get<MediaConfigRecord[]>(`/admin/media/${kind}`)).data },
  async save(kind: ProtocolKind, body: Record<string, unknown>) { return (await apiClient.post<MediaConfigRecord>(`/admin/media/${kind}`, body)).data },
  async import(kind: ProtocolKind, body: Record<string, unknown>) { return (await apiClient.post<MediaConfigRecord>(`/admin/media/${kind}/import`, body)).data },
  async publish(kind: ProtocolKind, body: Record<string, unknown>) { return (await apiClient.post<MediaConfigRecord>(`/admin/media/${kind}/publish`, body)).data },
  async rollback(kind: ProtocolKind, body: Record<string, unknown>) { return (await apiClient.post<MediaConfigRecord>(`/admin/media/${kind}/rollback`, body)).data },
  async archive(kind: ProtocolKind, id: string, expected_version: number) { return (await apiClient.post(`/admin/media/${kind}/archive`, { id, expected_version })).data },
  async export(kind: ProtocolKind, id: string) { return (await apiClient.get(`/admin/media/${kind}/${encodeURIComponent(id)}/export`)).data },
  async preview(body: Record<string, unknown>) { return (await apiClient.post('/admin/media/templates/preview', body)).data },
  async discover(body: Record<string, unknown>) { return (await apiClient.post<{ models: string[] }>('/admin/media/templates/discover', body)).data },
  async test(body: Record<string, unknown>, idempotencyKey: string) { return (await apiClient.post('/admin/media/templates/test', body, { headers: { 'Idempotency-Key': idempotencyKey } })).data }
}

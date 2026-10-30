const PREVIEW_TIMEOUT_BASE_MS = 30_000
const PREVIEW_TIMEOUT_MAX_MS = 180_000
const PREVIEW_UPLOAD_BYTES_PER_SECOND = 128 * 1024

/** Extend free mapping preview time for large encoded materials, with a hard cap. */
export function mediaPreviewTimeoutMs(body: Record<string, unknown>) {
  const bytes = new TextEncoder().encode(JSON.stringify(body)).byteLength
  const uploadAllowance = Math.ceil(bytes / PREVIEW_UPLOAD_BYTES_PER_SECOND) * 1_000
  return Math.min(PREVIEW_TIMEOUT_MAX_MS, PREVIEW_TIMEOUT_BASE_MS + uploadAllowance)
}

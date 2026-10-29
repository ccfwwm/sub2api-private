import { describe, expect, it } from 'vitest'
import { buildCcSwitchImportDeeplink, CLAUDE_CC_SWITCH_MODEL } from '../ccswitchImport'

describe('Antigravity CC Switch endpoint', () => {
  it.each([
    ['https://api.example.com', 'https://api.example.com/antigravity'],
    ['https://api.example.com/', 'https://api.example.com/antigravity'],
    ['https://api.example.com///', 'https://api.example.com/antigravity'],
    ['https://api.example.com/sub2api/', 'https://api.example.com/sub2api/antigravity']
  ])('joins the platform path onto %s for both clients', (baseUrl, endpoint) => {
    for (const clientType of ['claude', 'gemini'] as const) {
      const url = new URL(buildCcSwitchImportDeeplink({
        baseUrl, clientType, platform: 'antigravity', providerName: 'Sub2API',
        apiKey: 'sk-test', usageScript: 'return true'
      }))
      expect(url.searchParams.get('endpoint')).toBe(endpoint)
      expect(url.searchParams.get('app')).toBe(clientType)
      expect(url.searchParams.get('homepage')).toBe(baseUrl)
      if (clientType === 'claude') {
        expect(url.searchParams.get('model')).toBe(CLAUDE_CC_SWITCH_MODEL)
        expect(url.searchParams.get('opusModel')).toBe(CLAUDE_CC_SWITCH_MODEL)
      } else {
        expect(url.searchParams.has('model')).toBe(false)
        expect(url.searchParams.has('opusModel')).toBe(false)
      }
    }
  })
})

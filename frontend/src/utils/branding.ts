import { sanitizeUrl } from '@/utils/url'
import ciyuanLogo from '@/assets/landing/ciyuan.webp'

export const ciyuanSiteUrl = 'https://code.aicodeme.xyz'

export function isCiyuanBrand(siteName: string, siteOrigin: string): boolean {
  return siteName.trim() === '词元互联' || siteOrigin === ciyuanSiteUrl
}

export function gatewayBrandLogo(siteName: string, siteOrigin: string, configuredLogo: string): string {
  return isCiyuanBrand(siteName, siteOrigin) ? ciyuanLogo : configuredLogo || ciyuanLogo
}

export function consoleBrandLogo(siteName: string, siteOrigin: string, configuredLogo: string): string {
  return configuredLogo || (isCiyuanBrand(siteName, siteOrigin) ? ciyuanLogo : '/logo.svg')
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = sanitizedLogoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = sanitizedLogoUrl
}

import claude from '@/assets/landing/logos/claude.svg'
import deepseek from '@/assets/landing/logos/deepseek.svg'
import gemini from '@/assets/landing/logos/gemini.svg'
import grok from '@/assets/landing/logos/grok.svg'
import kimi from '@/assets/landing/logos/kimi.svg'
import openai from '@/assets/landing/logos/openai.svg'

export const featuredGatewayModels = [
  { id: 'claude', label: 'Claude', logo: claude },
  { id: 'gpt', label: 'GPT', logo: openai },
  { id: 'gemini', label: 'Gemini', logo: gemini },
  { id: 'deepseek', label: 'DeepSeek', logo: deepseek },
  { id: 'kimi', label: 'Kimi', logo: kimi },
] as const

export const gatewayModels = [...featuredGatewayModels, { id: 'grok', label: 'Grok', logo: grok }] as const

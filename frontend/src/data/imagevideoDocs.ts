import pythonClient from '../../public/examples/imagevideo.py?raw'
import javascriptClient from '../../public/examples/imagevideo.mjs?raw'

export const docsTitle = '词元互联图片、视频与音乐接口文档'
export const docsVersion = 'v1.2.6 · 2026-10-14'
export const apiOrigin = 'https://code.aicodeme.xyz'
export type DocBlock = { kind: 'heading'; text: string } | { kind: 'endpoint'; method: string; path: string } | { kind: 'text'; text: string } | { kind: 'code'; language: string; text: string; label: string } | { kind: 'table'; headers: string[]; rows: string[][] }
export interface DocSection { id: string; group: string; navTitle: string; title: string; subtitle: string; blocks: DocBlock[] }
const text = (value: string): DocBlock => ({ kind: 'text', text: value })
const code = (language: string, value: string, label = language): DocBlock => ({ kind: 'code', language, text: value, label })
const table = (headers: string[], rows: string[][]): DocBlock => ({ kind: 'table', headers, rows })
const json = (value: unknown, label = '响应示意（字段按场景变化）') => code('json', JSON.stringify(value, null, 2), label)
const curl = (path: string, payload?: unknown) => code('bash', `curl '${apiOrigin}${path}' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY"${payload === undefined ? '' : ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(payload, null, 2)}'`}`, 'cURL')
const taskResponse = { id: 'task_example', object: 'video', model: 'grok-imagine-video', status: 'queued', progress: 0, created_at: 1791936000, billing: { currency: 'USD', state: 'held', reserved: '0.00', accepted: '0.00', settled: '0.00', released: '0.00', refunded: '0.00', maximum_charge: '0.00', version: 1, quantities: {} }, data: [] }

const endpoint = (method: string, path: string): DocBlock => ({kind:'endpoint',method,path})
export const imagevideoModels: {id:string;type:string;limits:string}[] = [
  {
    "id": "gpt-image-2-1k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gpt-image-2-4k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gpt-image-2.5-4k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gpt-image-2.5-flare-1k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gpt-image-2.5-sunburst-1k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gpt-image-2.5-sunburst-4k",
    "type": "image",
    "limits": "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
  },
  {
    "id": "gemini-3-pro-image-default",
    "type": "image",
    "limits": "Gemini 原生 generateContent；文本生图"
  },
  {
    "id": "gemini-3-pro-image-preview-default",
    "type": "image",
    "limits": "Gemini 原生 generateContent；文本生图"
  },
  {
    "id": "gemini-3.1-flash-image-default",
    "type": "image",
    "limits": "Gemini 原生 generateContent；文本生图"
  },
  {
    "id": "gemini-3.1-flash-image-preview-default",
    "type": "image",
    "limits": "Gemini 原生 generateContent；文本生图"
  },
  {
    "id": "gemini-nano-banana-2.1-default",
    "type": "image",
    "limits": "Gemini 原生 generateContent；文本生图"
  },
  {
    "id": "grok-imagine-video",
    "type": "video",
    "limits": "1–15 秒，默认 8；480p / 720p；单张参考图"
  },
  {
    "id": "grok-imagine-video-1.5",
    "type": "video",
    "limits": "1–15 秒，默认 8；480p / 720p / 1080p；单张参考图"
  },
  {
    "id": "omni-flash",
    "type": "video",
    "limits": "4 / 6 / 8 / 10 秒，默认 10；720p；16:9；文本生成"
  },
  {
    "id": "veo-3.1-fast",
    "type": "video",
    "limits": "8 秒；720p；16:9；文本生成"
  },
  {
    "id": "veo-3.1-lite",
    "type": "video",
    "limits": "8 秒；720p；16:9；文本生成"
  },
  {
    "id": "veo-3.1-quality",
    "type": "video",
    "limits": "8 秒；720p；16:9；文本生成"
  },
  {
    "id": "MinimaxH3-2K",
    "type": "video",
    "limits": "4–15 秒，默认 15；768p / 2K；图片 9、视频 3、音频 3"
  },
  {
    "id": "JSadanco2.0",
    "type": "video",
    "limits": "5–15 秒；显式 seconds；最多 9 张参考图"
  },
  {
    "id": "doolaa2.0",
    "type": "video",
    "limits": "5–15 秒；显式 seconds；文本生成"
  },
  {
    "id": "doolaa2.5",
    "type": "video",
    "limits": "5–30 秒；显式 seconds；文本生成"
  },
  {
    "id": "XXsedanco2.5S",
    "type": "video",
    "limits": "5–30 秒；显式 seconds；文本生成"
  },
  {
    "id": "TTseadanco2.5-720",
    "type": "video",
    "limits": "5–30 秒；显式 seconds；文本生成"
  },
  {
    "id": "wan3-1080p1",
    "type": "video",
    "limits": "5–30 秒；显式 seconds；文本生成"
  },
  {
    "id": "lyria-3-pro",
    "type": "audio",
    "limits": "Chat 与专用音乐接口；5–300 秒时长提示；n=1"
  },
  {
    "id": "lyria-3.5",
    "type": "audio",
    "limits": "Chat 与专用音乐接口；5–300 秒时长提示；n=1"
  }
]

export const imagevideoSections: DocSection[] = [

  {
    "id": "start",
    "group": "开始使用",
    "navTitle": "认证与快速调用",
    "title": "快速开始",
    "subtitle": "一个 API Key，连接图片、视频与音乐创作。",
    "blocks": [
      {
        "kind": "text",
        "text": "本页按调用场景提供完整地址、参数表、请求示例、响应与结果读取方式。在控制台创建绑定“图片音频视频分组”（imagevideo）的 API Key。生成与下载使用 API Key；控制台登录密码和登录令牌用于账号管理。"
      },
      {
        "kind": "table",
        "headers": [
          "用途",
          "完整地址"
        ],
        "rows": [
          [
            "本站根地址",
            "https://code.aicodeme.xyz"
          ],
          [
            "OpenAI 兼容根地址",
            "https://code.aicodeme.xyz/v1"
          ],
          [
            "Gemini 原生根地址",
            "https://code.aicodeme.xyz/v1beta"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "认证与环境变量"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "export MEDIA_API_KEY=\"<你的 API Key>\"",
        "label": "macOS / Linux"
      },
      {
        "kind": "code",
        "language": "powershell",
        "text": "$env:MEDIA_API_KEY = \"<你的 API Key>\"\n# PowerShell 请使用 curl.exe 调用 cURL。",
        "label": "PowerShell"
      },
      {
        "kind": "code",
        "language": "http",
        "text": "Authorization: Bearer <API_KEY>\nContent-Type: application/json",
        "label": "JSON 请求头"
      },
      {
        "kind": "text",
        "text": "上传文件时使用 multipart/form-data，不要手动设置 Content-Type，cURL 会生成正确的 boundary。代码中的文件名、素材 URL、task_example 请替换为自己的值。"
      },
      {
        "kind": "heading",
        "text": "最小图片请求"
      },
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/images/generations"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/images/generations' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"gpt-image-2-1k\",\n  \"prompt\": \"湖蓝色调的山谷，清晨薄雾，极简插画\",\n  \"n\": 1\n}'",
        "label": "cURL"
      },
      {
        "kind": "text",
        "text": "HTTP 200 时读取 data[].url 或解码 data[].b64_json。本站 URL 通常是相对路径，拼接本站根地址并携带创建任务的原 API Key 下载。同步等待约 175 秒，也可能返回 HTTP 202 任务状态，此时保存 id 并继续查询。async:true 可立即返回任务状态；客户端断开不取消任务。"
      }
    ]
  },
  {
    "id": "models",
    "group": "开始使用",
    "navTitle": "模型清单与选择",
    "title": "模型清单",
    "subtitle": "26 个模型 ID，按完整名称调用。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/models"
      },
      {
        "kind": "text",
        "text": "模型 ID 区分大小写。本页清单对应 v1.2.6；使用自己的 Key 查询当前可见模型。通用 OpenAI 客户端填写兼容根地址 /v1，下载脚本的 MEDIA_BASE_URL 填本站根地址。"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/models' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"object\": \"list\",\n  \"data\": [\n    {\n      \"id\": \"gpt-image-2-1k\",\n      \"object\": \"model\"\n    },\n    {\n      \"id\": \"lyria-3.5\",\n      \"object\": \"model\"\n    }\n  ]\n}",
        "label": "模型响应节选"
      },
      {
        "kind": "table",
        "headers": [
          "模型 ID",
          "媒体类型",
          "调用范围"
        ],
        "rows": [
          [
            "gpt-image-2-1k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gpt-image-2-4k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gpt-image-2.5-4k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gpt-image-2.5-flare-1k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gpt-image-2.5-sunburst-1k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gpt-image-2.5-sunburst-4k",
            "image",
            "文生图、multipart 图生图；n=1；1024x1024 / 1536x1024 / 1024x1536"
          ],
          [
            "gemini-3-pro-image-default",
            "image",
            "Gemini 原生 generateContent；文本生图"
          ],
          [
            "gemini-3-pro-image-preview-default",
            "image",
            "Gemini 原生 generateContent；文本生图"
          ],
          [
            "gemini-3.1-flash-image-default",
            "image",
            "Gemini 原生 generateContent；文本生图"
          ],
          [
            "gemini-3.1-flash-image-preview-default",
            "image",
            "Gemini 原生 generateContent；文本生图"
          ],
          [
            "gemini-nano-banana-2.1-default",
            "image",
            "Gemini 原生 generateContent；文本生图"
          ],
          [
            "grok-imagine-video",
            "video",
            "1–15 秒，默认 8；480p / 720p；单张参考图"
          ],
          [
            "grok-imagine-video-1.5",
            "video",
            "1–15 秒，默认 8；480p / 720p / 1080p；单张参考图"
          ],
          [
            "omni-flash",
            "video",
            "4 / 6 / 8 / 10 秒，默认 10；720p；16:9；文本生成"
          ],
          [
            "veo-3.1-fast",
            "video",
            "8 秒；720p；16:9；文本生成"
          ],
          [
            "veo-3.1-lite",
            "video",
            "8 秒；720p；16:9；文本生成"
          ],
          [
            "veo-3.1-quality",
            "video",
            "8 秒；720p；16:9；文本生成"
          ],
          [
            "MinimaxH3-2K",
            "video",
            "4–15 秒，默认 15；768p / 2K；图片 9、视频 3、音频 3"
          ],
          [
            "JSadanco2.0",
            "video",
            "5–15 秒；显式 seconds；最多 9 张参考图"
          ],
          [
            "doolaa2.0",
            "video",
            "5–15 秒；显式 seconds；文本生成"
          ],
          [
            "doolaa2.5",
            "video",
            "5–30 秒；显式 seconds；文本生成"
          ],
          [
            "XXsedanco2.5S",
            "video",
            "5–30 秒；显式 seconds；文本生成"
          ],
          [
            "TTseadanco2.5-720",
            "video",
            "5–30 秒；显式 seconds；文本生成"
          ],
          [
            "wan3-1080p1",
            "video",
            "5–30 秒；显式 seconds；文本生成"
          ],
          [
            "lyria-3-pro",
            "audio",
            "Chat 与专用音乐接口；5–300 秒时长提示；n=1"
          ],
          [
            "lyria-3.5",
            "audio",
            "Chat 与专用音乐接口；5–300 秒时长提示；n=1"
          ]
        ]
      },
      {
        "kind": "text",
        "text": "GPT 用于生成与编辑图片；Gemini 使用原生协议；Grok 提供文字与单图视频；JS / Minimax 支持参考素材；Lyria 生成音乐。duration 与 seconds、ratio 与 aspect_ratio 是同义字段，同一请求选其中一个即可。"
      }
    ]
  },
  {
    "id": "gpt-text",
    "group": "GPT 图片",
    "navTitle": "文生图",
    "title": "GPT · 文生图",
    "subtitle": "使用 JSON 文字提示生成图片。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/images/generations"
      },
      {
        "kind": "table",
        "headers": [
          "模型 ID",
          "用途"
        ],
        "rows": [
          [
            "gpt-image-2-1k",
            "文生图与参考图编辑"
          ],
          [
            "gpt-image-2-4k",
            "文生图与参考图编辑"
          ],
          [
            "gpt-image-2.5-4k",
            "文生图与参考图编辑"
          ],
          [
            "gpt-image-2.5-flare-1k",
            "文生图与参考图编辑"
          ],
          [
            "gpt-image-2.5-sunburst-1k",
            "文生图与参考图编辑"
          ],
          [
            "gpt-image-2.5-sunburst-4k",
            "文生图与参考图编辑"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "六个 GPT 图片模型之一，保留完整别名"
          ],
          [
            "prompt",
            "string",
            "是",
            "图片描述或编辑要求，中英文均可"
          ],
          [
            "n",
            "integer",
            "否",
            "固定为 1，建议显式填写"
          ],
          [
            "size",
            "string",
            "否",
            "1024x1024 / 1536x1024 / 1024x1536；模型别名中的 1k / 4k 不作为 size 参数值"
          ],
          [
            "response_format",
            "string",
            "否",
            "url（默认）或 b64_json"
          ],
          [
            "async",
            "boolean",
            "否",
            "true 立即返回任务状态；表单使用字符串 true"
          ]
        ]
      },
      {
        "kind": "text",
        "text": "提示词建议描述主体、构图、风格、光线和画面文字。size 是像素尺寸，省略时由模型决定输出；模型别名与 size 为不同字段。"
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/images/generations' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"gpt-image-2-1k\",\n  \"prompt\": \"清晨山谷，柔和自然光，摄影风格\",\n  \"size\": \"1536x1024\",\n  \"n\": 1,\n  \"response_format\": \"url\"\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"created\": 1791936000,\n  \"data\": [\n    {\n      \"url\": \"/v1/tasks/task_example/artifacts/output-0/content\"\n    }\n  ]\n}",
        "label": "URL 格式 · HTTP 200"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"created\": 1791936000,\n  \"data\": [\n    {\n      \"b64_json\": \"<Base64 图片内容>\"\n    }\n  ]\n}",
        "label": "Base64 格式 · HTTP 200"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"image.generation\",\n  \"model\": \"gpt-image-2-1k\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "异步任务 · HTTP 202"
      },
      {
        "kind": "text",
        "text": "HTTP 200 时读取 data[].url 或解码 data[].b64_json。本站 URL 通常是相对路径，拼接本站根地址并携带创建任务的原 API Key 下载。同步等待约 175 秒，也可能返回 HTTP 202 任务状态，此时保存 id 并继续查询。async:true 可立即返回任务状态；客户端断开不取消任务。"
      }
    ]
  },
  {
    "id": "gpt-edit",
    "group": "GPT 图片",
    "navTitle": "图生图 · 单图 / 多图",
    "title": "GPT · 图生图与编辑",
    "subtitle": "上传一张或两张参考图，通过 /v1/images/edits 编辑、合成或改变风格。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/images/edits"
      },
      {
        "kind": "text",
        "text": "使用 multipart/form-data。单图使用 image=@文件，多图重复 image[]=@文件；用“第一张图 / 第二张图”指代上传顺序。模型名称与文生图相同。"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "六个 GPT 图片模型之一，保留完整别名"
          ],
          [
            "image / image[]",
            "file",
            "是",
            "PNG / JPEG / WebP，1–2 张；每张最多 32 MiB；整个请求也最多 32 MiB，留出表单开销"
          ],
          [
            "prompt",
            "string",
            "是",
            "图片描述或编辑要求，中英文均可"
          ],
          [
            "n",
            "integer",
            "否",
            "固定为 1，建议显式填写"
          ],
          [
            "size",
            "string",
            "否",
            "1024x1024 / 1536x1024 / 1024x1536；模型别名中的 1k / 4k 不作为 size 参数值"
          ],
          [
            "response_format",
            "string",
            "否",
            "url（默认）或 b64_json"
          ],
          [
            "async",
            "boolean",
            "否",
            "true 立即返回任务状态；表单使用字符串 true"
          ],
          [
            "mask",
            "file",
            "否",
            "PNG 透明遮罩，与第一张参考图同尺寸；透明区域表示编辑区域，遮罩不计入参考图数量"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "单张参考图"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/images/edits' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -F 'model=gpt-image-2-1k' \\\n  -F 'image=@reference.png' \\\n  -F 'prompt=保持主体轮廓，将红色方块改成蓝色圆形，背景保持白色' \\\n  -F 'n=1' \\\n  -F 'size=1024x1024' \\\n  -F 'response_format=url'",
        "label": "单图编辑 · cURL"
      },
      {
        "kind": "heading",
        "text": "多张参考图"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/images/edits' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -F 'model=gpt-image-2-1k' \\\n  -F 'image[]=@scene.png' \\\n  -F 'image[]=@subject.png' \\\n  -F 'prompt=将第二张图的主体放入第一张图的场景，统一光线与透视' \\\n  -F 'n=1' \\\n  -F 'response_format=url'",
        "label": "多图合成 · cURL"
      },
      {
        "kind": "heading",
        "text": "使用遮罩"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/images/edits' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -F 'model=gpt-image-2-1k' \\\n  -F 'image=@reference.png' \\\n  -F 'mask=@mask.png' \\\n  -F 'prompt=将遮罩透明区域改为蓝色圆形，保持其他区域' \\\n  -F 'n=1'",
        "label": "局部编辑 · cURL"
      },
      {
        "kind": "text",
        "text": "n、async 等表单字段使用普通文本值。不要重复提交不同名称的同一素材字段。需直接获得图片编码时，改用 response_format=b64_json。"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"created\": 1791936000,\n  \"data\": [\n    {\n      \"url\": \"/v1/tasks/task_example/artifacts/output-0/content\"\n    }\n  ]\n}",
        "label": "URL 格式 · HTTP 200"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"created\": 1791936000,\n  \"data\": [\n    {\n      \"b64_json\": \"<Base64 图片内容>\"\n    }\n  ]\n}",
        "label": "Base64 格式 · HTTP 200"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"image.generation\",\n  \"model\": \"gpt-image-2-1k\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "异步任务 · HTTP 202"
      },
      {
        "kind": "text",
        "text": "HTTP 200 时读取 data[].url 或解码 data[].b64_json。本站 URL 通常是相对路径，拼接本站根地址并携带创建任务的原 API Key 下载。同步等待约 175 秒，也可能返回 HTTP 202 任务状态，此时保存 id 并继续查询。async:true 可立即返回任务状态；客户端断开不取消任务。"
      }
    ]
  },
  {
    "id": "gpt-result",
    "group": "GPT 图片",
    "navTitle": "响应字段与保存图片",
    "title": "GPT · 保存图片",
    "subtitle": "URL 下载、Base64 解码与任务恢复。",
    "blocks": [
      {
        "kind": "table",
        "headers": [
          "字段",
          "类型",
          "说明"
        ],
        "rows": [
          [
            "id",
            "string",
            "本站任务 ID，可统一查询"
          ],
          [
            "created",
            "integer",
            "创建时间，Unix 秒"
          ],
          [
            "data",
            "array",
            "生成的图片列表"
          ],
          [
            "data[].url",
            "string",
            "制品下载路径"
          ],
          [
            "data[].b64_json",
            "string",
            "Base64 文件内容"
          ],
          [
            "usage",
            "object",
            "渠道返回时提供实际用量"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "URL 鉴权下载"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/tasks/task_example/artifacts/output-0/content' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" -D headers.txt -o result.png",
        "label": "图片下载"
      },
      {
        "kind": "heading",
        "text": "Base64 保存"
      },
      {
        "kind": "code",
        "language": "python",
        "text": "import base64\nfrom pathlib import Path\n\n# result 是接口 JSON 响应。\nPath(\"result.png\").write_bytes(base64.b64decode(result[\"data\"][0][\"b64_json\"]))",
        "label": "Python · 图片解码"
      },
      {
        "kind": "text",
        "text": "URL 下载按实际 Content-Type 选择 .png / .jpg / .webp。Base64 内容是文件编码，解码后保存，无需再次请求 URL。同步响应带任务 ID 时，可用本页客户端 resume 统一查询并保存制品。"
      }
    ]
  },
  {
    "id": "gemini",
    "group": "Gemini 图片",
    "navTitle": "原生文生图",
    "title": "Gemini · 原生文生图",
    "subtitle": "模型名放在 URL 中，文字提示放在 contents.parts。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1beta/models/gemini-3.1-flash-image-preview-default:generateContent"
      },
      {
        "kind": "table",
        "headers": [
          "模型 ID",
          "协议"
        ],
        "rows": [
          [
            "gemini-3-pro-image-default",
            "Gemini generateContent"
          ],
          [
            "gemini-3-pro-image-preview-default",
            "Gemini generateContent"
          ],
          [
            "gemini-3.1-flash-image-default",
            "Gemini generateContent"
          ],
          [
            "gemini-3.1-flash-image-preview-default",
            "Gemini generateContent"
          ],
          [
            "gemini-nano-banana-2.1-default",
            "Gemini generateContent"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "URL 中的 model",
            "string",
            "是",
            "上表完整模型名，保留 -default 后缀"
          ],
          [
            "contents",
            "array",
            "是",
            "有序消息，role=user"
          ],
          [
            "contents[].parts[].text",
            "string",
            "是",
            "图片提示词"
          ],
          [
            "generationConfig.responseModalities",
            "string[]",
            "否",
            "[\"TEXT\",\"IMAGE\"]"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1beta/models/gemini-3.1-flash-image-preview-default:generateContent' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"contents\": [\n    {\n      \"role\": \"user\",\n      \"parts\": [\n        {\n          \"text\": \"水彩风格的湖畔日出，温暖色调，正方形构图\"\n        }\n      ]\n    }\n  ],\n  \"generationConfig\": {\n    \"responseModalities\": [\n      \"TEXT\",\n      \"IMAGE\"\n    ]\n  }\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "原生响应"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"task_id\": \"task_example\",\n  \"candidates\": [\n    {\n      \"content\": {\n        \"role\": \"model\",\n        \"parts\": [\n          {\n            \"text\": \"生成结果\"\n          },\n          {\n            \"inline_data\": {\n              \"mime_type\": \"image/png\",\n              \"data\": \"<Base64 图片内容>\"\n            }\n          }\n        ]\n      },\n      \"finishReason\": \"STOP\"\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "保留 Gemini 原生 candidates 结构。逐个遍历 content.parts，图片和文字可能位于不同 part。HTTP 202 时保存 id 轮询通用任务。"
      }
    ]
  },
  {
    "id": "gemini-result",
    "group": "Gemini 图片",
    "navTitle": "读取原生图片结果",
    "title": "Gemini · 读取图片结果",
    "subtitle": "解码 inline_data，或用 task_id 查询制品。",
    "blocks": [
      {
        "kind": "table",
        "headers": [
          "字段",
          "说明"
        ],
        "rows": [
          [
            "task_id",
            "本站任务 ID"
          ],
          [
            "candidates[].content.parts[].text",
            "文字说明"
          ],
          [
            "inline_data.data / inlineData.data",
            "Base64 图片"
          ],
          [
            "mime_type / mimeType",
            "实际 MIME"
          ],
          [
            "file_data.file_uri / fileData.fileUri",
            "文件 URI；本站路径需要原 Key 鉴权"
          ]
        ]
      },
      {
        "kind": "code",
        "language": "python",
        "text": "import base64\nfrom pathlib import Path\n\nfor candidate in result.get(\"candidates\", []):\n    for index, part in enumerate(candidate.get(\"content\", {}).get(\"parts\", [])):\n        image = part.get(\"inline_data\") or part.get(\"inlineData\")\n        if image:\n            mime = image.get(\"mime_type\") or image.get(\"mimeType\")\n            suffix = {\"image/png\": \".png\", \"image/jpeg\": \".jpg\", \"image/webp\": \".webp\"}[mime]\n            Path(f\"gemini-{index}{suffix}\").write_bytes(base64.b64decode(image[\"data\"]))",
        "label": "Python · 原生结果解码"
      },
      {
        "kind": "text",
        "text": "也可保存 task_id 后使用本页完整客户端 resume：统一查询本站制品，拼接相对下载路径并携带原 API Key。"
      }
    ]
  },
  {
    "id": "grok-video-submit",
    "group": "Grok 视频",
    "navTitle": "文生视频",
    "title": "Grok · 文生视频",
    "subtitle": "选择时长、分辨率与画幅，提交视频任务。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "grok-imagine-video / grok-imagine-video-1.5"
          ],
          [
            "prompt",
            "string",
            "是",
            "镜头、动作与场景描述"
          ],
          [
            "seconds",
            "integer",
            "否",
            "1–15，默认 8；duration 同义"
          ],
          [
            "resolution",
            "string",
            "否",
            "480p / 720p；1.5 另支持 1080p"
          ],
          [
            "ratio",
            "string",
            "否",
            "1:1 / 16:9 / 9:16 / 4:3 / 3:4 / 3:2 / 2:3"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"grok-imagine-video\",\n  \"prompt\": \"镜头缓慢掠过金色麦田，夕阳逆光，平稳运镜\",\n  \"seconds\": 8,\n  \"resolution\": \"720p\",\n  \"ratio\": \"16:9\"\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"grok-imagine-video\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      }
    ]
  },
  {
    "id": "grok-video-image",
    "group": "Grok 视频",
    "navTitle": "图生视频",
    "title": "Grok · 图生视频",
    "subtitle": "将一张参考图转换为动态镜头，比例沿用输入图。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "grok-imagine-video / grok-imagine-video-1.5"
          ],
          [
            "image",
            "object",
            "JSON 时是",
            "{\"url\":\"公开 URL 或 data URL\"}，最多一张"
          ],
          [
            "input_reference",
            "file",
            "表单时是",
            "PNG / JPEG / WebP；与 JSON image 二选一"
          ],
          [
            "prompt",
            "string",
            "否",
            "建议描述运动与镜头"
          ],
          [
            "seconds",
            "integer",
            "否",
            "1–15，默认 8"
          ],
          [
            "resolution",
            "string",
            "否",
            "480p / 720p；1.5 可用 1080p"
          ]
        ]
      },
      {
        "kind": "text",
        "text": "图生视频省略 ratio，画幅由参考图决定。图片与完整请求受 32 MiB 上限约束。URL 素材必须无需登录即可访问。"
      },
      {
        "kind": "heading",
        "text": "公开 URL 示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"grok-imagine-video\",\n  \"prompt\": \"云朵缓慢移动，镜头轻轻前推\",\n  \"seconds\": 8,\n  \"image\": {\n    \"url\": \"https://code.aicodeme.xyz/your-public-image.png\"\n  }\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "本地文件示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -F 'model=grok-imagine-video' \\\n  -F 'prompt=云朵缓慢移动，镜头轻轻前推' \\\n  -F 'seconds=8' \\\n  -F 'input_reference=@reference.png'",
        "label": "multipart 图生视频"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"grok-imagine-video\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      }
    ]
  },
  {
    "id": "grok-video-result",
    "group": "Grok 视频",
    "navTitle": "查询与取片",
    "title": "Grok · 查询与取片",
    "subtitle": "查询和下载都使用本站任务 ID。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/videos/task_example"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos/task_example' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"grok-imagine-video\",\n  \"status\": \"completed\",\n  \"progress\": 100,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": [\n    {\n      \"key\": \"output-0\",\n      \"type\": \"video\",\n      \"mime_type\": \"video/mp4\",\n      \"size_bytes\": 123456,\n      \"duration_seconds\": \"8\",\n      \"content_url\": \"/v1/tasks/task_example/artifacts/output-0/content\",\n      \"available\": true\n    }\n  ],\n  \"completed_at\": 1791936060\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/videos/task_example/content"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos/task_example/content' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" -D headers.txt -o video.mp4",
        "label": "取片 · cURL"
      },
      {
        "kind": "text",
        "text": "兼容 /v1/videos/generations/{id} 及 /{id}/content。完整费用与制品信息使用 /v1/tasks/{id}。下载返回二进制文件，按 HTTP 状态和 MIME 检查结果。"
      }
    ]
  },
  {
    "id": "omni-video-submit",
    "group": "Omni 视频",
    "navTitle": "文生视频",
    "title": "Omni · 文生视频",
    "subtitle": "文字描述主体、运动与镜头。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos/generations"
      },
      {
        "kind": "table",
        "headers": [
          "模型 ID",
          "时长",
          "分辨率",
          "画幅"
        ],
        "rows": [
          [
            "omni-flash",
            "4 / 6 / 8 / 10 秒，默认 10",
            "720p",
            "16:9"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "omni-flash"
          ],
          [
            "prompt",
            "string",
            "是",
            "画面与运动描述"
          ],
          [
            "seconds",
            "integer",
            "否",
            "4 / 6 / 8 / 10 秒，默认 10"
          ],
          [
            "resolution",
            "string",
            "否",
            "720p"
          ],
          [
            "ratio",
            "string",
            "否",
            "16:9"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"omni-flash\",\n  \"prompt\": \"航拍海岸线，平稳运镜，自然光，电影质感\",\n  \"seconds\": 10,\n  \"resolution\": \"720p\",\n  \"ratio\": \"16:9\"\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"omni-flash\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      }
    ]
  },
  {
    "id": "omni-video-result",
    "group": "Omni 视频",
    "navTitle": "查询与下载",
    "title": "Omni · 查询与下载",
    "subtitle": "跟踪同一任务直到完成。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/tasks/task_example"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/tasks/task_example' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"omni-flash\",\n  \"status\": \"completed\",\n  \"progress\": 100,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": [\n    {\n      \"key\": \"output-0\",\n      \"type\": \"video\",\n      \"mime_type\": \"video/mp4\",\n      \"size_bytes\": 123456,\n      \"duration_seconds\": \"8\",\n      \"content_url\": \"/v1/tasks/task_example/artifacts/output-0/content\",\n      \"available\": true\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/tasks/task_example/artifacts' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "text",
        "text": "读取 artifacts[].content_url，按本站根地址拼接并携带原 API Key 下载。也可使用 /v1/videos/{id}/content 获取首个产物。需要从图片生成视频时，可使用本页 Grok 或 JS / Minimax 参考素材场景。"
      }
    ]
  },
  {
    "id": "veo-text",
    "group": "Veo 视频",
    "navTitle": "文生视频与参数",
    "title": "Veo · 文生视频",
    "subtitle": "文字描述主体、运动与镜头。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos"
      },
      {
        "kind": "table",
        "headers": [
          "模型 ID",
          "时长",
          "分辨率",
          "画幅"
        ],
        "rows": [
          [
            "veo-3.1-fast",
            "固定 8 秒，可省略",
            "720p",
            "16:9"
          ],
          [
            "veo-3.1-lite",
            "固定 8 秒，可省略",
            "720p",
            "16:9"
          ],
          [
            "veo-3.1-quality",
            "固定 8 秒，可省略",
            "720p",
            "16:9"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "veo-3.1-fast / veo-3.1-lite / veo-3.1-quality"
          ],
          [
            "prompt",
            "string",
            "是",
            "画面与运动描述"
          ],
          [
            "seconds",
            "integer",
            "否",
            "固定 8 秒，可省略"
          ],
          [
            "resolution",
            "string",
            "否",
            "720p"
          ],
          [
            "ratio",
            "string",
            "否",
            "16:9"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"veo-3.1-fast\",\n  \"prompt\": \"航拍海岸线，平稳运镜，自然光，电影质感\",\n  \"seconds\": 8,\n  \"resolution\": \"720p\",\n  \"ratio\": \"16:9\"\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"veo-3.1-fast\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      }
    ]
  },
  {
    "id": "veo-result",
    "group": "Veo 视频",
    "navTitle": "查询与下载",
    "title": "Veo · 查询与下载",
    "subtitle": "跟踪同一任务直到完成。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/tasks/task_example"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/tasks/task_example' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"veo-3.1-fast\",\n  \"status\": \"completed\",\n  \"progress\": 100,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": [\n    {\n      \"key\": \"output-0\",\n      \"type\": \"video\",\n      \"mime_type\": \"video/mp4\",\n      \"size_bytes\": 123456,\n      \"duration_seconds\": \"8\",\n      \"content_url\": \"/v1/tasks/task_example/artifacts/output-0/content\",\n      \"available\": true\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/tasks/task_example/artifacts' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "text",
        "text": "读取 artifacts[].content_url，按本站根地址拼接并携带原 API Key 下载。也可使用 /v1/videos/{id}/content 获取首个产物。需要从图片生成视频时，可使用本页 Grok 或 JS / Minimax 参考素材场景。"
      }
    ]
  },
  {
    "id": "other-video",
    "group": "更多视频模型",
    "navTitle": "文字视频与模型差异",
    "title": "其他视频 · 文生视频",
    "subtitle": "逐模型选择时长，保留完整别名和大小写。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos"
      },
      {
        "kind": "table",
        "headers": [
          "模型",
          "时长（秒）",
          "resolution",
          "参考素材"
        ],
        "rows": [
          [
            "MinimaxH3-2K",
            "4–15，默认 15",
            "768p / 2K",
            "图片、视频、音频"
          ],
          [
            "JSadanco2.0",
            "5–15，必须显式 seconds",
            "省略",
            "图片"
          ],
          [
            "doolaa2.0",
            "5–15，必须显式 seconds",
            "省略",
            "文本"
          ],
          [
            "doolaa2.5",
            "5–30，必须显式 seconds",
            "省略",
            "文本"
          ],
          [
            "XXsedanco2.5S",
            "5–30，必须显式 seconds",
            "省略",
            "文本"
          ],
          [
            "TTseadanco2.5-720",
            "5–30，必须显式 seconds",
            "省略",
            "文本"
          ],
          [
            "wan3-1080p1",
            "5–30，必须显式 seconds",
            "省略",
            "文本"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "上表模型 ID"
          ],
          [
            "prompt",
            "string",
            "是",
            "画面与运动描述"
          ],
          [
            "seconds",
            "integer",
            "是",
            "对应模型范围；Minimax 可省略"
          ],
          [
            "resolution",
            "string",
            "否",
            "Minimax 768p / 2K，其余省略"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"doolaa2.5\",\n  \"prompt\": \"城市夜景，缓慢推进镜头，霓虹灯与湿润路面\",\n  \"seconds\": 15\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"doolaa2.5\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      },
      {
        "kind": "text",
        "text": "模型名字的 720 / 1080p / 2K 属于模型 ID，resolution 是否填写按表选择；不要由名字推断参数值。"
      }
    ]
  },
  {
    "id": "video-reference",
    "group": "更多视频模型",
    "navTitle": "参考图与多素材视频",
    "title": "其他视频 · 参考素材",
    "subtitle": "JS 参考图与 Minimax 图片、视频、音频输入。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/videos"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "JSadanco2.0 / MinimaxH3-2K"
          ],
          [
            "prompt",
            "string",
            "是",
            "说明素材用途与生成目标"
          ],
          [
            "seconds",
            "integer",
            "是",
            "JS 5–15，Minimax 4–15"
          ],
          [
            "images",
            "array",
            "否",
            "最多 9 张；公开 URL 字符串或 {url}；JS 也接受 data URL"
          ],
          [
            "videos",
            "array",
            "否",
            "Minimax 最多 3 段；每段最多 30 秒、32 MiB"
          ],
          [
            "audios",
            "array",
            "否",
            "Minimax 最多 3 段；每段最多 30 秒、32 MiB"
          ],
          [
            "duration_seconds",
            "number",
            "视频 / 音频建议",
            "素材 URL 对象中标明时长"
          ],
          [
            "resolution",
            "string",
            "否",
            "Minimax 768p / 2K；JS 省略"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "JS · 参考图"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"JSadanco2.0\",\n  \"prompt\": \"根据参考画面生成平稳电影镜头，人物轻轻转身\",\n  \"seconds\": 10,\n  \"images\": [\n    {\n      \"url\": \"https://code.aicodeme.xyz/your-public-image.png\"\n    }\n  ]\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "Minimax · 多素材"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"MinimaxH3-2K\",\n  \"prompt\": \"参考图片构图、视频运动和音频节奏，生成连贯短片\",\n  \"seconds\": 10,\n  \"resolution\": \"768p\",\n  \"images\": [\n    \"https://code.aicodeme.xyz/your-public-image.png\"\n  ],\n  \"videos\": [\n    {\n      \"url\": \"https://code.aicodeme.xyz/your-public-video.mp4\",\n      \"duration_seconds\": 10\n    }\n  ],\n  \"audios\": [\n    {\n      \"url\": \"https://code.aicodeme.xyz/your-public-audio.mp3\",\n      \"duration_seconds\": 10\n    }\n  ]\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "Minimax · 本地上传"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/videos' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -F 'model=MinimaxH3-2K' \\\n  -F 'prompt=依据参考图片生成短片' \\\n  -F 'seconds=10' \\\n  -F 'resolution=768p' \\\n  -F 'image=@reference.png'",
        "label": "multipart 素材上传"
      },
      {
        "kind": "heading",
        "text": "响应与结果"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"MinimaxH3-2K\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      },
      {
        "kind": "text",
        "text": "素材按数组顺序提交；不要同时使用 images 和 metadata.content 重复同一输入。请求总大小最多 32 MiB，Base64 编码后大小也计入限制。"
      }
    ]
  },
  {
    "id": "audio-chat",
    "group": "Lyria 音乐",
    "navTitle": "Chat Completions",
    "title": "Lyria · Chat Completions",
    "subtitle": "用文字消息生成音乐，支持 JSON 或文字 SSE。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/chat/completions"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "lyria-3-pro / lyria-3.5"
          ],
          [
            "messages",
            "array",
            "是",
            "user / system 文本消息，content 为 string"
          ],
          [
            "duration",
            "integer",
            "否",
            "5–300 秒生成提示；seconds 同义"
          ],
          [
            "stream",
            "boolean",
            "否",
            "false 返回 JSON；true 在生成结束后发送文字 SSE"
          ],
          [
            "async",
            "boolean",
            "否",
            "true 返回任务；省略 / false 同步等待"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "请求示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/chat/completions' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"lyria-3.5\",\n  \"messages\": [\n    {\n      \"role\": \"user\",\n      \"content\": \"生成轻柔爵士音乐，以钢琴为主，温暖低音与舒缓鼓点\"\n    }\n  ],\n  \"duration\": 60,\n  \"stream\": false\n}'",
        "label": "cURL"
      },
      {
        "kind": "heading",
        "text": "响应示例"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"task_id\": \"task_example\",\n  \"object\": \"chat.completion\",\n  \"model\": \"lyria-3.5\",\n  \"choices\": [\n    {\n      \"index\": 0,\n      \"message\": {\n        \"role\": \"assistant\",\n        \"content\": \"音乐生成完成\"\n      },\n      \"finish_reason\": \"stop\"\n    }\n  ],\n  \"data\": [\n    {\n      \"url\": \"/v1/tasks/task_example/artifacts/output-0/content\"\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "优先保存 task_id 查询音乐制品。choices[].message.content 是文字，不是音频。实际 usage 若存在，以返回为准。同步等待可能回退为 HTTP 202 任务 JSON。"
      }
    ]
  },
  {
    "id": "audio-submit",
    "group": "Lyria 音乐",
    "navTitle": "提交音乐任务",
    "title": "Lyria · 提交音乐任务",
    "subtitle": "专用音频接口，用 prompt 描述音乐。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/audio/generations"
      },
      {
        "kind": "heading",
        "text": "请求参数"
      },
      {
        "kind": "table",
        "headers": [
          "参数",
          "类型",
          "必填",
          "说明"
        ],
        "rows": [
          [
            "model",
            "string",
            "是",
            "lyria-3-pro / lyria-3.5"
          ],
          [
            "prompt",
            "string",
            "是",
            "1–10000 字符；描述流派、情绪、乐器、节奏和结构"
          ],
          [
            "duration",
            "integer",
            "否",
            "5–300 秒生成提示；seconds 同义"
          ],
          [
            "title",
            "string",
            "否",
            "最多 160 字符"
          ],
          [
            "lyrics",
            "string",
            "否",
            "最多 20000 字符"
          ],
          [
            "instrumental",
            "boolean",
            "否",
            "true 为纯音乐，同时提交的 lyrics 被忽略"
          ],
          [
            "bpm",
            "integer",
            "否",
            "30–300"
          ],
          [
            "seed",
            "integer",
            "否",
            "0–2147483647"
          ],
          [
            "audio_format",
            "string",
            "否",
            "mp3 / wav / m4a；最终格式看 Content-Type"
          ],
          [
            "n",
            "integer",
            "否",
            "仅 1"
          ],
          [
            "async",
            "boolean",
            "否",
            "true 返回任务；省略 / false 同步等待"
          ]
        ]
      },
      {
        "kind": "heading",
        "text": "异步纯音乐请求"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/audio/generations' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"lyria-3.5\",\n  \"prompt\": \"温暖钢琴与弦乐，宁静舒缓，适合阅读\",\n  \"duration\": 60,\n  \"instrumental\": true,\n  \"async\": true\n}'",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"audio.generation\",\n  \"model\": \"lyria-3.5\",\n  \"status\": \"queued\",\n  \"progress\": 0,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": []\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "返回 HTTP 202 后保存 id，每隔 10–20 秒查询 GET /v1/tasks/{id}。completed 时读取 data[].content_url，携带原 API Key 下载；queued / in_progress 继续等待。failed 查看 diagnostic；unknown 保存任务 ID 并联系支持核对受理结果。不要因客户端超时重复生成。金额和时间仅为格式示例。"
      },
      {
        "kind": "heading",
        "text": "带歌词的同步请求"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/audio/generations' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"lyria-3-pro\",\n  \"prompt\": \"温暖轻快的流行民谣，木吉他与轻鼓点\",\n  \"title\": \"清晨的光\",\n  \"lyrics\": \"晨风轻轻吹过窗边\\n阳光落在你的肩\",\n  \"instrumental\": false,\n  \"duration\": 60,\n  \"n\": 1\n}'",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"created\": 1791936000,\n  \"data\": [\n    {\n      \"url\": \"/v1/tasks/task_example/artifacts/output-0/content\",\n      \"duration\": \"60.12\"\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "text",
        "text": "duration 是时长提示，实际文件时长和结算数量看任务结果。按实际 MIME 选择文件扩展名，不固定保存成 MP3。本页音乐生成不包含语音识别或语音合成。"
      }
    ]
  },
  {
    "id": "audio-compat",
    "group": "Lyria 音乐",
    "navTitle": "兼容路径与接口一览",
    "title": "Lyria · 兼容路径与查询",
    "subtitle": "音乐提交、任务查询和鉴权下载。",
    "blocks": [
      {
        "kind": "endpoint",
        "method": "POST",
        "path": "/v1/music/generations"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/music/generations' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n  \"model\": \"lyria-3-pro\",\n  \"prompt\": \"轻快电子音乐，清晰鼓点，纯音乐\",\n  \"duration\": 30,\n  \"instrumental\": true,\n  \"async\": true\n}'",
        "label": "cURL"
      },
      {
        "kind": "table",
        "headers": [
          "方法",
          "路径",
          "用途"
        ],
        "rows": [
          [
            "POST",
            "/v1/chat/completions",
            "文字消息生成"
          ],
          [
            "POST",
            "/v1/audio/generations",
            "专用音频提交"
          ],
          [
            "POST",
            "/v1/music/generations",
            "兼容音乐提交"
          ],
          [
            "GET",
            "/v1/audio/generations/{id}",
            "音乐状态"
          ],
          [
            "GET",
            "/v1/audio/generations/{id}/content",
            "下载首个音频"
          ],
          [
            "GET",
            "/v1/tasks/{id}/artifacts",
            "完整制品列表"
          ]
        ]
      },
      {
        "kind": "endpoint",
        "method": "GET",
        "path": "/v1/audio/generations/task_example"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/audio/generations/task_example' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\"",
        "label": "cURL"
      },
      {
        "kind": "code",
        "language": "json",
        "text": "{\n  \"id\": \"task_example\",\n  \"object\": \"video\",\n  \"model\": \"lyria-3-pro\",\n  \"status\": \"completed\",\n  \"progress\": 100,\n  \"created_at\": 1791936000,\n  \"billing\": {\n    \"currency\": \"USD\",\n    \"state\": \"held\",\n    \"reserved\": \"0.04\",\n    \"accepted\": \"0.00\",\n    \"settled\": \"0.00\",\n    \"released\": \"0.00\",\n    \"refunded\": \"0.00\",\n    \"maximum_charge\": \"0.04\",\n    \"version\": 1,\n    \"quantities\": {}\n  },\n  \"data\": [\n    {\n      \"key\": \"output-0\",\n      \"type\": \"audio\",\n      \"mime_type\": \"audio/wav\",\n      \"size_bytes\": 123456,\n      \"duration_seconds\": \"30.12\",\n      \"content_url\": \"/v1/tasks/task_example/artifacts/output-0/content\",\n      \"available\": true\n    }\n  ]\n}",
        "label": "JSON 响应示例"
      },
      {
        "kind": "code",
        "language": "bash",
        "text": "curl 'https://code.aicodeme.xyz/v1/audio/generations/task_example/content' \\\n  -H \"Authorization: Bearer $MEDIA_API_KEY\" -D audio-headers.txt -o music.bin",
        "label": "鉴权下载音乐"
      },
      {
        "kind": "text",
        "text": "/v1/music/generations 兼容提交；查询使用 /v1/audio/generations 或通用任务路径。下载 HTTP 200 后查看 Content-Type，再改为对应扩展名。"
      }
    ]
  }
,
  { id: 'audio-stream', group: 'Lyria 音乐', navTitle: '读取文字 SSE', title: 'Lyria · 文字 SSE', subtitle: '按事件读取生成完成后的文字消息。', blocks: [endpoint('POST','/v1/chat/completions'), text('先检查 HTTP 状态与 Content-Type；202 或 application/json 是任务 JSON，保存 id / task_id。text/event-stream 逐事件读取 choices[].delta.content。SSE 是文字；最终音频用任务 ID 下载。'),
    code('bash', `curl -N '${apiOrigin}/v1/chat/completions' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY" -H 'Content-Type: application/json' \\\n  -d '{"model":"lyria-3.5","messages":[{"role":"user","content":"生成舒缓钢琴音乐"}],"stream":true}'`, '音乐文字 SSE'),
    code('javascript', `// 服务端示例：fetch 后先检查状态及 Content-Type。
if (!response.ok) throw new Error('HTTP ' + response.status);
if (response.status === 202 || response.headers.get('content-type')?.includes('application/json')) {
  const task = await response.json(); console.log(task.id || task.task_id);
} else {
  const reader = response.body.getReader(), decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    buffer = buffer.replace(/\\r\\n/g, '\\n');
    let end;
    while ((end = buffer.indexOf('\\n\\n')) >= 0) {
      const event = buffer.slice(0, end); buffer = buffer.slice(end + 2);
      const data = event.split('\\n').filter(l => l.startsWith('data:')).map(l => l.slice(5).trim()).join('\\n');
      if (data && data !== '[DONE]') console.log(JSON.parse(data).choices?.[0]?.delta?.content || '');
    }
    if (done) break;
  }
}`, '按文字事件读取 SSE')
  ] },
  { id: 'tasks', group: '通用任务', navTitle: '状态、账务与下载', title: '通用任务与下载', subtitle: '提交一次，持续跟踪。', blocks: [
    table(['状态','处理方式'], [['queued','已排队，继续查询；内部 submitting 对外也映射为 queued'],['in_progress','生成中；建议间隔 10–20 秒查询'],['completed','读取 data 或制品列表并下载'],['failed','查看 diagnostic 与账务状态，确认失败后再决定新任务'],['unknown','受理结果待核对；保存任务 ID，禁止自动重新生成']]),
    curl('/v1/tasks/task_example'),
    json({...taskResponse,status:'completed',progress:100,completed_at:1791936060,data:[{key:'output-0',type:'video',mime_type:'video/mp4',size_bytes:123456,duration_seconds:'8',content_url:'/v1/tasks/task_example/artifacts/output-0/content',available:true}]}),
    curl('/v1/tasks/task_example/artifacts'), json({task_id:'task_example',artifacts:[{key:'output-0',type:'video',mime_type:'video/mp4',size_bytes:123456,duration_seconds:'8',content_url:'/v1/tasks/task_example/artifacts/output-0/content',available:true}]}),
    code('bash', `curl '${apiOrigin}/v1/tasks/task_example/artifacts/output-0/content' \\\n  -H "Authorization: Bearer $MEDIA_API_KEY" -D headers.txt -o artifact.bin`, '鉴权下载'),
    text('content_url 通常是相对路径，须与根地址拼接；仅向本站发送 Key，勿向其他域名转发认证头。下载不是公开直链，使用创建任务的原 Key；用户或 Key 不匹配不能访问。根据 Content-Type 决定 .png / .mp4 / .mp3 / .wav 等扩展名，勿固定保存成 MP3。available:false 或 HTTP 410 表示已失效。视频兼容 GET /v1/videos/{id} 与 /v1/videos/{id}/content。'),
    table(['账务字段','说明'], [['reserved / maximum_charge','预留额度 / 最大可能收费'],['accepted / settled','受理费 / 已结算费用'],['released / refunded','释放预留 / 已退款金额'],['state','held、pending_usage、settled、released、manual_review'],['quantities / calculation','计费数量与明细（结算后可用）'],['multiplier / time_multiplier / price_version','媒体倍率 / 分时倍率 / 锁定价格版本'],['version','管理员账务操作的乐观锁版本']]),
    text('预留不代表已消费；净费用通常为 accepted + settled − refunded。模型返回完成状态也需同时检查 billing.state；缺少真实用量时可能等待结算或进入人工核对，不要仅凭前端状态推断账务完成。')
  ] },
  { id: 'errors', group: '通用任务', navTitle: '估价与错误处理', title: '估价与错误处理', subtitle: '先确认参数，再创建任务。', blocks: [
    curl('/v1/sub2api/estimates', {operation:'video',request:{model:'grok-imagine-video',prompt:'海边日落',seconds:8}}),
    json({currency:'USD',calculation:{currency:'USD',total:'0.00',uncapped:'0.00',items:[],missing_usage:[]},maximum_charge:'0.00',price_version:'example',locks_price:false}),
    text('估价只做能力校验与费用计算，不发起生成。operation 使用 image、image_edit、gemini_image、video、audio 或 chat_music；request 为对应提交参数。真实输出时长、token 和账务明细可能使最终结算与预估不同。'),
    table(['HTTP / 错误','建议'], [['400','参数范围、冲突字段、素材格式或必填 seconds；修正请求'],['401 / 403','核对 Key、分组、模型权限及任务归属'],['409','幂等参数冲突；管理员版本冲突时刷新后核对'],['413','请求或素材过大'],['422','当前 Key 无法执行该请求；检查模型、分组和参数后联系支持'],['429','并发或额度限制；GET 可等待后再查，POST 勿盲重试'],['502 / 503 / 504 / 网络超时','可能已经受理，先用已有任务 ID 查询'],['410','制品已失效']]),
    json({error:{type:'unsupported_capability',message:'Unsupported resolution'}}, '错误结构示意'),
    text('一般模型单能力并发为 2、观察窗口约 3600 秒，最终以分组配置为准。可使用 Idempotency-Key 固定同一提交；相同 Key 绑定的请求参数必须一致。客户端超时不取消任务。提交前保存本地状态文件，提交成功保存 id / task_id；中断后只恢复查询，绝不自动再 POST。')
  ] },
  { id: 'clients', group: '通用任务', navTitle: 'Python / JavaScript', title: '完整客户端示例', subtitle: 'Python 与 JavaScript。', blocks: [
    text('Python 3.10+ 仅用标准库；JavaScript 使用 Node.js 20+，在服务端运行。将上方任一 JSON 请求保存为 request.json。GPT 编辑使用 --image 上传本地图片，可重复两次；文字参数另存为 edit.json。脚本分为 submit 与 resume：每个状态文件只提交一次；resume 完成轮询与制品下载。异步响应、同步成功及同步回退 202 均通过任务 ID 汇合到同一流程。脚本不消费 Chat SSE，使用 stream:false。'),
    code('bash', `# 设置 MEDIA_API_KEY 后执行；不把密钥写入脚本。
python imagevideo.py submit --endpoint /v1/videos --payload request.json --state task.json
python imagevideo.py resume --state task.json --output results

node imagevideo.mjs submit --endpoint /v1/audio/generations --payload request.json --state music.json
node imagevideo.mjs resume --state music.json --output music-results

# GPT 图生图：edit.json 只填 model、prompt、n 等文字参数。
python imagevideo.py submit --endpoint /v1/images/edits --payload edit.json --image scene.png --image subject.png --state edit-task.json
node imagevideo.mjs submit --endpoint /v1/images/edits --payload edit.json --image reference.png --state edit-task-node.json

# 图片用 /v1/images/generations；Gemini 用完整 /v1beta/models/{model}:generateContent。
# 恢复查询用原状态文件；新的生成必须使用新的状态文件。`, '运行方式'),
    code('python',pythonClient,'imagevideo.py'), code('javascript',javascriptClient,'imagevideo.mjs')
  ] }
]

export function sectionMarkdown(section: DocSection): string {
  return `## ${section.title}\n\n${section.subtitle}\n\n` + section.blocks.map(block => {
    if (block.kind === 'text') return block.text
    if (block.kind === 'heading') return `### ${block.text}`
    if (block.kind === 'endpoint') return `**${block.method}** ${apiOrigin}${block.path}`
    if (block.kind === 'code') return `### ${block.label}\n\n\`\`\`${block.language}\n${block.text}\n\`\`\``
    const row = (cells: string[]) => '| ' + cells.map(cell => cell.replace(/\|/g, '\\|').replace(/\n/g, '<br>')).join(' | ') + ' |'
    return [row(block.headers),row(block.headers.map(() => '---')),...block.rows.map(row)].join('\n')
  }).join('\n\n')
}
export const imagevideoMarkdown = () => `# ${docsTitle}\n\n版本：${docsVersion}\n\n根地址：${apiOrigin}\n\n` + imagevideoSections.map(sectionMarkdown).join('\n\n')

export const imagevideoGroups = [...new Set(imagevideoSections.map(s => s.group))].map(title => ({title,sections:imagevideoSections.filter(s => s.group === title)}))
export const groupMarkdown = (title: string) => imagevideoSections.filter(s => s.group === title).map(sectionMarkdown).join('\n\n')

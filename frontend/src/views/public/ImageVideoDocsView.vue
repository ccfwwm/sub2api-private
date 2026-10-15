<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, nextTick } from 'vue'
import { apiOrigin, docsTitle, docsVersion, sectionMarkdown } from '@/data/imagevideoDocs'

const menuOpen = ref(false), active = ref('start'), notice = ref('')
const dark = ref(false)
// Public pages render only the published database version.  The bundled
// content is used by the seed/editor build, never as a public fallback.
const imagevideoSections = ref([] as import('@/data/imagevideoDocs').DocSection[])
const imagevideoGroups = computed(() => [...new Set(imagevideoSections.value.map(s => s.group))].map(title => ({ title, sections: imagevideoSections.value.filter(s => s.group === title) })))
const publishedVersion = ref('')
const documentLoading = ref(true), documentError = ref(false)
const dynamicMarkdown = () => `# ${docsTitle}\n\n版本：${publishedVersion.value || docsVersion}\n\n根地址：${apiOrigin}\n\n` + imagevideoSections.value.map(sectionMarkdown).join('\n\n')
const dynamicGroupMarkdown = (title: string) => imagevideoSections.value.filter(s => s.group === title).map(sectionMarkdown).join('\n\n')
function toggleTheme() {
  dark.value = !dark.value
  try { localStorage.setItem('media-docs-theme', dark.value ? 'dark' : 'light') } catch { /* Storage may be unavailable. */ }
}
let observer: IntersectionObserver | undefined, timer: ReturnType<typeof setTimeout> | undefined
async function copy(value: string) {
  try { await navigator.clipboard.writeText(value); notice.value = '已复制' }
  catch { notice.value = '复制失败，请选择内容手动复制' }
  clearTimeout(timer); timer = setTimeout(() => { notice.value = '' }, 2500)
}
onMounted(() => {
  try { dark.value = localStorage.getItem('media-docs-theme') === 'dark' } catch { /* Default to light. */ }
  observer = new IntersectionObserver(entries => {
    const visible = entries.find(entry => entry.isIntersecting)
    if (visible) active.value = visible.target.id
  }, { rootMargin: '-5% 0px -75% 0px' })
  fetch('/api/v1/public-docs/imagevideo').then(response => response.ok ? response.json() : null).then(payload => {
    const content = payload?.data?.content
    if (Array.isArray(content) && content.length) {
      imagevideoSections.value = content
      publishedVersion.value = `v${payload.data.version}`
    } else documentError.value = true
  }).catch(() => { documentError.value = true }).finally(() => { documentLoading.value = false })
})
watch(imagevideoSections, async () => { await nextTick(); document.querySelectorAll('.media-doc-section').forEach(section => observer?.observe(section)) })
onUnmounted(() => { observer?.disconnect(); clearTimeout(timer) })
</script>

<template>
  <div class="media-docs" :class="{ 'docs-dark': dark }">
    <header class="media-doc-mobile">
      <router-link to="/">词元互联</router-link>
      <button :aria-expanded="menuOpen" aria-controls="media-doc-nav" @click="menuOpen = !menuOpen">{{ menuOpen ? '关闭目录 ×' : '目录 ☰' }}</button>
    </header>
    <button v-if="menuOpen" class="media-doc-mask" aria-label="关闭目录" @click="menuOpen = false" />
    <aside id="media-doc-nav" class="media-doc-sidebar" :class="{ open: menuOpen }">
      <router-link to="/" class="media-doc-brand"><img src="/logo.svg" alt="" />词元互联<span>↗</span></router-link>
      <p class="media-doc-eyebrow">MEDIA API / DOCUMENTATION</p>
      <nav aria-label="文档目录">
        <div v-for="group in imagevideoGroups" :key="group.title" class="media-doc-nav-group">
          <b>{{ group.title }}</b>
          <a v-for="section in group.sections" :key="section.id" :href="`#${section.id}`" :class="{ active: active === section.id }" @click="menuOpen = false; active = section.id">{{ section.navTitle }}</a>
        </div>
      </nav>
      <div class="media-doc-nav-footer"><p>API BASE</p><code>{{ apiOrigin }}</code><p>版本 {{ publishedVersion || docsVersion }} · 26 个模型</p><router-link to="/keys">管理 API Key ↗</router-link></div>
    </aside>
    <main class="media-doc-main">
      <div class="media-doc-top"><span>IMAGEVIDEO · 开发者指南</span><div class="media-doc-actions"><button :aria-pressed="dark" aria-label="切换文档主题" @click="toggleTheme">{{ dark ? '☀ 浅色' : '☾ 深色' }}</button><button @click="copy(dynamicMarkdown())">复制完整 Markdown ↗</button></div></div>
      <div class="media-doc-hero">
        <p class="media-doc-eyebrow">CREATE WITH ONE API</p>
        <h1 :aria-label="docsTitle"><span>词元互联</span>图片、视频与音乐接口文档</h1>
        <p class="media-doc-lead">每种调用场景独立列出接口地址、请求参数、示例与响应。</p>
        <div class="media-doc-tags"><span>图片生成</span><span>视频创作</span><span>音乐生成</span></div>
        <div class="media-doc-origin"><span>统一接口地址</span><code>{{ apiOrigin }}</code><button @click="copy(apiOrigin)">复制</button></div>
        <div class="media-doc-copy-guide"><button @click="copy(dynamicMarkdown())">一键复制全文</button><p>复制后发送给 AI，并描述你想生成的图片、视频或音乐，即可协助编写请求、检查参数和读取结果。</p></div>
      </div>
      <div v-if="documentLoading" class="media-doc-section">正在读取已发布的接口文档…</div>
      <div v-else-if="documentError" class="media-doc-section">文档暂时无法加载，请稍后刷新。</div>
      <section v-for="(section,index) in imagevideoSections" :id="section.id" :key="section.id" class="media-doc-section">
        <div v-if="index === 0 || imagevideoSections[index-1].group !== section.group" class="media-doc-product"><strong>{{ section.group }}</strong><button @click="copy(dynamicGroupMarkdown(section.group))">复制本组 Markdown</button></div>
        <div class="media-doc-section-heading"><div><p class="media-doc-eyebrow">{{ String(index+1).padStart(2,'0') }} / API GUIDE</p><h2>{{ section.title }}</h2><p class="media-doc-subtitle">{{ section.subtitle }}</p></div><button @click="copy(sectionMarkdown(section))">复制为 Markdown ↗</button></div>
        <template v-for="(block,blockIndex) in section.blocks" :key="blockIndex">
          <h3 v-if="block.kind === 'heading'" class="media-doc-block-heading">{{ block.text }}</h3>
          <div v-else-if="block.kind === 'endpoint'" class="media-doc-endpoint"><b>{{ block.method }}</b><code>{{ apiOrigin }}{{ block.path }}</code><button @click="copy(apiOrigin + block.path)">复制地址</button></div>
          <p v-else-if="block.kind === 'text'" class="media-doc-paragraph">{{ block.text }}</p>
          <div v-else-if="block.kind === 'table'" class="media-doc-table-wrap"><table><thead><tr><th v-for="header in block.headers" :key="header" scope="col">{{ header }}</th></tr></thead><tbody><tr v-for="(row,rowIndex) in block.rows" :key="rowIndex"><td v-for="(cell,cellIndex) in row" :key="cellIndex" :class="{ 'media-doc-model': cellIndex === 0 && section.id === 'start' && block.headers[0] === '模型 ID' }">{{ cell }}</td></tr></tbody></table></div>
          <div v-else class="media-doc-code"><div><span>{{ block.label }}</span><button @click="copy(block.text)">复制代码</button></div><pre :aria-label="block.label"><code>{{ block.text }}</code></pre></div>
        </template>
        <div v-if="section.id === 'clients'" class="media-doc-downloads"><a href="/examples/imagevideo.py" download>下载 Python 示例 ↓</a><a href="/examples/imagevideo.mjs" download>下载 JavaScript 示例 ↓</a></div>
      </section>
      <footer class="media-doc-footer">词元互联 · {{ publishedVersion || docsVersion }} <a href="#start">返回顶部 ↑</a></footer>
    </main>
    <div v-if="notice" class="media-doc-notice" role="status">{{ notice }}</div>
  </div>
</template>

<style scoped>
.media-docs{--bg:#fafbf8;--panel:#fff;--code:#f3f5f0;--ink:#263027;--muted:#687465;--accent:#496815;--accent-bg:#eff5df;--line:#dce3d6;--model:#9b5c20;background:var(--bg);color:var(--ink);min-height:100vh;font-family:"PingFang SC","Microsoft YaHei",sans-serif;font-size:14px;line-height:1.8}
.media-docs.docs-dark{--bg:#101310;--panel:#171c16;--code:#141a12;--ink:#e9eee1;--muted:#a0ab95;--accent:#d5fa42;--accent-bg:#202915;--line:#34402c;--model:#fbb76d;color-scheme:dark}
.media-docs button,.media-docs a{transition:color .15s,background .15s}.media-docs button:hover,.media-docs a:hover{color:var(--accent)}.media-docs button:focus-visible,.media-docs a:focus-visible{outline:2px solid var(--accent);outline-offset:4px}
.media-doc-sidebar{position:fixed;inset:0 auto 0 0;width:268px;padding:30px 24px 22px;border-right:1px solid var(--line);overflow:auto;display:flex;flex-direction:column;background:var(--bg);z-index:40}.media-doc-brand{display:flex;align-items:center;gap:10px;font-size:21px;font-weight:bold;margin-bottom:28px}.media-doc-brand img{width:28px;height:28px}.media-doc-brand span{margin-left:auto;color:var(--accent)}.media-doc-eyebrow{color:var(--muted);letter-spacing:.1em;font-size:11px;margin-bottom:12px}.media-doc-nav-group{margin:14px 0}.media-doc-nav-group b{display:block;color:var(--ink);font-size:12px;margin-bottom:5px}.media-doc-sidebar nav a{display:block;padding:6px 12px;border-left:2px solid var(--line);color:var(--muted);font-size:12px}.media-doc-sidebar nav a.active{border-color:var(--accent);color:var(--accent);background:var(--accent-bg)}.media-doc-nav-footer{margin-top:auto;padding-top:24px;font-size:11px;color:var(--muted)}.media-doc-nav-footer code{word-break:break-all;color:var(--accent)}.media-doc-nav-footer p{margin:10px 0}.media-doc-nav-footer a{display:block;margin-top:16px}
.media-doc-main{margin-left:268px;padding:0 56px;max-width:1420px}.media-doc-top{display:flex;justify-content:space-between;align-items:center;gap:20px;padding:22px 0;border-bottom:1px solid var(--line);font-size:11px;color:var(--muted)}.media-doc-actions{display:flex;gap:22px;color:var(--accent)}.media-doc-hero{padding:54px 0 48px}.media-doc-hero h1{font-family:Georgia,"Songti SC",SimSun,serif;font-size:clamp(34px,3.7vw,54px);font-weight:700;letter-spacing:-.02em;line-height:1.4;max-width:860px}.media-doc-hero h1 span{display:block}.media-doc-lead{color:var(--muted);font-size:16px;margin:22px 0}.media-doc-tags{display:flex;gap:8px;flex-wrap:wrap;margin:20px 0}.media-doc-tags span{color:var(--accent);border:1px solid var(--line);background:var(--accent-bg);padding:3px 12px;font-size:12px;border-radius:4px}.media-doc-origin{display:flex;align-items:center;gap:20px;padding:18px 22px;border:1px solid var(--line);background:var(--panel);flex-wrap:wrap;border-radius:6px}.media-doc-origin span{color:var(--muted);font-size:12px}.media-doc-origin code{color:var(--accent);overflow-wrap:anywhere}.media-doc-origin button{margin-left:auto;font-size:12px}.media-doc-copy-guide{display:flex;gap:18px;align-items:center;padding:18px 0}.media-doc-copy-guide button{flex-shrink:0;background:var(--accent);color:var(--bg);padding:9px 16px;border-radius:4px;font-weight:600}.media-doc-copy-guide p{color:var(--muted);font-size:12px}
.media-doc-section{padding:38px 0;border-top:1px solid var(--line);scroll-margin-top:26px}.media-doc-product{display:flex;justify-content:space-between;gap:20px;align-items:center;margin:0 0 30px;padding:15px 18px;background:var(--accent-bg);color:var(--accent);border-radius:6px}.media-doc-product strong{font-size:14px}.media-doc-product button{font-size:11px}.media-doc-section-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:20px;margin-bottom:24px}.media-doc-section-heading h2{font-family:Georgia,"Songti SC",SimSun,serif;font-size:32px;font-weight:bold;line-height:1.4}.media-doc-section-heading button{font-size:11px;color:var(--accent);padding-top:6px;white-space:nowrap}.media-doc-subtitle{color:var(--muted);margin-top:10px}.media-doc-paragraph{color:var(--muted);margin:20px 0;line-height:1.95;overflow-wrap:anywhere}.media-doc-block-heading{font-size:14px;font-weight:700;color:var(--accent);margin:28px 0 10px}.media-doc-endpoint{display:flex;align-items:flex-start;gap:14px;padding:14px 18px;border:1px solid var(--line);background:var(--panel);border-radius:5px;min-width:0}.media-doc-endpoint b{color:var(--accent);font:700 12px/1.8 Consolas,monospace;flex-shrink:0}.media-doc-endpoint code{overflow-wrap:anywhere;min-width:0;font-size:12px}.media-doc-endpoint button{margin-left:auto;flex-shrink:0;font-size:11px;color:var(--accent)}
.media-doc-table-wrap{max-width:100%;overflow:auto;margin:18px 0;border:1px solid var(--line);border-radius:5px}.media-doc-table-wrap table{width:100%;border-collapse:collapse;text-align:left;font-size:12px;min-width:540px}.media-doc-table-wrap th{color:var(--accent);background:var(--accent-bg);font-weight:600;padding:12px 15px}.media-doc-table-wrap td{border-top:1px solid var(--line);padding:12px 15px;vertical-align:top;color:var(--muted);overflow-wrap:anywhere;background:var(--panel)}.media-doc-table-wrap td:first-child{color:var(--ink);font-family:Consolas,monospace}.media-doc-code{border:1px solid var(--line);background:var(--code);margin:18px 0;min-width:0;border-radius:6px;overflow:hidden}.media-doc-code>div{padding:9px 16px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;gap:12px;font-size:11px;color:var(--muted)}.media-doc-code button{color:var(--accent)}.media-doc-code pre{padding:18px;overflow:auto;max-height:560px;font-size:12px;line-height:1.8;color:var(--ink);tab-size:2;margin:0}.media-doc-code code{font-family:Consolas,monospace}.media-doc-downloads{display:flex;gap:20px;flex-wrap:wrap;color:var(--accent)}.media-doc-footer{border-top:1px solid var(--line);padding:28px 0;color:var(--muted);font-size:12px;display:flex;justify-content:space-between}.media-doc-notice{position:fixed;bottom:28px;left:50%;transform:translateX(-50%);padding:10px 24px;background:var(--accent);color:var(--bg);z-index:60;border-radius:5px;box-shadow:0 8px 30px #0003}.media-doc-mobile,.media-doc-mask{display:none}
@media(min-width:1600px){.media-doc-main{padding-left:86px;padding-right:86px}}@media(max-width:900px){.media-doc-main{margin-left:0;padding:60px 20px 0}.media-doc-sidebar{transform:translateX(-100%);transition:transform .2s}.media-doc-sidebar.open{transform:translateX(0)}.media-doc-mobile{display:flex;position:fixed;top:0;left:0;right:0;z-index:35;justify-content:space-between;align-items:center;background:var(--bg);border-bottom:1px solid var(--line);padding:16px 20px;color:var(--accent)}.media-doc-mask{display:block;position:fixed;inset:0;background:#0008;z-index:38}.media-doc-hero{padding:34px 0}.media-doc-top{font-size:10px;align-items:flex-start}.media-doc-actions{gap:12px;flex-wrap:wrap;justify-content:flex-end}.media-doc-section-heading{flex-wrap:wrap}.media-doc-section-heading h2{font-size:27px}.media-doc-section{scroll-margin-top:90px}.media-doc-origin{padding:14px;gap:10px}.media-doc-code pre{padding:14px}.media-doc-hero h1{font-size:34px}.media-doc-copy-guide{align-items:flex-start;flex-direction:column;gap:10px}.media-doc-endpoint{flex-wrap:wrap}.media-doc-endpoint code{flex:1}.media-doc-endpoint button{margin-left:0}}
</style>

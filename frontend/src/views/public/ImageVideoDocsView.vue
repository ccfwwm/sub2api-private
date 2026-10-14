<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { apiOrigin, docsTitle, docsVersion, imagevideoSections, imagevideoMarkdown, sectionMarkdown } from '@/data/imagevideoDocs'

const menuOpen = ref(false), active = ref('start'), notice = ref('')
let observer: IntersectionObserver | undefined, timer: ReturnType<typeof setTimeout> | undefined
async function copy(value: string) {
  try { await navigator.clipboard.writeText(value); notice.value = '已复制' }
  catch { notice.value = '复制失败，请选择内容手动复制' }
  clearTimeout(timer); timer = setTimeout(() => { notice.value = '' }, 2500)
}
onMounted(() => {
  observer = new IntersectionObserver(entries => {
    const visible = entries.find(entry => entry.isIntersecting)
    if (visible) active.value = visible.target.id
  }, { rootMargin: '-5% 0px -75% 0px' })
  document.querySelectorAll('.media-doc-section').forEach(section => observer?.observe(section))
})
onUnmounted(() => { observer?.disconnect(); clearTimeout(timer) })
</script>

<template>
  <div class="media-docs">
    <header class="media-doc-mobile">
      <router-link to="/">词元互联</router-link>
      <button :aria-expanded="menuOpen" aria-controls="media-doc-nav" @click="menuOpen = !menuOpen">{{ menuOpen ? '关闭目录 ×' : '目录 ☰' }}</button>
    </header>
    <button v-if="menuOpen" class="media-doc-mask" aria-label="关闭目录" @click="menuOpen = false" />
    <aside id="media-doc-nav" class="media-doc-sidebar" :class="{ open: menuOpen }">
      <router-link to="/" class="media-doc-brand"><img src="/logo.svg" alt="" />词元互联<span>↗</span></router-link>
      <p class="media-doc-eyebrow">MEDIA API / DOCUMENTATION</p>
      <nav aria-label="文档目录">
        <a v-for="(section,index) in imagevideoSections" :key="section.id" :href="`#${section.id}`" :class="{ active: active === section.id }" @click="menuOpen = false; active = section.id"><span>{{ String(index+1).padStart(2,'0') }}</span>{{ section.title }}</a>
      </nav>
      <div class="media-doc-nav-footer"><p>API BASE</p><code>{{ apiOrigin }}</code><p>版本 {{ docsVersion }} · 26 个模型</p><router-link to="/keys">管理 API Key ↗</router-link></div>
    </aside>
    <main class="media-doc-main">
      <div class="media-doc-top"><span>IMAGEVIDEO · 开发者指南</span><button @click="copy(imagevideoMarkdown())">复制完整 Markdown ↗</button></div>
      <div class="media-doc-hero">
        <p class="media-doc-eyebrow">CREATE WITH ONE API</p>
        <h1 :aria-label="docsTitle"><span>词元互联</span>图片、视频与音乐接口文档</h1>
        <p class="media-doc-lead">让灵感成为画面、影像与旋律。</p>
        <div class="media-doc-tags"><span>图片生成</span><span>视频创作</span><span>音乐生成</span></div>
        <div class="media-doc-origin"><span>统一接口地址</span><code>{{ apiOrigin }}</code><button @click="copy(apiOrigin)">复制</button></div>
      </div>
      <section v-for="(section,index) in imagevideoSections" :id="section.id" :key="section.id" class="media-doc-section">
        <div class="media-doc-section-heading"><div><p class="media-doc-eyebrow">{{ String(index+1).padStart(2,'0') }} / API GUIDE</p><h2>{{ section.title }}</h2><p class="media-doc-subtitle">{{ section.subtitle }}</p></div><button @click="copy(sectionMarkdown(section))">复制为 Markdown ↗</button></div>
        <template v-for="(block,blockIndex) in section.blocks" :key="blockIndex">
          <p v-if="block.kind === 'text'" class="media-doc-paragraph">{{ block.text }}</p>
          <div v-else-if="block.kind === 'table'" class="media-doc-table-wrap"><table><thead><tr><th v-for="header in block.headers" :key="header" scope="col">{{ header }}</th></tr></thead><tbody><tr v-for="(row,rowIndex) in block.rows" :key="rowIndex"><td v-for="(cell,cellIndex) in row" :key="cellIndex" :class="{ 'media-doc-model': cellIndex === 0 && section.id === 'start' && block.headers[0] === '模型 ID' }">{{ cell }}</td></tr></tbody></table></div>
          <div v-else class="media-doc-code"><div><span>{{ block.label }}</span><button @click="copy(block.text)">复制代码</button></div><pre :aria-label="block.label"><code>{{ block.text }}</code></pre></div>
        </template>
        <div v-if="section.id === 'clients'" class="media-doc-downloads"><a href="/examples/imagevideo.py" download>下载 Python 示例 ↓</a><a href="/examples/imagevideo.mjs" download>下载 JavaScript 示例 ↓</a></div>
      </section>
      <footer class="media-doc-footer">词元互联 · {{ docsVersion }} <a href="#start">返回顶部 ↑</a></footer>
    </main>
    <div v-if="notice" class="media-doc-notice" role="status">{{ notice }}</div>
  </div>
</template>

<style scoped>
.media-docs{--accent:#d5fa42;background:#0c0d0d;color:#efeee8;min-height:100vh;font-family:Consolas,"PingFang SC","Microsoft YaHei",monospace;font-size:14px;line-height:1.8}
.media-docs button,.media-docs a{transition:color .15s,background .15s}
.media-docs button:hover,.media-docs a:hover{color:var(--accent)}
.media-docs button:focus-visible,.media-docs a:focus-visible{outline:2px solid var(--accent);outline-offset:4px}
.media-doc-sidebar{position:fixed;inset:0 auto 0 0;width:270px;padding:38px 26px 24px;border-right:1px solid #292b25;overflow:auto;display:flex;flex-direction:column;background:#0c0d0d;z-index:40}
.media-doc-brand{display:flex;align-items:center;gap:10px;font-size:20px;font-weight:bold;margin-bottom:36px}.media-doc-brand img{width:28px;height:28px}.media-doc-brand span{margin-left:auto;color:var(--accent)}
.media-doc-eyebrow{color:#9c9f91;letter-spacing:.13em;font-size:11px;margin-bottom:16px}.media-doc-sidebar nav{display:flex;flex-direction:column;gap:3px}.media-doc-sidebar nav a{padding:10px 12px;display:flex;gap:16px;border-left:2px solid transparent;color:#b8baae}.media-doc-sidebar nav a span{font-size:11px;color:#797d6a}.media-doc-sidebar nav a.active{border-color:var(--accent);color:var(--accent);background:#171c0e}
.media-doc-nav-footer{margin-top:auto;padding-top:36px;font-size:11px;color:#939787}.media-doc-nav-footer code{word-break:break-all;color:#d5fa42}.media-doc-nav-footer p{margin:12px 0}.media-doc-nav-footer a{display:block;margin-top:20px}
.media-doc-main{margin-left:270px;padding:0 60px;max-width:1500px}.media-doc-top{display:flex;justify-content:space-between;gap:20px;padding:25px 0;border-bottom:1px solid #292b25;font-size:11px;color:#9c9f91}.media-doc-top button{color:var(--accent)}
.media-doc-hero{padding:70px 0 60px}.media-doc-hero h1{font-family:"Noto Serif SC","Songti SC",SimSun,serif;font-size:clamp(36px,4.5vw,66px);font-weight:700;letter-spacing:-.02em;line-height:1.3;max-width:850px}.media-doc-hero h1 span{display:block}.media-doc-lead{color:#aaad9f;font-size:18px;margin:24px 0}.media-doc-tags{display:flex;gap:8px;flex-wrap:wrap;margin:24px 0}.media-doc-tags span{color:var(--accent);border:1px solid #4a5524;padding:3px 12px;font-size:12px}.media-doc-origin{display:flex;align-items:center;gap:20px;padding:20px 24px;border:1px solid #414a23;background:#14180c;flex-wrap:wrap}.media-doc-origin span{color:#9da389;font-size:12px}.media-doc-origin code{color:var(--accent);overflow-wrap:anywhere}.media-doc-origin button{margin-left:auto;font-size:12px}
.media-doc-section{padding:48px 0;border-top:1px solid #292b25;scroll-margin-top:32px}.media-doc-section-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:20px;margin-bottom:30px}.media-doc-section-heading h2{font-family:SimSun,"Songti SC",serif;font-size:36px;font-weight:bold;line-height:1.4}.media-doc-section-heading button{font-size:11px;color:var(--accent);padding-top:6px;white-space:nowrap}.media-doc-subtitle{color:#939787;margin-top:12px}.media-doc-paragraph{color:#b9bcb0;margin:22px 0;line-height:2;overflow-wrap:anywhere}
.media-doc-table-wrap{max-width:100%;overflow:auto;margin:24px 0;border:1px solid #303329}.media-doc-table-wrap table{width:100%;border-collapse:collapse;text-align:left;font-size:12px;min-width:520px}.media-doc-table-wrap th{color:var(--accent);background:#171b10;font-weight:normal;padding:13px 16px}.media-doc-table-wrap td{border-top:1px solid #292b25;padding:13px 16px;vertical-align:top;color:#bfc3b5;overflow-wrap:anywhere}.media-doc-table-wrap td:first-child{color:#e4e7da}.media-doc-table-wrap .media-doc-model{color:#fbb76d;font-family:Consolas,monospace}
.media-doc-code{border:1px solid #303329;background:#111410;margin:24px 0;min-width:0}.media-doc-code>div{padding:10px 18px;border-bottom:1px solid #292b25;display:flex;justify-content:space-between;gap:12px;font-size:11px;color:#9ea68e}.media-doc-code button{color:var(--accent)}.media-doc-code pre{padding:22px;overflow:auto;max-height:580px;font-size:12px;line-height:1.9;color:#dde7c2;tab-size:2}.media-doc-code code{font-family:Consolas,monospace}
.media-doc-downloads{display:flex;gap:20px;flex-wrap:wrap;color:var(--accent)}.media-doc-footer{border-top:1px solid #292b25;padding:32px 0;color:#898d7e;font-size:12px;display:flex;justify-content:space-between}.media-doc-notice{position:fixed;bottom:28px;left:50%;transform:translateX(-50%);padding:10px 24px;background:var(--accent);color:#101408;z-index:60;box-shadow:0 8px 30px #0008}.media-doc-mobile,.media-doc-mask{display:none}
@media(min-width:1600px){.media-doc-main{padding-left:100px;padding-right:100px}}
@media(max-width:900px){.media-doc-main{margin-left:0;padding:0 22px;padding-top:60px}.media-doc-sidebar{transform:translateX(-100%);transition:transform .2s}.media-doc-sidebar.open{transform:translateX(0)}.media-doc-mobile{display:flex;position:fixed;top:0;left:0;right:0;z-index:35;justify-content:space-between;align-items:center;background:#0c0d0df5;border-bottom:1px solid #292b25;padding:16px 22px;color:var(--accent)}.media-doc-mask{display:block;position:fixed;inset:0;background:#000a;z-index:38}.media-doc-hero{padding:42px 0}.media-doc-top{font-size:10px;align-items:center}.media-doc-section-heading{flex-wrap:wrap}.media-doc-section-heading h2{font-size:30px}.media-doc-section{scroll-margin-top:90px}.media-doc-origin{padding:16px;gap:12px}.media-doc-code pre{padding:16px}.media-doc-hero h1{font-size:38px}}
</style>

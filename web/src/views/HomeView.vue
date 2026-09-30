<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import i18n from '../i18n'

const { locale } = useI18n()

const L = computed(() => {
  const loc = locale.value as string
  const key = loc === 'en-US' ? 'en-US' : 'zh-CN'
  const msgs = i18n.global.messages.value as unknown as Record<string, HomeCopy>
  return msgs[key]
})

interface HomeCopy {
  home: {
    badge: string
    heroTitle1: string
    heroTitle2: string
    heroSub: string
    ctaStart: string
    ctaPricing: string
    ctaModels: string
    appsLabel: string
    apps: string[]
    stats: { num: string; label: string }[]
    demoTitle: string
    demoTabs: string[]
    demoStatus: string
    bento: { title: string; desc: string }[]
    ctaTitle: string
    ctaSub: string
    ctaBtn: string
    ctaDocs: string
  }
}

const demoTab = ref(0)

const demoChat = {
  endpoint: 'POST /v1/chat/completions',
  code: [
    ['k', 'curl'], [' ', ' https://api.kejiapi.dev/v1/chat/completions \\'],
    [' ', '  -H "Authorization: Bearer sk-keji-****" \\'],
    [' ', '  -H "Content-Type: application/json" \\'],
    [' ', '  -d \'{'],
    [' ', '    "model": "gpt-4o",'],
    ['s', '    "messages": [{"role": "user", "content": "Hello!"}]'],
    [' ', '  }\''],
  ] as [string, string][],
  resp: [
    ['k', '{ "id": "chatcmpl-8a2f…", "object": "chat.completion"'],
    ['k', ',  "model": "gpt-4o"'],
    ['k', ',  "choices": [{"index": 0'],
    ['k', '      ,"message": {"role": "assistant"'],
    ['s', '                ,"content": "Hi! How can I help?"'],
    ['k', '      }'],
    ['k', '      ,"finish_reason": "stop"}]'],
    ['k', ',  "usage": {"prompt_tokens": 12'],
    ['k', '              ,"completion_tokens": 9'],
    ['k', '              ,"total_tokens": 21}'],
  ] as [string, string][],
  meta: ['168 MS', '21 TOKENS', 'COST $0.0004', 'STREAM', 'SSE'],
}

const demoResp = {
  endpoint: 'POST /v1/responses',
  code: [
    ['k', 'curl'], [' ', ' https://api.kejiapi.dev/v1/responses \\'],
    [' ', '  -H "Authorization: Bearer sk-keji-****" \\'],
    [' ', '  -H "Content-Type: application/json" \\'],
    [' ', '  -d \'{'],
    [' ', '    "model": "gpt-4o",'],
    ['s', '    "input": "Summarize this repo"'],
    [' ', '  }\''],
  ] as [string, string][],
  resp: [
    ['k', '{ "id": "resp_3xk9…", "object": "response"'],
    ['k', ',  "model": "gpt-4o"'],
    ['k', ',  "status": "completed"'],
    ['k', ',  "output": [{"type": "message"'],
    ['k', '      ,"content": [{"type": "output_text"'],
    ['s', '          ,"text": "A self-hosted AI gateway…"'],
    ['k', '        }]}]'],
    ['k', ',  "usage": {"input_tokens": 412'],
    ['k', '              ,"output_tokens": 228}'],
  ] as [string, string][],
  meta: ['2.4 S', '640 TOKENS', 'COST $0.0021', 'JSON'],
}
</script>

<template>
  <div class="home">
    <!-- hero -->
    <section class="hero-wrap">
      <div class="hero-left">
        <span class="hero-badge"><span class="dot" />{{ L.home.badge }}</span>
        <h1 class="hero-title">
          {{ L.home.heroTitle1 }}<br /><span class="grad">{{ L.home.heroTitle2 }}</span>
        </h1>
        <p class="hero-sub">{{ L.home.heroSub }}</p>
        <div class="hero-ctas">
          <router-link to="/chat" class="cta-btn primary">{{ L.home.ctaStart }} →</router-link>
          <router-link to="/pricing" class="cta-btn">{{ L.home.ctaPricing }}</router-link>
          <router-link to="/models" class="cta-btn">{{ L.home.ctaModels }}</router-link>
        </div>
        <div class="app-pills">
          <span class="label">{{ L.home.appsLabel }}</span>
          <span v-for="a in L.home.apps" :key="a" class="app-pill">{{ a }}</span>
        </div>
      </div>
      <div class="hero-right">
        <div class="demo-card">
          <div class="demo-tabs">
            <button
              v-for="(tab, i) in L.home.demoTabs"
              :key="tab"
              class="demo-tab"
              :class="{ active: demoTab === i }"
              type="button"
              @click="demoTab = i"
            >
              {{ tab }}
            </button>
            <span class="demo-status"><span class="dot" />{{ L.home.demoStatus }}</span>
          </div>
          <div class="demo-endpoint"><b>{{ demoTab === 0 ? demoChat.endpoint : demoResp.endpoint }}</b></div>
          <pre class="demo-code"><span v-for="(line, i) in demoTab === 0 ? demoChat.code : demoResp.code" :key="'c' + i" class="line"><span v-for="(seg, j) in line" :key="j" :class="seg[0]">{{ seg[1] }}</span><br /></span></pre>
          <pre class="demo-code resp"><span v-for="(line, i) in demoTab === 0 ? demoChat.resp : demoResp.resp" :key="'r' + i" class="line"><span v-for="(seg, j) in line" :key="j" :class="seg[0]">{{ seg[1] }}</span><br /></span></pre>
          <div class="demo-meta">
            <span v-for="m in demoTab === 0 ? demoChat.meta : demoResp.meta" :key="m">{{ m }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- stats -->
    <section class="stat-row">
      <div v-for="s in L.home.stats" :key="s.label" class="stat-cell">
        <div class="stat-num"><span class="grad">{{ s.num }}</span></div>
        <div class="stat-label">{{ s.label }}</div>
      </div>
    </section>

    <!-- bento -->
    <section class="bento">
      <div v-for="(b, i) in L.home.bento" :key="b.title" class="bento-card">
        <span class="bento-num">{{ String(i + 1).padStart(2, '0') }}</span>
        <h3>{{ b.title }}</h3>
        <p>{{ b.desc }}</p>
      </div>
    </section>

    <!-- CTA -->
    <section class="cta-section">
      <h2>{{ L.home.ctaTitle }}</h2>
      <p>{{ L.home.ctaSub }}</p>
      <div class="hero-ctas" style="justify-content: center">
        <router-link to="/chat" class="cta-btn primary">{{ L.home.ctaBtn }} →</router-link>
        <router-link to="/models" class="cta-btn">{{ L.home.ctaDocs }}</router-link>
      </div>
    </section>

    <footer class="site-footer">
      <span>© {{ new Date().getFullYear() }} KejiAPI</span>
      <span>Self-hosted AI Gateway</span>
    </footer>
  </div>
</template>

<style scoped>
.home {
  max-width: 1080px;
  margin: 0 auto;
  padding: 48px 24px 0;
}
.hero-wrap {
  display: grid;
  grid-template-columns: 1.15fr 1fr;
  gap: 48px;
  align-items: center;
  min-height: 520px;
}
.demo-code.resp {
  border-top: 1px dashed var(--border);
}
@media (max-width: 960px) {
  .hero-wrap {
    grid-template-columns: 1fr;
    min-height: 0;
    gap: 32px;
  }
}
</style>
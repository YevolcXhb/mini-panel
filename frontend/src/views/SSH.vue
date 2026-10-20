<template>
  <div>
    <h2 class="page-title">💻 Web 终端</h2>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="本地终端" name="terminal">
        <el-alert type="info" show-icon :closable="false" style="margin-bottom: 12px" title="这是面板所在设备的本地终端，不是远程 SSH 连接。" />
        <div class="terminal-wrap">
          <div ref="terminalRef" class="term-output"></div>
        </div>
        <div style="margin-top: 12px">
          <el-button size="small" @click="reconnect">🔄 重新连接</el-button>
        </div>
        <el-alert
          type="success"
          show-icon
          :closable="false"
          style="margin-top: 12px"
          title="代理已自动注入"
          :description="proxyBanner"
        />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'

const activeTab = ref('terminal')
const terminalRef = ref<HTMLElement>()
const proxyHint = ref('')
const proxyBanner = ref('终端会话启动时会自动继承面板「设置 → 网络」里的代理配置，curl / wget / git / apt 无需手动 export。')
let term: Terminal
let ws: WebSocket
let fitAddon: FitAddon
let themeObserver: MutationObserver | null = null
let resizeObserver: ResizeObserver | null = null
let writeQueue: Promise<void> = Promise.resolve()

function getTerminalTheme() {
  const isDark = document.documentElement.classList.contains('dark')
  if (isDark) {
    return {
      background: '#000000',
      foreground: '#00ff00',
      cursor: '#00ff00',
      selectionBackground: 'rgba(0, 255, 0, 0.3)'
    }
  }
  return {
    background: '#f0f2f5',
    foreground: '#1a1d2e',
    cursor: '#1a1d2e',
    selectionBackground: 'rgba(26, 29, 46, 0.2)'
  }
}

function sendResize() {
  if (!term || !ws || ws.readyState !== WebSocket.OPEN) return
  ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
}

function doFit() {
  if (!term || !fitAddon) return
  fitAddon.fit()
  sendResize()
}

// 读取面板当前代理配置，用于终端连接成功后的提示行。
// 只在连接时拉一次：配置变更后重开终端即可生效。
async function loadProxyHint() {
  try {
    const token = localStorage.getItem('token') || ''
    const res = await fetch('/api/v1/terminal/proxy-env', {
      headers: { Authorization: `Bearer ${token}` }
    })
    const text = (await res.text()).trim()
    if (!text || text.startsWith('#') || text.startsWith('unset')) {
      proxyHint.value = '未启用代理，终端直连'
      proxyBanner.value = '当前未启用出站代理，终端为直连模式。在「设置 → 网络」开启代理后重开终端即可自动继承。'
      return
    }
    const m = text.match(/https?_proxy='([^']+)'/)
    const addr = m ? m[1] : ''
    proxyHint.value = addr ? `已继承代理 ${addr}` : '已继承代理配置'
    proxyBanner.value = `终端已自动继承代理 ${addr}，curl / wget / git / apt 将直接走代理。修改代理后执行 eval "$(proxy-on)" 或重连终端刷新。`
  } catch {
    proxyHint.value = ''
  }
}

function initTerminal() {
  if (term) {
    term.dispose()
    term = undefined as any
  }
  if (!terminalRef.value) return

  term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
    theme: getTerminalTheme(),
    convertEol: true
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalRef.value)
  doFit()

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token') || ''
  const wsUrl = `${protocol}//${window.location.host}/api/v1/terminal/ws?id=main`
  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    ws.send(JSON.stringify({ type: 'auth', token }))
    term.writeln('\x1b[32m[mini-panel terminal connected]\x1b[0m')
    // 提示当前代理继承情况，避免用户以为 curl 没走代理是配置没生效
    if (proxyHint.value) term.writeln(`\x1b[90m[${proxyHint.value}]\x1b[0m`)
    sendResize()
  }

  ws.onmessage = (e) => {
    if (typeof e.data === 'string') {
      // 后端拦截提示等文本消息直接显示
      term.write(e.data)
      return
    }
    // 串行写入，避免多个二进制帧乱序导致渲染错乱
    writeQueue = writeQueue
      .then(() => e.data.arrayBuffer())
      .then((buf) => {
        term.write(new Uint8Array(buf))
      })
      .catch(() => {})
  }

  ws.onclose = () => {
    term.writeln('\x1b[31m[connection closed]\x1b[0m')
  }

  ws.onerror = () => {
    term.writeln('\x1b[31m[connection error]\x1b[0m')
  }

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      // 终端输入统一按二进制帧发送，文本帧保留给 resize 控制消息
      ws.send(new TextEncoder().encode(data))
    }
  })
}

async function reconnect() {
  if (ws) ws.close()
  await loadProxyHint()
  initTerminal()
}

watch(activeTab, (v) => {
  if (v === 'terminal') {
    setTimeout(() => {
      initTerminal()
      requestAnimationFrame(() => {
        fitAddon?.fit()
        sendResize()
      })
    }, 100)
  }
})

onMounted(() => {
  loadProxyHint()
  if (activeTab.value === 'terminal') {
    initTerminal()
  }
  window.addEventListener('resize', doFit)

  resizeObserver = new ResizeObserver(() => {
    requestAnimationFrame(doFit)
  })
  if (terminalRef.value) {
    resizeObserver.observe(terminalRef.value)
  }

  themeObserver = new MutationObserver(() => {
    const newTheme = getTerminalTheme()
    term.options.theme = newTheme
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onUnmounted(() => {
  if (ws) ws.close()
  if (term) term.dispose()
  themeObserver?.disconnect()
  resizeObserver?.disconnect()
})
</script>

<style scoped>
.terminal-wrap {
  background: var(--bg);
  border: 1px solid var(--bdr);
  border-radius: var(--r);
  height: calc(100vh - 220px);
  overflow: hidden;
  position: relative;
}
.term-output {
  width: 100%;
  min-width: 0;
  height: 100%;
  padding: 0;
}
.term-output :deep(.xterm) {
  width: 100%;
  min-width: 0;
  height: 100%;
}

/* ── 移动端：终端高度按视口重算，提示框不撑破 ── */
@media (max-width: 900px) {
  h2.page-title {
    font-size: 17px;
    white-space: nowrap;
    flex-shrink: 0;
  }
  .terminal-wrap {
    height: calc(100dvh - 250px);
    min-height: 300px;
    border-radius: 6px;
  }
  .term-output :deep(.xterm) {
    padding: 4px 2px;
  }
  .term-output :deep(.xterm-viewport) {
    overflow-x: hidden !important;
  }
  :deep(.el-alert) {
    padding: 8px 10px !important;
  }
  :deep(.el-alert__title) {
    font-size: 12.5px !important;
    line-height: 1.45;
    white-space: normal;
    word-break: break-word;
  }
  :deep(.el-alert__description) {
    font-size: 12px !important;
    line-height: 1.5;
    margin: 4px 0 0 !important;
    word-break: break-word;
  }
  :deep(.el-tabs__header) {
    margin-bottom: 10px;
  }
  :deep(.el-tabs__item) {
    font-size: 13px;
    height: 36px;
    line-height: 36px;
  }
}

@media (max-width: 480px) {
  .terminal-wrap {
    height: calc(100dvh - 235px);
    min-height: 260px;
  }
  :deep(.el-alert__title) {
    font-size: 12px !important;
  }
}
</style>

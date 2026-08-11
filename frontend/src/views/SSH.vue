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

function reconnect() {
  if (ws) ws.close()
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
</style>

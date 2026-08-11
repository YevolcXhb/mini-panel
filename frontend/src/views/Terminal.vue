<template>
  <div>
    <h2 class="page-title">💻 Web 终端</h2>
    <div class="terminal-wrap">
      <div ref="terminalRef" class="term-output"></div>
    </div>
    <div style="margin-top: 12px">
      <el-button size="small" @click="reconnect">🔄 重新连接</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'

const terminalRef = ref<HTMLElement>()
let term: Terminal
let ws: WebSocket
let fitAddon: FitAddon
let themeObserver: MutationObserver | null = null

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

function initTerminal() {
  if (term) {
    term.dispose()
    term = undefined as any
  }

  term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
    theme: getTerminalTheme(),
    convertEol: true
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalRef.value!)
  fitAddon.fit()

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token') || ''
  const wsUrl = `${protocol}//${window.location.host}/api/v1/terminal/ws?id=main`
  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    ws.send(JSON.stringify({ type: 'auth', token }))
    term.writeln('\x1b[32m[mini-panel terminal connected]\x1b[0m')
  }

  ws.onmessage = (e) => {
    const reader = new FileReader()
    reader.onload = () => {
      term.write(new Uint8Array(reader.result as ArrayBuffer))
    }
    reader.readAsArrayBuffer(e.data)
  }

  ws.onclose = () => {
    term.writeln('\x1b[31m[connection closed]\x1b[0m')
  }

  ws.onerror = () => {
    term.writeln('\x1b[31m[connection error]\x1b[0m')
  }

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data)
    }
  })
}

function reconnect() {
  if (ws) ws.close()
  initTerminal()
}

onMounted(() => {
  initTerminal()
  window.addEventListener('resize', () => fitAddon?.fit())

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
})
</script>

<style scoped>
.terminal-wrap {
  background: var(--bg);
  border: 1px solid var(--bdr);
  border-radius: var(--r);
  height: calc(100vh - 200px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.term-output {
  flex: 1;
  padding: 12px;
}
.term-output :deep(.xterm) {
  height: 100%;
}
</style>

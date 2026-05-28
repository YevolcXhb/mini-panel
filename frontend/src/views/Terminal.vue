<template>
  <div>
    <el-card>
      <template #header>
        <div class="terminal-header">
          <span>Web 终端</span>
          <el-button size="small" @click="reconnect">重新连接</el-button>
        </div>
      </template>
      <div ref="terminalRef" class="terminal-container"></div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'

const terminalRef = ref<HTMLElement>()
let term: Terminal
let ws: WebSocket
let fitAddon: FitAddon

function initTerminal() {
  if (term) term.dispose()
  term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4'
    }
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalRef.value!)
  fitAddon.fit()

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/api/v1/terminal/ws?id=main`
  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    term.writeln('\x1b[32mConnected to mini-panel terminal\x1b[0m')
  }

  ws.onmessage = (e) => {
    const reader = new FileReader()
    reader.onload = () => {
      term.write(new Uint8Array(reader.result as ArrayBuffer))
    }
    reader.readAsArrayBuffer(e.data)
  }

  ws.onclose = () => {
    term.writeln('\x1b[31mConnection closed\x1b[0m')
  }

  ws.onerror = (err) => {
    term.writeln('\x1b[31mConnection error\x1b[0m')
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
})

onUnmounted(() => {
  if (ws) ws.close()
  if (term) term.dispose()
})
</script>

<style scoped>
.terminal-container {
  height: 600px;
  background: #1e1e1e;
  border-radius: 4px;
  padding: 10px;
}
.terminal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

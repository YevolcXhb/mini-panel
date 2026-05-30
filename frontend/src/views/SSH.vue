<template>
  <div>
    <h2 class="page-title">🔑 SSH 管理</h2>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="SSH 配置" name="config">
        <div class="info-card" style="max-width:600px">
          <div style="display:flex;flex-direction:column;gap:16px">
            <el-form label-width="100px">
              <el-form-item label="主机地址">
                <el-input placeholder="如 192.168.1.100" />
              </el-form-item>
              <el-form-item label="端口">
                <el-input placeholder="22" />
              </el-form-item>
              <el-form-item label="用户名">
                <el-input placeholder="root" />
              </el-form-item>
              <el-form-item label="密码">
                <el-input type="password" placeholder="SSH 密码" show-password />
              </el-form-item>
              <el-form-item label="密钥">
                <el-input type="textarea" :rows="4" placeholder="粘贴 SSH 私钥（可选）" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary">保存配置</el-button>
                <el-button>测试连接</el-button>
              </el-form-item>
            </el-form>
          </div>
        </div>
      </el-tab-pane>
      <el-tab-pane label="Web 终端" name="terminal">
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
    theme: {
      background: '#000000',
      foreground: '#00ff00',
      cursor: '#00ff00',
      selectionBackground: 'rgba(0, 255, 0, 0.3)'
    },
    convertEol: true
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(terminalRef.value)
  fitAddon.fit()

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token') || ''
  const wsUrl = `${protocol}//${window.location.host}/api/v1/terminal/ws?id=main&token=${token}`
  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
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

watch(activeTab, (v) => {
  if (v === 'terminal') {
    setTimeout(() => {
      initTerminal()
      fitAddon?.fit()
    }, 100)
  }
})

onMounted(() => {
  if (activeTab.value === 'terminal') {
    initTerminal()
  }
  window.addEventListener('resize', () => fitAddon?.fit())
})

onUnmounted(() => {
  if (ws) ws.close()
  if (term) term.dispose()
})
</script>

<style scoped>
.terminal-wrap {
  background: #000;
  border: 1px solid var(--bdr);
  border-radius: var(--r);
  height: calc(100vh - 220px);
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

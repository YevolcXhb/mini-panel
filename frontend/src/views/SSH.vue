<template>
  <div>
    <h2 class="page-title">🔑 SSH 管理</h2>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="SSH 配置" name="config">
        <div class="info-card" style="max-width:600px">
          <div style="display:flex;flex-direction:column;gap:16px">
            <el-form :model="sshForm" label-width="100px">
              <el-form-item label="主机地址">
                <el-input v-model="sshForm.host" placeholder="如 192.168.1.100" />
              </el-form-item>
              <el-form-item label="端口">
                <el-input v-model="sshForm.port" placeholder="22" />
              </el-form-item>
              <el-form-item label="用户名">
                <el-input v-model="sshForm.username" placeholder="root" />
              </el-form-item>
              <el-form-item label="密码">
                <el-input v-model="sshForm.password" type="password" placeholder="SSH 密码" show-password />
              </el-form-item>
              <el-form-item label="密钥">
                <el-input v-model="sshForm.privateKey" type="textarea" :rows="4" placeholder="粘贴 SSH 私钥（可选）" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" @click="saveConfig">保存配置</el-button>
                <el-button @click="testConnection">测试连接</el-button>
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
import { ElMessage } from 'element-plus'

const activeTab = ref('terminal')
const terminalRef = ref<HTMLElement>()
let term: Terminal
let ws: WebSocket
let fitAddon: FitAddon
let themeObserver: MutationObserver | null = null

const SSH_CONFIG_KEY = 'minipanel_ssh_config'

interface SSHConfig {
  host: string
  port: string
  username: string
  password: string
  privateKey: string
}

const sshForm = ref<SSHConfig>({
  host: '',
  port: '22',
  username: 'root',
  password: '',
  privateKey: ''
})

function loadConfig() {
  try {
    const raw = localStorage.getItem(SSH_CONFIG_KEY)
    if (raw) {
      const cfg = JSON.parse(raw)
      sshForm.value = { ...sshForm.value, ...cfg }
    }
  } catch (e) {
    // ignore
  }
}

function saveConfig() {
  localStorage.setItem(SSH_CONFIG_KEY, JSON.stringify(sshForm.value))
  ElMessage.success('配置已保存')
}

function testConnection() {
  ElMessage.info('测试连接功能开发中，请先使用 Web 终端')
}

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
  loadConfig()
  if (activeTab.value === 'terminal') {
    initTerminal()
  }
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
  height: calc(100vh - 220px);
  overflow: hidden;
  position: relative;
}
.term-output {
  width: 100%;
  height: 100%;
  padding: 0;
}
.term-output :deep(.xterm) {
  height: 100%;
}
</style>

<template>
  <div class="agent-container">
    <!-- 头部 -->
    <div class="agent-header">
      <div class="header-left">
        <el-icon class="agent-icon"><ChatDotRound /></el-icon>
        <span class="agent-title">Mini Agent</span>
        <el-tag v-if="streaming" type="warning" size="small">思考中...</el-tag>
      </div>
      <div class="header-right">
        <el-button text @click="showSettings = true">
          <el-icon><Setting /></el-icon> 设置
        </el-button>
        <el-button text type="danger" @click="clearChat">
          <el-icon><Delete /></el-icon> 清空
        </el-button>
      </div>
    </div>

    <!-- 消息区域 -->
    <div class="messages-area" ref="messagesRef">
      <!-- 空状态 -->
      <div v-if="messages.length === 0" class="empty-state">
        <el-icon class="empty-icon"><ChatLineRound /></el-icon>
        <h3>👋 你好，我是 Mini Agent</h3>
        <p>你可以问我关于服务器运维的任何问题，例如：</p>
        <div class="examples">
          <el-tag v-for="ex in examples" :key="ex" class="example-tag" @click="sendExample(ex)">
            {{ ex }}
          </el-tag>
        </div>
      </div>

      <div v-for="(msg, idx) in messages" :key="idx" class="message-wrapper">
        <!-- 用户消息 -->
        <div v-if="msg.role === 'user'" class="message user-message">
          <div class="message-content">{{ msg.content }}</div>
        </div>

        <!-- 助手消息 -->
        <div v-else-if="msg.role === 'assistant'" class="message assistant-message">
          <div class="message-avatar">🤖</div>
          <div class="message-body">
            <div v-if="msg.content" class="message-text" v-html="renderMarkdown(msg.content)"></div>
            <span v-if="streaming && idx === messages.length - 1 && streamBuffer.length > 0" class="typing-cursor">▌</span>
            <!-- 工具调用展示 -->
            <div v-if="msg.toolCalls && msg.toolCalls.length > 0" class="tool-calls">
              <div v-for="tc in msg.toolCalls" :key="tc.id" class="tool-call-card">
                <div class="tool-call-header">
                  <el-icon><Tools /></el-icon>
                  <span>调用工具: <strong>{{ tc.function.name }}</strong></span>
                </div>
                <pre class="tool-args">{{ formatJson(tc.function.arguments) }}</pre>
              </div>
            </div>
          </div>
        </div>

        <!-- 工具结果 -->
        <div v-else-if="msg.role === 'tool'" class="message tool-message">
          <div class="tool-result-card">
            <div class="tool-result-header">
              <el-icon><CircleCheck /></el-icon>
              <span>工具结果: {{ msg.toolName }}</span>
            </div>
            <pre class="tool-result-content">{{ msg.content }}</pre>
          </div>
        </div>
      </div>

      <!-- 确认对话框 -->
      <div v-if="pendingConfirm" class="confirm-card">
        <el-alert type="warning" :closable="false">
          <template #title>
            <span>⚠️ 需要确认</span>
          </template>
          <div class="confirm-content">
            <p>{{ pendingConfirm.message }}</p>
            <pre v-if="pendingConfirm.command" class="confirm-command">{{ pendingConfirm.command }}</pre>
          </div>
          <div class="confirm-actions">
            <el-button @click="handleConfirm(false)">取消</el-button>
            <el-button type="primary" @click="handleConfirm(true)">确认执行</el-button>
          </div>
        </el-alert>
      </div>
    </div>

    <!-- 输入区域 -->
    <div class="input-area">
      <el-input
        v-model="inputMessage"
        type="textarea"
        :rows="2"
        placeholder="输入你的问题..."
        @keydown.enter.prevent="handleSend"
        :disabled="streaming"
      />
      <el-button
        type="primary"
        :icon="Promotion"
        :loading="streaming"
        @click="handleSend"
        class="send-btn"
      >
        发送
      </el-button>
    </div>

    <!-- 设置抽屉 -->
    <el-drawer v-model="showSettings" title="Agent 设置" size="500px">
      <el-form :model="config" label-width="120px">
        <el-form-item label="提供商">
          <el-select v-model="config.provider" style="width: 100%">
            <el-option label="OpenAI" value="openai" />
            <el-option label="Anthropic (Claude)" value="anthropic" />
            <el-option label="DeepSeek" value="deepseek" />
            <el-option label="Ollama" value="ollama" />
            <el-option label="自定义 (OpenAI 兼容)" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="API Base URL">
          <el-input v-model="config.base_url" placeholder="https://api.openai.com/v1" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="config.apiKey" type="password" show-password placeholder="sk-..." />
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="config.model" placeholder="gpt-4o-mini" />
        </el-form-item>
        <el-form-item label="Temperature">
          <el-slider v-model="config.temperature" :min="0" :max="2" :step="0.1" />
        </el-form-item>
        <el-form-item label="Max Tokens">
          <el-input-number v-model="config.max_tokens" :min="512" :max="131072" :step="1024" />
        </el-form-item>
        <el-form-item label="启用技能">
          <el-checkbox-group v-model="config.skills">
            <el-checkbox v-for="skill in availableSkills" :key="skill.id" :label="skill.id">
              {{ skill.name }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveConfig">保存设置</el-button>
          <el-button @click="loadConfig">重置</el-button>
        </el-form-item>
      </el-form>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { ChatDotRound, Setting, Delete, ChatLineRound, Tools, CircleCheck, Promotion } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { agentApi, type AgentConfig, type StreamChunk } from '../api/agent'

interface ChatMessage {
  role: string
  content: string
  toolCalls?: any[]
  toolName?: string
}

const messagesRef = ref<HTMLDivElement>()
const inputMessage = ref('')
const streaming = ref(false)
const showSettings = ref(false)
const messages = ref<ChatMessage[]>([])
const currentStream = ref<string[]>([])
const pendingConfirm = ref<{ toolCallId: string; command: string; message: string } | null>(null)
const currentSessionId = ref(0)
let currentController: AbortController | null = null

// 打字机效果状态
const streamBuffer = ref('')
let typewriterTimer: number | null = null

function startTypewriter() {
  if (typewriterTimer) return
  typewriterTimer = window.setInterval(() => {
    if (streamBuffer.value.length === 0) {
      stopTypewriter()
      return
    }
    let charsToAdd = 1
    if (streamBuffer.value.length > 80) charsToAdd = 4
    else if (streamBuffer.value.length > 40) charsToAdd = 2
    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.content += streamBuffer.value.substring(0, charsToAdd)
    }
    streamBuffer.value = streamBuffer.value.substring(charsToAdd)
    scrollToBottom()
  }, 20)
}

function stopTypewriter() {
  if (typewriterTimer) {
    clearInterval(typewriterTimer)
    typewriterTimer = null
  }
  if (streamBuffer.value.length > 0) {
    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.content += streamBuffer.value
    }
    streamBuffer.value = ''
  }
}

const examples = [
  '查看系统状态',
  '列出所有容器',
  'Nginx 配置有什么问题吗',
  '备份所有网站',
  '查看防火墙规则',
  '重启 MySQL 容器',
]

const config = ref<AgentConfig & { apiKey: string; skills: string[] }>({
  provider: 'openai',
  base_url: '',
  model: 'gpt-4o-mini',
  temperature: 0.3,
  max_tokens: 4096,
  enabled: true,
  system_prompt: '',
  apiKey: '',
  skills: ['system', 'container', 'website', 'database', 'firewall', 'file', 'backup', 'web']
})

const availableSkills = ref<{ id: string; name: string; description: string; icon: string }[]>([])

onMounted(() => {
  loadConfig()
  createSession()
})

const defaultSkills = ['system', 'container', 'website', 'database', 'firewall', 'file', 'backup', 'web']

async function loadConfig() {
  try {
    const res: any = await agentApi.getConfig()
    if (res.code === 200) {
      Object.assign(config.value, res.data)
      if (res.data.skills && typeof res.data.skills === 'string' && res.data.skills.trim()) {
        try {
          config.value.skills = JSON.parse(res.data.skills)
        } catch {
          config.value.skills = [...defaultSkills]
        }
      } else {
        config.value.skills = [...defaultSkills]
      }
      if (res.available_skills && Array.isArray(res.available_skills)) {
        availableSkills.value = res.available_skills
      }
    }
  } catch {
    // ignore
  }
}

async function saveConfig() {
  try {
    const payload: any = {
      provider: config.value.provider,
      base_url: config.value.base_url,
      model: config.value.model,
      temperature: config.value.temperature,
      max_tokens: config.value.max_tokens,
      enabled: config.value.enabled,
      system_prompt: config.value.system_prompt,
      skills: JSON.stringify(config.value.skills)
    }
    if (config.value.apiKey && config.value.apiKey.trim()) {
      payload.api_key = config.value.apiKey.trim()
    }
    const res: any = await agentApi.updateConfig(payload)
    if (res.code === 200) {
      ElMessage.success('设置已保存')
      showSettings.value = false
    }
  } catch (err: any) {
    ElMessage.error(err.message || '保存失败')
  }
}

async function createSession() {
  try {
    const res: any = await agentApi.createSession()
    if (res.code === 200) {
      currentSessionId.value = res.data.id
    }
  } catch {
    // ignore
  }
}

function sendExample(text: string) {
  inputMessage.value = text
  handleSend()
}

function handleSend() {
  const text = inputMessage.value.trim()
  if (!text || streaming.value) return

  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '' })
  inputMessage.value = ''
  streaming.value = true
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  scrollToBottom()

  const chunks: StreamChunk[] = []

  currentController = agentApi.chat(
    currentSessionId.value,
    text,
    (chunk) => {
      chunks.push(chunk)
      handleChunk(chunk)
    },
    () => {
      finalizeStream(chunks)
    },
    (err) => {
      streaming.value = false
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${err}` })
      scrollToBottom()
    }
  )
}

function handleChunk(chunk: StreamChunk) {
  switch (chunk.type) {
    case 'message':
      if (chunk.content) {
        currentStream.value.push(chunk.content)
        streamBuffer.value += chunk.content
        startTypewriter()
      }
      break
    case 'tool_call':
      // 工具调用会在 finalize 时统一添加
      break
    case 'tool_result':
      // 工具结果会在 finalize 时统一添加
      break
    case 'confirm_required':
      pendingConfirm.value = {
        toolCallId: chunk.tool_call_id || '',
        command: chunk.command || '',
        message: chunk.message || '需要确认此操作'
      }
      streaming.value = false
      stopTypewriter()
      break
    case 'error':
      streaming.value = false
      stopTypewriter()
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${chunk.error}` })
      scrollToBottom()
      break
    case 'done':
      streaming.value = false
      // 不停止打字机，让缓冲区内容继续逐字显示完毕
      break
  }
}

function finalizeStream(chunks: StreamChunk[]) {
  streaming.value = false
  // 不清空 streamBuffer，让打字机继续逐字显示直到 buffer 为空
  currentStream.value = []

  const toolCalls: any[] = []
  const toolResults: ChatMessage[] = []

  for (const chunk of chunks) {
    if (chunk.type === 'tool_call') {
      toolCalls.push({
        id: chunk.tool_call_id,
        function: { name: chunk.tool_name, arguments: chunk.content }
      })
    } else if (chunk.type === 'tool_result') {
      toolResults.push({
        role: 'tool',
        content: chunk.content || '',
        toolName: chunk.tool_name || ''
      })
    }
  }

  const lastMsg = messages.value[messages.value.length - 1]
  if (lastMsg && lastMsg.role === 'assistant') {
    if (toolCalls.length > 0) {
      lastMsg.toolCalls = toolCalls
    }
  }
  for (const tr of toolResults) {
    messages.value.push(tr)
  }

  scrollToBottom()
}

function handleConfirm(confirmed: boolean) {
  if (!pendingConfirm.value) return
  const { toolCallId } = pendingConfirm.value
  pendingConfirm.value = null
  streamBuffer.value = ''
  stopTypewriter()

  if (confirmed) {
    messages.value.push({ role: 'assistant', content: '✅ 已确认执行' })
  } else {
    messages.value.push({ role: 'assistant', content: '❌ 已取消操作' })
  }
  scrollToBottom()

  agentApi.confirm(
    currentSessionId.value,
    toolCallId,
    confirmed,
    (chunk) => handleChunk(chunk),
    () => streaming.value = false,
    (err) => {
      streaming.value = false
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${err}` })
      scrollToBottom()
    }
  )
}

function clearChat() {
  messages.value = []
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  pendingConfirm.value = null
  if (currentController) {
    currentController.abort()
  }
  createSession()
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
    }
  })
}

function renderMarkdown(text: string): string {
  // 简单的 Markdown 渲染
  if (!text) return ''
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  // 代码块
  html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>')
  // 行内代码
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
  // 粗体
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  // 换行
  html = html.replace(/\n/g, '<br>')
  return html
}

function formatJson(str: string): string {
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}
</script>

<style scoped>
.agent-container {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 60px);
  background: var(--bg);
}

.agent-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 20px;
  background: var(--card);
  border-bottom: 1px solid var(--bdr);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.agent-icon {
  font-size: 24px;
  color: var(--acc);
}

.agent-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--txt);
}

.header-right {
  display: flex;
  gap: 8px;
}

.messages-area {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.empty-state {
  text-align: center;
  margin-top: 80px;
  color: var(--txt2);
}

.empty-icon {
  font-size: 64px;
  color: var(--dim);
  margin-bottom: 20px;
}

.examples {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  margin-top: 20px;
}

.example-tag {
  cursor: pointer;
  transition: all 0.2s;
}

.example-tag:hover {
  background: var(--acc);
  color: #fff;
}

.message-wrapper {
  display: flex;
  flex-direction: column;
}

.message {
  max-width: 80%;
  padding: 12px 16px;
  border-radius: 12px;
  line-height: 1.6;
}

.user-message {
  align-self: flex-end;
  background: var(--acc);
  color: #fff;
}

.assistant-message {
  align-self: flex-start;
  display: flex;
  gap: 10px;
  background: transparent;
  padding: 0;
  max-width: 90%;
}

.message-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--acc-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
}

.message-body {
  background: var(--card);
  padding: 12px 16px;
  border-radius: 12px;
  box-shadow: var(--shadow);
  flex: 1;
}

.message-text {
  color: var(--txt);
  word-break: break-word;
}

.message-text :deep(pre) {
  background: #282c34;
  color: #abb2bf;
  padding: 12px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 8px 0;
}

.message-text :deep(code) {
  background: var(--bg2);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
  color: var(--red);
}

.tool-message {
  align-self: flex-start;
  max-width: 90%;
  padding: 0;
}

.tool-calls {
  margin-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tool-call-card {
  background: var(--acc-bg);
  border: 1px solid var(--acc);
  border-radius: 8px;
  padding: 10px;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--acc);
  font-size: 13px;
  margin-bottom: 6px;
}

.tool-args {
  margin: 0;
  padding: 8px;
  background: var(--bg2);
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
}

.tool-result-card {
  background: rgba(52, 211, 153, 0.08);
  border: 1px solid var(--grn);
  border-radius: 8px;
  padding: 10px;
  margin-left: 46px;
}

.tool-result-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--grn);
  font-size: 13px;
  margin-bottom: 6px;
}

.tool-result-content {
  margin: 0;
  padding: 8px;
  background: var(--card);
  border-radius: 4px;
  font-size: 12px;
  overflow-x: auto;
  max-height: 300px;
  overflow-y: auto;
}

.confirm-card {
  align-self: flex-start;
  margin-left: 46px;
  max-width: 80%;
}

.confirm-content {
  margin: 10px 0;
  color: var(--txt);
}

.confirm-command {
  background: rgba(240, 101, 112, 0.08);
  border: 1px solid var(--red);
  padding: 8px;
  border-radius: 4px;
  margin-top: 8px;
  font-family: monospace;
  color: var(--txt);
}

.confirm-actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
}

.input-area {
  display: flex;
  gap: 10px;
  padding: 16px 20px;
  background: var(--card);
  border-top: 1px solid var(--bdr);
}

.send-btn {
  align-self: flex-end;
  height: 52px;
}

.typing-cursor {
  display: inline-block;
  color: var(--acc);
  animation: blink 1s step-end infinite;
  margin-left: 2px;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
</style>

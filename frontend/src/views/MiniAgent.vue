<template>
  <div class="agent-container">
    <!-- 会话侧边栏 -->
    <div class="sessions-sidebar">
      <div class="sidebar-header">
        <el-button type="primary" class="new-chat-btn" @click="createNewSession">
          <el-icon><Plus /></el-icon> 新建会话
        </el-button>
      </div>
      <div class="sessions-list">
        <div
          v-for="session in sessions"
          :key="session.id"
          class="session-item"
          :class="{ active: session.id === currentSessionId }"
          @click="switchSession(session.id)"
        >
          <el-icon class="session-icon"><ChatDotRound /></el-icon>
          <span class="session-title">{{ session.title }}</span>
          <el-icon class="delete-btn" @click.stop="deleteSession(session.id)"><Delete /></el-icon>
        </div>
      </div>
    </div>

    <!-- 主聊天区域 -->
    <div class="chat-main">
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
          <el-button text type="danger" @click="clearCurrentChat">
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
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { ChatDotRound, Setting, Delete, ChatLineRound, Tools, CircleCheck, Promotion, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { agentApi, type AgentConfig, type StreamChunk } from '../api/agent'

interface ChatMessage {
  role: string
  content: string
  toolCalls?: any[]
  toolName?: string
}

interface Session {
  id: number
  title: string
  created_at: string
  updated_at: string
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

const sessions = ref<Session[]>([])

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
  loadSessions()
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

async function loadSessions() {
  try {
    const res: any = await agentApi.listSessions()
    if (res.code === 200) {
      sessions.value = res.data || []
      if (sessions.value.length === 0) {
        await createNewSession()
      } else {
        await switchSession(sessions.value[0].id)
      }
    }
  } catch (err) {
    console.error('加载会话列表失败', err)
    await createNewSession()
  }
}

async function createNewSession() {
  try {
    if (currentController) {
      currentController.abort()
    }
    stopTypewriter()
    pendingConfirm.value = null
    streaming.value = false

    const res: any = await agentApi.createSession()
    if (res.code === 200) {
      const newSession = res.data
      sessions.value.unshift(newSession)
      currentSessionId.value = newSession.id
      messages.value = []
      inputMessage.value = ''
      streamBuffer.value = ''
      currentStream.value = []
      scrollToBottom()
    }
  } catch (err: any) {
    ElMessage.error('创建会话失败: ' + err.message)
  }
}

async function switchSession(sessionId: number) {
  if (streaming.value) {
    if (currentController) {
      currentController.abort()
    }
    streaming.value = false
  }
  stopTypewriter()
  pendingConfirm.value = null
  currentSessionId.value = sessionId
  await loadSessionMessages(sessionId)
}

async function loadSessionMessages(sessionId: number) {
  try {
    const res: any = await agentApi.getMessages(sessionId)
    if (res.code === 200) {
      const rawMessages = res.data || []
      messages.value = []
      
      // 转换后端消息格式到前端格式
      for (let i = 0; i < rawMessages.length; i++) {
        const m = rawMessages[i]
        if (m.role === 'user') {
          messages.value.push({ role: 'user', content: m.content })
        } else if (m.role === 'assistant') {
          const msg: ChatMessage = { role: 'assistant', content: m.content || '' }
          if (m.tool_calls && m.tool_calls.trim()) {
            try {
              msg.toolCalls = JSON.parse(m.tool_calls)
            } catch {}
          }
          messages.value.push(msg)
        } else if (m.role === 'tool') {
          messages.value.push({
            role: 'tool',
            content: m.content,
            toolName: m.tool_name
          })
        }
      }
      
      nextTick(() => scrollToBottom())
    }
  } catch (err: any) {
    console.error('加载历史消息失败', err)
    messages.value = []
  }
}

async function deleteSession(sessionId: number) {
  try {
    await ElMessageBox.confirm('确定要删除这个会话吗？删除后无法恢复。', '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    
    const res: any = await agentApi.deleteSession(sessionId)
    if (res.code === 200) {
      ElMessage.success('已删除')
      sessions.value = sessions.value.filter(s => s.id !== sessionId)
      if (currentSessionId.value === sessionId) {
        if (sessions.value.length > 0) {
          await switchSession(sessions.value[0].id)
        } else {
          await createNewSession()
        }
      }
    }
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error('删除失败: ' + err.message)
    }
  }
}

function sendExample(text: string) {
  inputMessage.value = text
  handleSend()
}

function handleSend() {
  const text = inputMessage.value.trim()
  if (!text || streaming.value || currentSessionId.value === 0) return

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
      break
  }
}

function finalizeStream(chunks: StreamChunk[]) {
  streaming.value = false
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

  // 刷新会话列表，更新标题
  loadSessions().then(() => {})
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

  streaming.value = true
  messages.value.push({ role: 'assistant', content: '' })

  const chunks: StreamChunk[] = []

  currentController = agentApi.confirm(
    currentSessionId.value,
    toolCallId,
    confirmed,
    (chunk) => handleChunk(chunk),
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

function clearCurrentChat() {
  messages.value = []
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  pendingConfirm.value = null
  if (currentController) {
    currentController.abort()
  }
  createNewSession()
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
    }
  })
}

function renderMarkdown(text: string): string {
  if (!text) return ''
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>')
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
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
  height: calc(100vh - 60px);
  background: var(--bg);
}

.sessions-sidebar {
  width: 260px;
  background: var(--card);
  border-right: 1px solid var(--bdr);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid var(--bdr);
}

.new-chat-btn {
  width: 100%;
}

.sessions-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.session-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  margin-bottom: 4px;
  transition: all 0.2s;
  color: var(--txt);
}

.session-item:hover {
  background: var(--bg2);
}

.session-item.active {
  background: var(--acc-bg);
  color: var(--acc);
}

.session-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.session-title {
  flex: 1;
  font-size: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.delete-btn {
  opacity: 0;
  font-size: 14px;
  color: var(--txt2);
  transition: opacity 0.2s;
  flex-shrink: 0;
}

.session-item:hover .delete-btn {
  opacity: 1;
}

.delete-btn:hover {
  color: var(--red);
}

.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
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

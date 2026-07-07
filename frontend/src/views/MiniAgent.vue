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
          <el-tag v-if="orchestrateMode" type="success" size="small">编排模式</el-tag>
          <el-tag v-if="streaming" type="warning" size="small">{{ phaseLabel || '思考中...' }}</el-tag>
        </div>
        <div class="header-right">
          <TokenStats
            v-if="tokensSaved > 0 || compressionCount > 0"
            :tokens-saved="tokensSaved"
            :compression-count="compressionCount"
            :cache-hits="cacheHits"
          />
          <el-button text @click="showSettings = true">
            <el-icon><Setting /></el-icon> 设置
          </el-button>
          <el-button text type="danger" @click="clearCurrentChat">
            <el-icon><Delete /></el-icon> 清空
          </el-button>
        </div>
      </div>

      <!-- 阶段进度条 -->
      <PhaseProgress
        v-if="orchestrateMode && currentPhase"
        :current-phase="currentPhase"
        :completed-phases="completedPhases"
        :step-number="stepNumber"
        :max-steps="maxSteps"
      />

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
              <!-- 实时工具调用卡片 -->
              <div v-if="msg.toolCalls && msg.toolCalls.length > 0" class="tool-calls">
                <ToolCallCard
                  v-for="tc in msg.toolCalls"
                  :key="tc.id"
                  :tool-call="tc"
                  :tool-result="msg.toolResults ? msg.toolResults[tc.id] || null : null"
                  :phase="msg.phase || currentPhase"
                />
              </div>
            </div>
          </div>

          <!-- 工具结果（仅在历史加载时显示，实时模式已聚合到 assistant 消息） -->
          <div v-else-if="msg.role === 'tool' && !msg.aggregated" class="message tool-message">
            <div class="tool-result-card">
              <div class="tool-result-header">
                <el-icon><CircleCheck /></el-icon>
                <span>工具结果: {{ msg.toolName }}</span>
              </div>
              <pre class="tool-result-content">{{ msg.content }}</pre>
            </div>
          </div>
        </div>

        <!-- 计划确认面板 -->
        <PlanPanel
          v-if="pendingPlan"
          :plan="pendingPlan.plan"
          @confirm="handlePlanConfirm(true)"
          @cancel="handlePlanConfirm(false)"
          @modify="handlePlanModify"
        />

        <!-- 确认对话框（工具调用确认，非计划确认） -->
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
        <div class="action-buttons">
          <el-button
            v-if="streaming"
            type="danger"
            @click="handleStop"
          >
            停止
          </el-button>
          <el-button
            v-if="!streaming && lastInput"
            @click="handleRegenerate"
          >
            重新生成
          </el-button>
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
      </div>

      <!-- 设置抽屉 -->
      <el-drawer v-model="showSettings" title="Agent 设置" size="500px">
        <el-form :model="config" label-width="120px">
          <el-form-item label="编排模式">
            <el-switch v-model="orchestrateMode" active-text="三阶段编排" inactive-text="单轮对话" />
            <div class="form-tip">编排模式：PLANNING → CODING → REVIEWING，复杂任务时启用</div>
          </el-form-item>
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
import { ChatDotRound, Setting, Delete, ChatLineRound, CircleCheck, Promotion, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { agentApi, type AgentConfig, type StreamChunk } from '../api/agent'
import PhaseProgress from '../components/agent/PhaseProgress.vue'
import PlanPanel from '../components/agent/PlanPanel.vue'
import ToolCallCard from '../components/agent/ToolCallCard.vue'
import TokenStats from '../components/agent/TokenStats.vue'

interface ToolCallInfo {
  id: string
  name: string
  arguments: string
}

interface ToolResultInfo {
  content: string
  success: boolean
}

interface ChatMessage {
  role: string
  content: string
  toolCalls?: ToolCallInfo[]
  toolResults?: Record<string, ToolResultInfo>
  toolName?: string
  aggregated?: boolean // 历史加载时已聚合到 assistant 消息的 tool 消息标记
  phase?: string
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
const pendingPlan = ref<{ plan: string } | null>(null)
const currentSessionId = ref(0)
let currentController: AbortController | null = null

const sessions = ref<Session[]>([])

// 三阶段编排状态
const orchestrateMode = ref(false)
const currentPhase = ref('')
const completedPhases = ref<string[]>([])
const stepNumber = ref(0)
const maxSteps = ref(0)

// 压缩和缓存统计
const tokensSaved = ref(0)
const compressionCount = ref(0)
const cacheHits = ref(0)

// 上次输入（用于重新生成）
const lastInput = ref('')

// 打字机效果状态
const streamBuffer = ref('')
let typewriterTimer: number | null = null

const phaseLabel = ref('')

function startTypewriter() {
  if (typewriterTimer) return
  typewriterTimer = window.setInterval(() => {
    if (streamBuffer.value.length === 0) {
      stopTypewriter()
      return
    }
    // 长文本立即显示，短文本打字效果
    let charsToAdd = 1
    if (streamBuffer.value.length > 100) charsToAdd = streamBuffer.value.length
    else if (streamBuffer.value.length > 40) charsToAdd = 3
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
    pendingPlan.value = null
    streaming.value = false
    resetOrchestrationState()

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

function resetOrchestrationState() {
  currentPhase.value = ''
  completedPhases.value = []
  stepNumber.value = 0
  maxSteps.value = 0
  phaseLabel.value = ''
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
  pendingPlan.value = null
  resetOrchestrationState()
  currentSessionId.value = sessionId
  await loadSessionMessages(sessionId)
}

async function loadSessionMessages(sessionId: number) {
  try {
    const res: any = await agentApi.getMessages(sessionId)
    if (res.code === 200) {
      const rawMessages = res.data || []
      messages.value = []

      // 转换后端消息格式到前端格式，聚合工具调用和结果
      for (let i = 0; i < rawMessages.length; i++) {
        const m = rawMessages[i]
        if (m.role === 'user') {
          messages.value.push({ role: 'user', content: m.content })
        } else if (m.role === 'assistant') {
          const msg: ChatMessage = { role: 'assistant', content: m.content || '' }
          if (m.tool_calls && m.tool_calls.trim()) {
            try {
              const rawToolCalls = JSON.parse(m.tool_calls)
              msg.toolCalls = rawToolCalls.map((tc: any) => ({
                id: tc.id || tc.ID || '',
                name: tc.function?.name || '',
                arguments: tc.function?.arguments || '{}'
              }))
              msg.toolResults = {}
            } catch {}
          }
          messages.value.push(msg)
        } else if (m.role === 'tool') {
          // 尝试聚合到前一个 assistant 消息的 toolResults
          const lastAssistant = messages.value[messages.value.length - 1]
          if (lastAssistant && lastAssistant.role === 'assistant' && lastAssistant.toolResults) {
            // 通过 tool_name 匹配（历史消息可能没有 tool_call_id）
            const tcId = m.tool_call_id || ''
            if (tcId && lastAssistant.toolResults[tcId]) {
              // 已存在，跳过
            } else if (tcId) {
              lastAssistant.toolResults[tcId] = {
                content: m.content,
                success: !m.content.startsWith('执行失败') && !m.content.startsWith('Error')
              }
            } else {
              // 无 tool_call_id，作为独立 tool 消息显示
              messages.value.push({
                role: 'tool',
                content: m.content,
                toolName: m.tool_name,
                aggregated: false
              })
            }
          } else {
            messages.value.push({
              role: 'tool',
              content: m.content,
              toolName: m.tool_name,
              aggregated: false
            })
          }
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

  lastInput.value = text
  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '' })
  inputMessage.value = ''
  streaming.value = true
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  resetOrchestrationState()
  // 重置压缩统计（每次新对话清零）
  tokensSaved.value = 0
  compressionCount.value = 0
  cacheHits.value = 0
  scrollToBottom()

  if (orchestrateMode.value) {
    startOrchestration(text)
  } else {
    startChat(text)
  }
}

function startChat(text: string) {
  currentController = agentApi.chat(
    currentSessionId.value,
    text,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
    (err) => {
      streaming.value = false
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${err}` })
      scrollToBottom()
    }
  )
}

function startOrchestration(text: string) {
  currentController = agentApi.orchestrate(
    currentSessionId.value,
    text,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
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
      // 实时添加工具调用到当前 assistant 消息
      {
        const lastMsg = messages.value[messages.value.length - 1]
        if (lastMsg && lastMsg.role === 'assistant') {
          if (!lastMsg.toolCalls) lastMsg.toolCalls = []
          if (!lastMsg.toolResults) lastMsg.toolResults = {}
          lastMsg.toolCalls.push({
            id: chunk.tool_call_id || '',
            name: chunk.tool_name || '',
            arguments: chunk.content || '{}'
          })
          scrollToBottom()
        }
      }
      break

    case 'tool_result':
      // 实时更新对应工具调用的结果
      {
        const lastMsg = messages.value[messages.value.length - 1]
        if (lastMsg && lastMsg.role === 'assistant' && lastMsg.toolResults && chunk.tool_call_id) {
          lastMsg.toolResults[chunk.tool_call_id] = {
            content: chunk.content || '',
            success: chunk.success ?? true
          }
          scrollToBottom()
        }
      }
      break

    case 'phase_start':
      currentPhase.value = chunk.phase || ''
      maxSteps.value = chunk.max_steps || 0
      stepNumber.value = 0
      updatePhaseLabel(chunk.phase || '')
      // 从已完成列表中移除（重新进入该阶段时）
      completedPhases.value = completedPhases.value.filter(p => p !== chunk.phase)
      break

    case 'phase_complete':
      if (chunk.phase && !completedPhases.value.includes(chunk.phase)) {
        completedPhases.value.push(chunk.phase)
      }
      break

    case 'plan_ready':
      pendingPlan.value = { plan: chunk.plan || '' }
      streaming.value = false
      stopTypewriter()
      break

    case 'compression_triggered':
      tokensSaved.value += chunk.tokens_saved || 0
      compressionCount.value++
      if (chunk.step_number) stepNumber.value = chunk.step_number
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
      // 编排模式下 done 可能表示阶段完成，不重置 currentPhase
      if (chunk.success === false && !pendingPlan.value) {
        // 错误退出，重置阶段状态
        resetOrchestrationState()
      }
      break
  }
}

function updatePhaseLabel(phase: string) {
  switch (phase) {
    case 'planning': phaseLabel.value = '规划中...'; break
    case 'coding': phaseLabel.value = '执行中...'; break
    case 'reviewing': phaseLabel.value = '审查中...'; break
    default: phaseLabel.value = '思考中...'
  }
}

function finalizeStream() {
  streaming.value = false
  currentStream.value = []
  stopTypewriter()

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

  currentController = agentApi.confirm(
    currentSessionId.value,
    toolCallId,
    confirmed,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
    (err) => {
      streaming.value = false
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${err}` })
      scrollToBottom()
    }
  )
}

// 计划确认处理
function handlePlanConfirm(confirmed: boolean) {
  if (!pendingPlan.value) return
  pendingPlan.value = null
  streamBuffer.value = ''
  stopTypewriter()

  if (confirmed) {
    messages.value.push({ role: 'assistant', content: '✅ 已确认计划，开始执行...' })
  } else {
    messages.value.push({ role: 'assistant', content: '❌ 已取消执行计划' })
    resetOrchestrationState()
    return
  }
  scrollToBottom()

  streaming.value = true
  messages.value.push({ role: 'assistant', content: '' })

  currentController = agentApi.confirmPlan(
    currentSessionId.value,
    confirmed,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
    (err) => {
      streaming.value = false
      messages.value.push({ role: 'assistant', content: `❌ 错误: ${err}` })
      scrollToBottom()
    }
  )
}

// 计划修改：将计划回填到输入框，用户编辑后重新发起编排
function handlePlanModify(modifiedPlan: string) {
  pendingPlan.value = null
  inputMessage.value = modifiedPlan
  ElMessage.info('计划已回填到输入框，编辑后重新发送')
  resetOrchestrationState()
}

// 停止生成
function handleStop() {
  if (currentController) {
    currentController.abort()
    currentController = null
  }
  streaming.value = false
  stopTypewriter()
  if (streamBuffer.value) {
    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.content += streamBuffer.value
    }
    streamBuffer.value = ''
  }
}

// 重新生成
function handleRegenerate() {
  if (!lastInput.value || streaming.value) return
  const text = lastInput.value
  // 移除最后的 assistant 消息
  if (messages.value.length > 0) {
    const last = messages.value[messages.value.length - 1]
    if (last.role === 'assistant') {
      messages.value.pop()
    }
  }
  inputMessage.value = text
  handleSend()
}

function clearCurrentChat() {
  messages.value = []
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  pendingConfirm.value = null
  pendingPlan.value = null
  if (currentController) {
    currentController.abort()
  }
  resetOrchestrationState()
  tokensSaved.value = 0
  compressionCount.value = 0
  cacheHits.value = 0
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
  align-items: center;
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

.action-buttons {
  display: flex;
  flex-direction: column;
  gap: 6px;
  justify-content: flex-end;
}

.send-btn {
  min-width: 80px;
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

.form-tip {
  font-size: 11px;
  color: var(--txt2);
  margin-top: 4px;
  line-height: 1.4;
}
</style>

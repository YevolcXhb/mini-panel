<template>
  <div class="agent-container">
    <!-- 会话侧边栏 -->
    <div class="sessions-sidebar">
      <div class="sidebar-header">
        <button class="new-chat-btn" @click="createNewSession">
          <svg class="btn-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          <span>新建对话</span>
        </button>
      </div>

      <div class="sessions-list">
        <div class="session-category-label">近期对话</div>
        <div
          v-for="session in sessions"
          :key="session.id"
          class="session-item"
          :class="{ active: session.id === currentSessionId }"
          @click="switchSession(session.id)"
        >
          <svg class="session-item-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path stroke-linecap="round" stroke-linejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
          </svg>
          <span class="session-title">{{ session.title || '新对话' }}</span>
          <button class="session-del-btn" title="删除会话" @click.stop="deleteSession(session.id)">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- 主聊天区域 -->
    <div class="chat-main">
      <!-- 极简高质感顶栏 -->
      <div class="gemini-header">
        <div class="header-left">
          <div class="sparkle-logo">
            <svg viewBox="0 0 24 24" fill="url(#sparkle-grad)" class="gemini-sparkle-svg">
              <defs>
                <linearGradient id="sparkle-grad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" stop-color="#4285f4" />
                  <stop offset="50%" stop-color="#9b72cf" />
                  <stop offset="100%" stop-color="#d96570" />
                </linearGradient>
              </defs>
              <path d="M12 2C12.5 7.5 16.5 11.5 22 12C16.5 12.5 12.5 16.5 12 22C11.5 16.5 7.5 12.5 2 12C7.5 11.5 11.5 7.5 12 2Z" />
            </svg>
          </div>
          <div class="header-meta">
            <span class="agent-brand">Mini Agent</span>
            <span class="agent-model-tag">{{ config.model || 'Gemini Pro' }}</span>
          </div>
          <el-tag v-if="orchestrateMode" size="small" class="mode-tag">
            <span class="dot-live"></span> 阶段编排模式
          </el-tag>
          <div v-if="streaming" class="live-status-pill">
            <span class="status-pulse-dot"></span>
            <span>{{ phaseLabel || '思考中...' }}</span>
          </div>
        </div>

        <div class="header-right">
          <TokenStats
            v-if="tokensSaved > 0 || compressionCount > 0"
            :tokens-saved="tokensSaved"
            :compression-count="compressionCount"
            :cache-hits="cacheHits"
          />
          <button class="topbar-btn" title="Agent 设置" @click="showSettings = true">
            <el-icon><Setting /></el-icon>
            <span class="btn-text">设置</span>
          </button>
          <button class="topbar-btn danger-hover" title="清空并新建" @click="clearCurrentChat">
            <el-icon><Delete /></el-icon>
            <span class="btn-text">清空</span>
          </button>
        </div>
      </div>

      <!-- 阶段进度指示条 -->
      <div v-if="orchestrateMode && currentPhase" class="phase-progress-wrapper">
        <PhaseProgress
          :current-phase="currentPhase"
          :completed-phases="completedPhases"
          :step-number="stepNumber"
          :max-steps="maxSteps"
        />
      </div>

      <!-- 消息流动滚动区 -->
      <div class="messages-area" ref="messagesRef">
        <!-- Gemini 欢迎与引导界面 -->
        <div v-if="messages.length === 0" class="gemini-hero-state">
          <div class="hero-sparkle-bg">
            <div class="gemini-star-icon">
              <svg viewBox="0 0 24 24" fill="url(#hero-grad)" class="hero-svg">
                <defs>
                  <linearGradient id="hero-grad" x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stop-color="#4285f4" />
                    <stop offset="45%" stop-color="#9b72cf" />
                    <stop offset="100%" stop-color="#d96570" />
                  </linearGradient>
                </defs>
                <path d="M12 2C12.5 7.5 16.5 11.5 22 12C16.5 12.5 12.5 16.5 12 22C11.5 16.5 7.5 12.5 2 12C7.5 11.5 11.5 7.5 12 2Z" />
              </svg>
            </div>
            <h1 class="hero-greeting">
              <span class="greeting-gradient">你好</span>，我是运维智能助手
            </h1>
            <p class="hero-subtext">已连接服务器底层引擎，可为你编排诊断、容器管理、Web安全防护与指令执行</p>
          </div>

          <div class="hero-cards-grid">
            <div
              v-for="(card, i) in exampleCards"
              :key="i"
              class="hero-card"
              @click="sendExample(card.prompt)"
            >
              <div class="hero-card-icon">{{ card.icon }}</div>
              <div class="hero-card-title">{{ card.title }}</div>
              <div class="hero-card-desc">{{ card.desc }}</div>
              <div class="hero-card-arrow">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        <!-- 消息列表 -->
        <div v-for="(msg, idx) in messages" :key="idx" class="message-row">
          <!-- 用户提问气泡 -->
          <div v-if="msg.role === 'user'" class="user-bubble-container">
            <div class="user-bubble">
              <div class="bubble-text">{{ msg.content }}</div>
            </div>
          </div>

          <!-- 助手回复（Gemini 沉浸式流式卡片） -->
          <div v-else-if="msg.role === 'assistant'" class="assistant-container">
            <div class="gemini-avatar">
              <svg viewBox="0 0 24 24" fill="url(#sparkle-grad)" class="avatar-svg">
                <path d="M12 2C12.5 7.5 16.5 11.5 22 12C16.5 12.5 12.5 16.5 12 22C11.5 16.5 7.5 12.5 2 12C7.5 11.5 11.5 7.5 12 2Z" />
              </svg>
            </div>

            <div class="assistant-content-wrapper">
              <!-- 工具调用折叠进度条（类似 Gemini 思考过程） -->
              <div v-if="msg.toolCalls && msg.toolCalls.length > 0" class="thought-process-container">
                <ToolCallCard
                  v-for="tc in msg.toolCalls"
                  :key="tc.id"
                  :tool-call="tc"
                  :tool-result="msg.toolResults ? msg.toolResults[tc.id] || null : null"
                  :phase="msg.phase || currentPhase"
                />
              </div>

              <!-- 思考中骨架占位 -->
              <div v-if="!msg.content && streaming && idx === messages.length - 1 && (!msg.toolCalls || msg.toolCalls.length === 0)" class="gemini-thinking-shimmer">
                <div class="shimmer-line line-1"></div>
                <div class="shimmer-line line-2"></div>
              </div>

              <!-- 渲染 Markdown 消息 -->
              <div
                v-if="msg.content"
                class="markdown-body gemini-markdown"
                v-html="renderMarkdown(msg.content)"
              ></div>

              <!-- 光标闪烁 -->
              <span
                v-if="streaming && idx === messages.length - 1 && (streamBuffer.length > 0 || currentStream.length > 0)"
                class="gemini-cursor"
              ></span>
            </div>
          </div>

          <!-- 历史工具结果独立项 -->
          <div v-else-if="msg.role === 'tool' && !msg.aggregated" class="tool-result-row">
            <div class="tool-result-pill">
              <span class="tool-tag">工具执行</span>
              <span class="tool-name">{{ msg.toolName }}</span>
              <pre class="tool-data">{{ msg.content }}</pre>
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

        <!-- 危险命令/工具确认卡片 (Gemini 风险拦截风格) -->
        <div v-if="pendingConfirm" class="gemini-confirm-dialog">
          <div class="confirm-icon-box">
            <svg viewBox="0 0 24 24" fill="none" stroke="#f0a830" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <div class="confirm-info">
            <div class="confirm-title">安全授权提示</div>
            <div class="confirm-desc">{{ pendingConfirm.message }}</div>
            <div v-if="pendingConfirm.command" class="confirm-cmd-preview">
              <code>{{ pendingConfirm.command }}</code>
            </div>
            <div class="confirm-buttons">
              <button class="pill-btn secondary" @click="handleConfirm(false)">拒绝取消</button>
              <button class="pill-btn primary" @click="handleConfirm(true)">授权执行</button>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部 Google Gemini 悬浮输入胶囊 (Floating Prompt Bar) -->
      <div class="floating-prompt-wrapper">
        <div class="gemini-input-pill" :class="{ 'is-focused': isInputFocused, 'is-streaming': streaming }">
          <textarea
            ref="textareaRef"
            v-model="inputMessage"
            class="gemini-textarea"
            :placeholder="streaming ? 'Mini Agent 正在工作中...' : '向 Mini Agent 发送指令或提问运维问题... (Enter 发送, Shift+Enter 换行)'"
            rows="1"
            :disabled="streaming"
            @focus="isInputFocused = true"
            @blur="isInputFocused = false"
            @keydown.enter.exact.prevent="handleSend"
            @input="adjustTextareaHeight"
          ></textarea>

          <div class="pill-actions">
            <!-- 重新生成按钮 -->
            <button
              v-if="!streaming && lastInput"
              class="pill-action-btn"
              title="重新生成上一次回答"
              @click="handleRegenerate"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>

            <!-- 停止生成按钮 -->
            <button
              v-if="streaming"
              class="pill-stop-btn"
              title="停止生成"
              @click="handleStop"
            >
              <div class="stop-square"></div>
            </button>

            <!-- 发送按钮 -->
            <button
              v-else
              class="pill-send-btn"
              :class="{ 'can-send': inputMessage.trim().length > 0 }"
              :disabled="!inputMessage.trim() || currentSessionId === 0"
              @click="handleSend"
            >
              <svg viewBox="0 0 24 24" fill="currentColor">
                <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z" />
              </svg>
            </button>
          </div>
        </div>
        <div class="gemini-footer-tip">Mini Agent 具备系统命令行与容器交互能力，请核实关键配置与操作结果</div>
      </div>

      <!-- Agent 设置抽屉 -->
      <el-drawer v-model="showSettings" title="Agent 配置中心" size="480px" class="gemini-drawer">
        <el-form :model="config" label-width="120px" class="gemini-form">
          <el-form-item label="阶段编排模式">
            <el-switch v-model="orchestrateMode" active-text="多阶段规划执行" inactive-text="标准直答" />
            <div class="form-tip">开启后执行：PLANNING → CODING → REVIEWING，适用复杂任务</div>
          </el-form-item>
          <el-form-item label="模型提供商">
            <el-select v-model="config.provider" style="width: 100%">
              <el-option label="OpenAI" value="openai" />
              <el-option label="Anthropic (Claude)" value="anthropic" />
              <el-option label="DeepSeek" value="deepseek" />
              <el-option label="Ollama (本地模型)" value="ollama" />
              <el-option label="自定义 (OpenAI 兼容协议)" value="custom" />
            </el-select>
          </el-form-item>
          <el-form-item label="API Base URL">
            <el-input v-model="config.base_url" placeholder="https://api.openai.com/v1" />
          </el-form-item>
          <el-form-item label="API Key">
            <el-input v-model="config.apiKey" type="password" show-password placeholder="sk-..." />
          </el-form-item>
          <el-form-item label="模型名称">
            <el-input v-model="config.model" placeholder="gpt-4o-mini / deepseek-chat" />
          </el-form-item>
          <el-form-item label="Temperature">
            <el-slider v-model="config.temperature" :min="0" :max="2" :step="0.1" />
          </el-form-item>
          <el-form-item label="Max Tokens">
            <el-input-number v-model="config.max_tokens" :min="512" :max="131072" :step="1024" />
          </el-form-item>
          <el-divider content-position="left">安全执行策略</el-divider>
          <el-form-item label="高危指令自动执行">
            <el-switch
              v-model="config.allow_dangerous_commands"
              active-text="直接执行"
              inactive-text="强制交互确认"
              inline-prompt
            />
            <div class="form-tip danger-tip">
              ⚠️ 开启后危险命令（rm -rf /、shutdown 等）将不再弹出确认气泡。
            </div>
          </el-form-item>
          <el-form-item label="执行超时 (秒)">
            <el-input-number v-model="config.exec_timeout_seconds" :min="10" :max="3600" :step="30" />
            <div class="form-tip">单次系统指令最大允许运行时间</div>
          </el-form-item>
          <el-form-item label="运维技能包">
            <el-checkbox-group v-model="config.skills">
              <el-checkbox v-for="skill in availableSkills" :key="skill.id" :label="skill.id">
                {{ skill.name }}
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>
          <div class="drawer-actions">
            <el-button type="primary" class="gemini-submit-btn" @click="saveConfig">保存并生效</el-button>
            <el-button @click="loadConfig">重置</el-button>
          </div>
        </el-form>
      </el-drawer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { Setting, Delete } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { agentApi, type StreamChunk } from '../api/agent'
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
  aggregated?: boolean
  phase?: string
}

interface Session {
  id: number
  title: string
  created_at: string
  updated_at: string
}

const messagesRef = ref<HTMLDivElement>()
const textareaRef = ref<HTMLTextAreaElement>()
const inputMessage = ref('')
const isInputFocused = ref(false)
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

// 上次输入
const lastInput = ref('')

// 打字机效果状态
const streamBuffer = ref('')
let typewriterTimer: number | null = null
const phaseLabel = ref('')

// 快捷卡片
const exampleCards = [
  {
    icon: '⚡',
    title: '系统全方位巡检',
    desc: '查看 CPU、内存、磁盘负载与前台热点进程',
    prompt: '对当前服务器进行一次全方位巡检，输出详细的资源使用报告和异常进程建议'
  },
  {
    icon: '🐳',
    title: 'Docker 容器状态',
    desc: '扫描运行中的容器状态与异常重启记录',
    prompt: '列出所有运行中的 Docker 容器，并检查是否有异常退出的实例'
  },
  {
    icon: '🌐',
    title: 'Web 与 Nginx 诊断',
    desc: '检查反向代理配置、SSL证书与错误日志',
    prompt: '检查 Nginx 服务配置与最近的错误日志，分析是否有潜在的访问故障'
  },
  {
    icon: '🛡️',
    title: '安全防护与防火墙',
    desc: '审查安全开放端口与高危规则',
    prompt: '查看当前系统防火墙规则，分析有哪些端口暴露并给出加固建议'
  }
]

const availableSkills = ref<{ id: string; name: string; description: string; icon: string }[]>([])

const config = ref<{
  provider: string
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  enabled: boolean
  system_prompt: string
  apiKey: string
  skills: string[]
  allow_dangerous_commands: boolean
  exec_timeout_seconds: number
}>({
  provider: 'openai',
  base_url: '',
  model: 'gpt-4o-mini',
  temperature: 0.3,
  max_tokens: 4096,
  enabled: true,
  system_prompt: '',
  apiKey: '',
  skills: ['system', 'container', 'website', 'database', 'firewall', 'file', 'backup', 'web'],
  allow_dangerous_commands: false,
  exec_timeout_seconds: 120
})

const defaultSkills = ['system', 'container', 'website', 'database', 'firewall', 'file', 'backup', 'web']

onMounted(() => {
  loadConfig()
  loadSessions()
})

function adjustTextareaHeight() {
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
      const newHeight = Math.min(textareaRef.value.scrollHeight, 160)
      textareaRef.value.style.height = `${newHeight}px`
    }
  })
}

function startTypewriter() {
  if (typewriterTimer) return
  typewriterTimer = window.setInterval(() => {
    if (streamBuffer.value.length === 0) {
      stopTypewriter()
      return
    }
    let charsToAdd = 1
    if (streamBuffer.value.length > 200) charsToAdd = 8
    else if (streamBuffer.value.length > 100) charsToAdd = 4
    else if (streamBuffer.value.length > 40) charsToAdd = 2

    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.content += streamBuffer.value.substring(0, charsToAdd)
    }
    streamBuffer.value = streamBuffer.value.substring(charsToAdd)
    scrollToBottom()
  }, 16)
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
      skills: JSON.stringify(config.value.skills),
      allow_dangerous_commands: config.value.allow_dangerous_commands,
      exec_timeout_seconds: config.value.exec_timeout_seconds
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
      adjustTextareaHeight()
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

      for (let i = 0; i < rawMessages.length; i++) {
        const m = rawMessages[i]
        if (m.role === 'user') {
          messages.value.push({ role: 'user', content: m.content })
        } else if (m.role === 'assistant') {
          const msg: ChatMessage = { role: 'assistant', content: m.content || '' }
          if (m.tool_calls && m.tool_calls.trim()) {
            try {
              const parsedCalls = JSON.parse(m.tool_calls)
              msg.toolCalls = (parsedCalls || []).map((tc: any) => ({
                id: tc.id || '',
                name: tc.name || (tc.function ? tc.function.name : ''),
                arguments: tc.arguments || (tc.function ? tc.function.arguments : '{}')
              }))
              msg.toolResults = {}
              let j = i + 1
              while (j < rawMessages.length && rawMessages[j].role === 'tool') {
                const toolMsg = rawMessages[j]
                if (toolMsg.tool_call_id) {
                  msg.toolResults[toolMsg.tool_call_id] = {
                    content: toolMsg.content || '',
                    success: !toolMsg.content?.startsWith('Error:') && !toolMsg.content?.startsWith('❌')
                  }
                  toolMsg._aggregated = true
                }
                j++
              }
            } catch {
              // ignore
            }
          }
          messages.value.push(msg)
        } else if (m.role === 'tool' && !m._aggregated) {
          messages.value.push({
            role: 'tool',
            content: m.content,
            toolName: m.tool_name,
            aggregated: false
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
    await ElMessageBox.confirm('确定要删除此会话吗？记录将永久移除。', '确认移除', {
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
  adjustTextareaHeight()
  handleSend()
}

// 统一安全的错误处理，防止双重空幽灵气泡
function setErrorMessage(errText: string) {
  streaming.value = false
  stopTypewriter()
  const lastMsg = messages.value[messages.value.length - 1]
  if (lastMsg && lastMsg.role === 'assistant' && !lastMsg.content && (!lastMsg.toolCalls || lastMsg.toolCalls.length === 0)) {
    lastMsg.content = `❌ ${errText}`
  } else {
    messages.value.push({ role: 'assistant', content: `❌ ${errText}` })
  }
  scrollToBottom()
}

function handleSend() {
  const text = inputMessage.value.trim()
  if (!text || streaming.value || currentSessionId.value === 0) return

  lastInput.value = text
  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '' })
  inputMessage.value = ''
  adjustTextareaHeight()

  streaming.value = true
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  resetOrchestrationState()
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
    (err) => setErrorMessage(`请求异常: ${err}`)
  )
}

function startOrchestration(text: string) {
  currentController = agentApi.orchestrate(
    currentSessionId.value,
    text,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
    (err) => setErrorMessage(`编排异常: ${err}`)
  )
}

function handleChunk(chunk: StreamChunk) {
  switch (chunk.type) {
    case 'token':
    case 'message':
      if (chunk.content) {
        currentStream.value.push(chunk.content)
        streamBuffer.value += chunk.content
        startTypewriter()
      }
      break

    case 'tool_call':
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
      {
        const lastMsg = messages.value[messages.value.length - 1]
        if (lastMsg && lastMsg.role === 'assistant' && lastMsg.toolResults && chunk.tool_call_id) {
          lastMsg.toolResults[chunk.tool_call_id] = {
            content: chunk.content || '',
            success: chunk.success ?? true
          }
          if (chunk.cached) cacheHits.value++
          scrollToBottom()
        }
      }
      break

    case 'phase_start':
      currentPhase.value = chunk.phase || ''
      maxSteps.value = chunk.max_steps || 0
      stepNumber.value = 0
      updatePhaseLabel(chunk.phase || '')
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
        message: chunk.message || '系统需要您授权此操作'
      }
      streaming.value = false
      stopTypewriter()
      break

    case 'error':
      setErrorMessage(chunk.error || '遇到未知异常')
      break

    case 'done':
      streaming.value = false
      if (chunk.success === false && !pendingPlan.value) {
        resetOrchestrationState()
      }
      break
  }
}

function updatePhaseLabel(phase: string) {
  switch (phase) {
    case 'planning': phaseLabel.value = '规划蓝图中...'; break
    case 'coding': phaseLabel.value = '执行运维中...'; break
    case 'reviewing': phaseLabel.value = '审查总结中...'; break
    default: phaseLabel.value = '思考中...'
  }
}

function finalizeStream() {
  streaming.value = false
  currentStream.value = []
  stopTypewriter()
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
    messages.value.push({ role: 'assistant', content: '✨ 已授权执行，正在处理...' })
  } else {
    messages.value.push({ role: 'assistant', content: '🚫 操作已拒绝取消' })
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
    (err) => setErrorMessage(`授权处理异常: ${err}`)
  )
}

function handlePlanConfirm(confirmed: boolean) {
  if (!pendingPlan.value) return
  pendingPlan.value = null
  streamBuffer.value = ''
  stopTypewriter()

  if (confirmed) {
    messages.value.push({ role: 'assistant', content: '🚀 计划已核准，进入执行阶段...' })
  } else {
    messages.value.push({ role: 'assistant', content: '❌ 计划已取消' })
    resetOrchestrationState()
    agentApi.confirmPlan(currentSessionId.value, false, () => {}, () => {}, () => {})
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
    (err) => setErrorMessage(`计划执行异常: ${err}`)
  )
}

function handlePlanModify(modifiedPlan: string) {
  pendingPlan.value = null
  inputMessage.value = modifiedPlan
  adjustTextareaHeight()
  ElMessage.info('计划已回填至输入框')
  agentApi.confirmPlan(currentSessionId.value, false, () => {}, () => {}, () => {})
  resetOrchestrationState()
}

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

function handleRegenerate() {
  if (!lastInput.value || streaming.value) return
  if (messages.value.length > 0) {
    const last = messages.value[messages.value.length - 1]
    if (last.role === 'assistant') {
      messages.value.pop()
    }
  }
  messages.value.push({ role: 'assistant', content: '' })
  streaming.value = true
  currentStream.value = []
  streamBuffer.value = ''
  stopTypewriter()
  resetOrchestrationState()
  scrollToBottom()

  currentController = agentApi.chat(
    currentSessionId.value,
    lastInput.value,
    (chunk) => handleChunk(chunk),
    () => finalizeStream(),
    (err) => setErrorMessage(`重新生成异常: ${err}`),
    true
  )
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

// 增强 Markdown 解析器：代码块、内联代码、标题、表格、加粗、斜体、列表、引用块
function renderMarkdown(text: string): string {
  if (!text) return ''

  // 1. 抽取代码块避免受后续规则干扰
  const codeBlocks: string[] = []
  let processed = text.replace(/```([a-zA-Z0-9_-]*)\n?([\s\S]*?)```/g, (_, lang, code) => {
    const safeCode = escapeHtml(code.trim())
    const blockIndex = codeBlocks.length
    codeBlocks.push(
      `<div class="gemini-code-block"><div class="code-header"><span class="code-lang">${lang || 'code'}</span><button class="code-copy-btn" onclick="navigator.clipboard.writeText(this.closest('.gemini-code-block').querySelector('code').innerText);this.innerText='已复制';setTimeout(()=>this.innerText='复制',1500)">复制</button></div><pre><code>${safeCode}</code></pre></div>`
    )
    return `§§CODEBLOCK_${blockIndex}§§`
  })

  // 2. 转义剩余基础字符
  processed = escapeHtml(processed)

  // 3. 内联代码
  processed = processed.replace(/`([^`]+)`/g, '<code class="gemini-inline-code">$1</code>')

  // 4. 标题解析
  processed = processed.replace(/^### (.*$)/gim, '<h3 class="gemini-h3">$1</h3>')
  processed = processed.replace(/^## (.*$)/gim, '<h2 class="gemini-h2">$1</h2>')
  processed = processed.replace(/^# (.*$)/gim, '<h1 class="gemini-h1">$1</h1>')

  // 5. 强调与加粗
  processed = processed.replace(/\*\*\*(.*?)\*\*\*/g, '<strong><em>$1</em></strong>')
  processed = processed.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  processed = processed.replace(/\*([^\*]+)\*/g, '<em>$1</em>')

  // 6. 引用块
  processed = processed.replace(/^\> (.*$)/gim, '<blockquote class="gemini-quote">$1</blockquote>')

  // 7. 列表项
  processed = processed.replace(/^\s*[-*+]\s+(.*$)/gim, '<li class="gemini-li">$1</li>')
  processed = processed.replace(/(<li class="gemini-li">.*<\/li>)/gis, '<ul class="gemini-ul">$1</ul>')
  processed = processed.replace(/<\/ul>\s*<ul class="gemini-ul">/g, '')

  // 8. 换行
  processed = processed.replace(/\n\n+/g, '<p class="gemini-p"></p>')
  processed = processed.replace(/\n/g, '<br/>')

  // 9. 还原代码块
  codeBlocks.forEach((block, idx) => {
    processed = processed.replace(`§§CODEBLOCK_${idx}§§`, block)
  })

  return processed
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;')
}
</script>

<style scoped>
/* ═══════════════════════════════════════════════
   Mini Agent — Gemini Modern Styling
   ═══════════════════════════════════════════════ */

.agent-container {
  display: flex;
  flex: 1;
  background: var(--card);
  border: 1px solid var(--bdr-light);
  border-radius: var(--r-lg);
  overflow: hidden;
  color: var(--txt);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
}

/* ── 侧边栏 ── */
.sessions-sidebar {
  width: 260px;
  background: var(--bg2);
  border-right: 1px solid var(--bdr-light);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  transition: all 0.3s ease;
}

.sidebar-header {
  padding: 16px;
  border-bottom: 1px solid var(--bdr);
}

.new-chat-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: 24px;
  border: 1px solid var(--gemini-pill-bdr, rgba(66, 133, 244, 0.25));
  background: var(--acc-bg);
  color: var(--txt);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.new-chat-btn:hover {
  background: var(--acc-bg2);
  border-color: var(--acc);
  transform: translateY(-1px);
}

.btn-icon {
  width: 16px;
  height: 16px;
}

.sessions-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 8px;
}

.session-category-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--dim);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 4px 12px 8px;
}

.session-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 12px;
  cursor: pointer;
  margin-bottom: 4px;
  transition: all 0.15s ease;
  color: var(--txt2);
  position: relative;
}

.session-item:hover {
  background: var(--bg2);
  color: var(--txt);
}

.session-item.active {
  background: var(--acc-bg);
  color: var(--acc);
  font-weight: 500;
}

.session-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 3px;
  border-radius: 0 4px 4px 0;
  background: var(--acc);
}

.session-item-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.session-title {
  flex: 1;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.session-del-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--dim);
  width: 22px;
  height: 22px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: all 0.2s;
}

.session-del-btn svg {
  width: 14px;
  height: 14px;
}

.session-item:hover .session-del-btn {
  opacity: 1;
}

.session-del-btn:hover {
  color: var(--red);
  background: rgba(240, 101, 112, 0.12);
}

/* ── 主区域 ── */
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  position: relative;
  background: transparent;
}

/* ── Gemini 极简 Header ── */
.gemini-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 24px;
  background: transparent;
  border-bottom: 1px solid var(--bdr-light);
  backdrop-filter: blur(12px);
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sparkle-logo {
  display: flex;
  align-items: center;
  justify-content: center;
}

.gemini-sparkle-svg {
  width: 24px;
  height: 24px;
  filter: drop-shadow(0 0 8px rgba(66, 133, 244, 0.4));
  animation: sparkle-rotate 6s ease-in-out infinite alternate;
}

@keyframes sparkle-rotate {
  0% { transform: scale(0.95) rotate(-5deg); }
  100% { transform: scale(1.05) rotate(5deg); }
}

.header-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-brand {
  font-size: 16px;
  font-weight: 600;
  background: var(--gemini-gradient-text, linear-gradient(90deg, #4285f4, #9b72cf));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.agent-model-tag {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 12px;
  background: var(--bg2);
  color: var(--dim);
}

.mode-tag {
  border-radius: 12px;
  background: rgba(52, 211, 153, 0.12);
  border: 1px solid rgba(52, 211, 153, 0.3);
  color: var(--grn);
}

.dot-live {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--grn);
  margin-right: 4px;
  animation: pulse-dot 1.5s infinite;
}

.live-status-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 16px;
  background: rgba(240, 168, 48, 0.12);
  border: 1px solid rgba(240, 168, 48, 0.3);
  color: var(--org);
  font-size: 12px;
}

.status-pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--org);
  animation: pulse-dot 1s infinite;
}

@keyframes pulse-dot {
  0%, 100% { transform: scale(0.8); opacity: 0.6; }
  50% { transform: scale(1.2); opacity: 1; }
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.topbar-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 16px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--txt2);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

.topbar-btn:hover {
  background: var(--bg2);
  color: var(--txt);
}

.topbar-btn.danger-hover:hover {
  color: var(--red);
  background: rgba(240, 101, 112, 0.1);
}

.phase-progress-wrapper {
  padding: 10px 24px 0;
}

/* ── 消息区域 ── */
.messages-area {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 24px;
  scroll-behavior: smooth;
}

/* ── Gemini 欢迎卡片与引导 ── */
.gemini-hero-state {
  max-width: 800px;
  margin: 40px auto 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.hero-sparkle-bg {
  margin-bottom: 32px;
}

.gemini-star-icon {
  display: flex;
  justify-content: center;
  margin-bottom: 16px;
}

.hero-svg {
  width: 48px;
  height: 48px;
  filter: drop-shadow(0 0 16px rgba(66, 133, 244, 0.45));
}

.hero-greeting {
  font-size: 28px;
  font-weight: 600;
  margin: 0 0 8px;
  color: var(--txt);
}

.greeting-gradient {
  background: var(--gemini-gradient-text, linear-gradient(90deg, #4285f4, #9b72cf));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.hero-subtext {
  font-size: 14px;
  color: var(--dim);
  max-width: 520px;
  line-height: 1.6;
  margin: 0 auto;
}

.hero-cards-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
  width: 100%;
}

.hero-card {
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 16px;
  padding: 16px;
  text-align: left;
  cursor: pointer;
  position: relative;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.hero-card:hover {
  border-color: var(--acc);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px var(--gemini-glow, rgba(66, 133, 244, 0.15));
}

.hero-card-icon {
  font-size: 20px;
  margin-bottom: 8px;
}

.hero-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--txt);
  margin-bottom: 4px;
}

.hero-card-desc {
  font-size: 12px;
  color: var(--dim);
  line-height: 1.5;
}

.hero-card-arrow {
  position: absolute;
  right: 14px;
  top: 14px;
  width: 16px;
  height: 16px;
  color: var(--dim);
  opacity: 0;
  transition: all 0.2s;
}

.hero-card:hover .hero-card-arrow {
  opacity: 1;
  color: var(--acc);
  transform: translateX(2px);
}

/* ── 消息项布局 ── */
.message-row {
  display: flex;
  flex-direction: column;
  max-width: 900px;
  width: 100%;
  margin: 0 auto;
}

/* 用户气泡（右上药丸椭圆风格） */
.user-bubble-container {
  display: flex;
  justify-content: flex-end;
}

.user-bubble {
  max-width: 75%;
  background: var(--bg2);
  border: 1px solid var(--bdr);
  border-radius: 20px;
  padding: 12px 18px;
  color: var(--txt);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.bubble-text {
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

/* 助手回复（左侧星芒通栏风格） */
.assistant-container {
  display: flex;
  gap: 16px;
  width: 100%;
}

.gemini-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: var(--acc-bg);
}

.avatar-svg {
  width: 20px;
  height: 20px;
}

.assistant-content-wrapper {
  flex: 1;
  min-width: 0;
}

/* 思考骨架屏 */
.gemini-thinking-shimmer {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 0;
}

.shimmer-line {
  height: 14px;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--bg2) 25%, var(--bdr) 50%, var(--bg2) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.6s infinite;
}

.shimmer-line.line-1 { width: 60%; }
.shimmer-line.line-2 { width: 40%; }

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

/* 打字机光标 */
.gemini-cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: var(--acc);
  margin-left: 4px;
  vertical-align: middle;
  border-radius: 2px;
  animation: gemini-blink 0.8s ease-in-out infinite;
}

@keyframes gemini-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

/* ── Markdown 样式对标 ── */
.gemini-markdown {
  font-size: 14.5px;
  line-height: 1.7;
  color: var(--txt);
  word-break: break-word;
}

.gemini-markdown :deep(.gemini-h1) {
  font-size: 20px;
  font-weight: 700;
  margin: 16px 0 10px;
  color: var(--txt);
}

.gemini-markdown :deep(.gemini-h2) {
  font-size: 17px;
  font-weight: 600;
  margin: 14px 0 8px;
  color: var(--txt);
}

.gemini-markdown :deep(.gemini-h3) {
  font-size: 15px;
  font-weight: 600;
  margin: 12px 0 6px;
  color: var(--txt);
}

.gemini-markdown :deep(.gemini-p) {
  margin: 10px 0;
}

.gemini-markdown :deep(.gemini-quote) {
  border-left: 3px solid var(--acc);
  padding-left: 12px;
  margin: 10px 0;
  color: var(--dim);
  background: var(--acc-bg);
  border-radius: 0 6px 6px 0;
  padding: 6px 12px;
}

.gemini-markdown :deep(.gemini-ul) {
  padding-left: 20px;
  margin: 8px 0;
}

.gemini-markdown :deep(.gemini-li) {
  margin-bottom: 4px;
}

.gemini-markdown :deep(.gemini-inline-code) {
  background: var(--bg2);
  color: var(--acc2);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  border: 1px solid var(--bdr);
}

.gemini-markdown :deep(.gemini-code-block) {
  margin: 12px 0;
  border-radius: 12px;
  overflow: hidden;
  background: #181b28;
  border: 1px solid var(--bdr);
}

.gemini-markdown :deep(.code-header) {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 14px;
  background: #12141f;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  font-size: 12px;
  color: var(--dim);
}

.gemini-markdown :deep(.code-copy-btn) {
  background: transparent;
  border: none;
  color: var(--dim);
  cursor: pointer;
  font-size: 12px;
  transition: color 0.2s;
}

.gemini-markdown :deep(.code-copy-btn:hover) {
  color: #fff;
}

.gemini-markdown :deep(pre) {
  margin: 0;
  padding: 12px 14px;
  overflow-x: auto;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  line-height: 1.5;
  color: #e2e4ed;
}

/* 历史工具卡片 */
.tool-result-row {
  margin-left: 48px;
}

.tool-result-pill {
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 12px;
}

.tool-tag {
  background: var(--acc-bg);
  color: var(--acc);
  padding: 2px 6px;
  border-radius: 4px;
  margin-right: 6px;
  font-weight: 500;
}

.tool-name {
  font-weight: 600;
  color: var(--txt);
}

.tool-data {
  margin: 6px 0 0;
  max-height: 160px;
  overflow-y: auto;
  font-family: monospace;
  color: var(--dim);
}

/* 确认卡片 */
.gemini-confirm-dialog {
  display: flex;
  gap: 16px;
  background: var(--card);
  border: 1px solid rgba(240, 168, 48, 0.4);
  border-radius: 16px;
  padding: 18px;
  margin-left: 48px;
  max-width: 650px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
}

.confirm-icon-box {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(240, 168, 48, 0.15);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.confirm-icon-box svg {
  width: 20px;
  height: 20px;
}

.confirm-info {
  flex: 1;
}

.confirm-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--txt);
  margin-bottom: 4px;
}

.confirm-desc {
  font-size: 13px;
  color: var(--txt2);
  line-height: 1.5;
}

.confirm-cmd-preview {
  margin: 10px 0;
  padding: 8px 12px;
  background: #12141f;
  border-radius: 8px;
  border: 1px solid var(--bdr);
}

.confirm-cmd-preview code {
  font-family: monospace;
  color: var(--red);
  font-size: 13px;
}

.confirm-buttons {
  display: flex;
  gap: 10px;
  margin-top: 12px;
}

.pill-btn {
  padding: 6px 16px;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all 0.2s;
}

.pill-btn.secondary {
  background: var(--bg2);
  color: var(--txt);
}

.pill-btn.secondary:hover {
  background: var(--bdr);
}

.pill-btn.primary {
  background: var(--acc);
  color: #fff;
}

.pill-btn.primary:hover {
  opacity: 0.9;
}

/* ── 底部 Gemini 悬浮输入胶囊 ── */
.floating-prompt-wrapper {
  padding: 10px 24px 20px;
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 100%;
  box-sizing: border-box;
}

.gemini-input-pill {
  width: 100%;
  max-width: 860px;
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 28px;
  padding: 10px 16px 10px 20px;
  display: flex;
  align-items: flex-end;
  gap: 12px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.08);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  box-sizing: border-box;
}

.gemini-input-pill.is-focused {
  border-color: var(--acc);
  box-shadow: 0 6px 28px var(--gemini-glow, rgba(66, 133, 244, 0.18));
}

.gemini-input-pill.is-streaming {
  border-color: rgba(240, 168, 48, 0.4);
}

.gemini-textarea {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--txt);
  font-size: 14.5px;
  line-height: 1.5;
  resize: none;
  max-height: 160px;
  min-height: 24px;
  padding: 4px 0;
  font-family: inherit;
}

.gemini-textarea::placeholder {
  color: var(--dim);
}

.pill-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 2px;
}

.pill-action-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  background: var(--bg2);
  color: var(--txt2);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s;
}

.pill-action-btn svg {
  width: 16px;
  height: 16px;
}

.pill-action-btn:hover {
  background: var(--bdr);
  color: var(--txt);
}

.pill-stop-btn {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: none;
  background: var(--red);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.stop-square {
  width: 12px;
  height: 12px;
  background: #fff;
  border-radius: 2px;
}

.pill-stop-btn:hover {
  transform: scale(1.05);
}

.pill-send-btn {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: none;
  background: var(--bg2);
  color: var(--dim);
  cursor: not-allowed;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.pill-send-btn svg {
  width: 16px;
  height: 16px;
}

.pill-send-btn.can-send {
  background: var(--acc);
  color: #fff;
  cursor: pointer;
}

.pill-send-btn.can-send:hover {
  transform: scale(1.06);
  box-shadow: 0 2px 10px rgba(66, 133, 244, 0.35);
}

.gemini-footer-tip {
  margin-top: 8px;
  font-size: 11px;
  color: var(--dim);
  text-align: center;
}

/* 抽屉样式微调 */
.gemini-form {
  padding: 10px 0;
}

.form-tip {
  font-size: 12px;
  color: var(--dim);
  margin-top: 4px;
}

.danger-tip {
  color: var(--red);
}

.drawer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 24px;
}

.gemini-submit-btn {
  background: var(--acc);
  border-color: var(--acc);
}
</style>

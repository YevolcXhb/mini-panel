import api from './index'

export interface AgentConfig {
  provider: string
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  enabled: boolean
  system_prompt: string
  skills?: string
}

export interface AgentSession {
  id: number
  title: string
  created_at: string
  updated_at: string
}

export interface AgentMessage {
  id: number
  session_id: number
  role: string
  content: string
  tool_calls: string
  tool_call_id: string
  tool_name: string
  tool_result: string
  created_at: string
}

export interface StreamChunk {
  type: string
  content?: string
  tool_call_id?: string
  tool_name?: string
  command?: string
  message?: string
  success?: boolean
  error?: string
  cached?: boolean
  // 三阶段编排扩展字段
  phase?: string           // planning / coding / reviewing
  plan?: string            // plan_ready 事件的计划内容
  tokens_saved?: number    // 压缩节省的 token 数
  step_number?: number     // 当前步数
  max_steps?: number       // 最大步数
}

// SSE 回调签名
type SSECallbacks = {
  onChunk: (chunk: StreamChunk) => void
  onDone: () => void
  onError: (err: string) => void
}

// parseSSEStream 解析 SSE 流并分发回调。返回 AbortController 供外部中断。
// 四个端点（chat / confirm / orchestrate / confirm-plan）共用此逻辑。
function parseSSEStream(url: string, body: Record<string, unknown>, cb: SSECallbacks): AbortController {
  const controller = new AbortController()
  fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
    },
    body: JSON.stringify(body),
    signal: controller.signal
  }).then(async (res) => {
    if (!res.ok) {
      const text = await res.text()
      cb.onError(text || `HTTP ${res.status}`)
      return
    }
    const reader = res.body?.getReader()
    if (!reader) {
      cb.onError('no response body')
      return
    }
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const trimmed = line.trim()
        if (trimmed.startsWith('data:')) {
          const jsonStr = trimmed.slice(5).trim()
          if (jsonStr === '[DONE]') continue
          try {
            const chunk: StreamChunk = JSON.parse(jsonStr)
            cb.onChunk(chunk)
          } catch {
            // ignore parse error
          }
        }
      }
    }
    cb.onDone()
  }).catch((err) => {
    if (err.name === 'AbortError') return
    cb.onError(err.message || '网络错误')
  })
  return controller
}

export const agentApi = {
  getConfig: () => api.get('/agent/config'),
  updateConfig: (data: AgentConfig) => api.put('/agent/config', data),

  listSessions: () => api.get('/agent/sessions'),
  createSession: (title?: string) => api.post('/agent/sessions', { title }),
  deleteSession: (id: number) => api.delete(`/agent/sessions/${id}`),
  getMessages: (id: number) => api.get(`/agent/sessions/${id}/messages`),

  // chat 单轮 ReAct 聊天（SSE）
  chat: (sessionId: number, message: string, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void, regenerate = false) => {
    return parseSSEStream('/api/v1/agent/chat', { session_id: sessionId, message, regenerate }, { onChunk, onDone, onError })
  },

  // confirm 工具调用确认（SSE）
  confirm: (sessionId: number, toolCallId: string, confirmed: boolean, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void) => {
    return parseSSEStream('/api/v1/agent/confirm', { session_id: sessionId, tool_call_id: toolCallId, confirmed }, { onChunk, onDone, onError })
  },

  // orchestrate 三阶段编排（SSE）：PLANNING → plan_ready → 等待确认
  orchestrate: (sessionId: number, message: string, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void) => {
    return parseSSEStream('/api/v1/agent/orchestrate', { session_id: sessionId, message }, { onChunk, onDone, onError })
  },

  // confirmPlan 确认或取消编排计划（SSE）：confirmed=true 继续 CODING→REVIEWING
  confirmPlan: (sessionId: number, confirmed: boolean, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void) => {
    return parseSSEStream('/api/v1/agent/confirm-plan', { session_id: sessionId, confirmed }, { onChunk, onDone, onError })
  }
}

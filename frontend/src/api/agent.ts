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
}

export const agentApi = {
  getConfig: () => api.get('/agent/config'),
  updateConfig: (data: AgentConfig) => api.put('/agent/config', data),

  listSessions: () => api.get('/agent/sessions'),
  createSession: (title?: string) => api.post('/agent/sessions', { title }),
  deleteSession: (id: number) => api.delete(`/agent/sessions/${id}`),
  getMessages: (id: number) => api.get(`/agent/sessions/${id}/messages`),

  chat: (sessionId: number, message: string, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void) => {
    const controller = new AbortController()
    fetch('/api/v1/agent/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
      },
      body: JSON.stringify({ session_id: sessionId, message }),
      signal: controller.signal
    }).then(async (res) => {
      if (!res.ok) {
        const text = await res.text()
        onError(text)
        return
      }
      const reader = res.body?.getReader()
      if (!reader) {
        onError('no response body')
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
              onChunk(chunk)
            } catch {
              // ignore parse error
            }
          }
        }
      }
      onDone()
    }).catch((err) => {
      onError(err.message || '网络错误')
    })
    return controller
  },

  confirm: (sessionId: number, toolCallId: string, confirmed: boolean, onChunk: (chunk: StreamChunk) => void, onDone: () => void, onError: (err: string) => void) => {
    const controller = new AbortController()
    fetch('/api/v1/agent/confirm', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
      },
      body: JSON.stringify({ session_id: sessionId, tool_call_id: toolCallId, confirmed }),
      signal: controller.signal
    }).then(async (res) => {
      if (!res.ok) {
        const text = await res.text()
        onError(text)
        return
      }
      const reader = res.body?.getReader()
      if (!reader) {
        onError('no response body')
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
            try {
              const chunk: StreamChunk = JSON.parse(jsonStr)
              onChunk(chunk)
            } catch {
              // ignore
            }
          }
        }
      }
      onDone()
    }).catch((err) => {
      onError(err.message || '网络错误')
    })
    return controller
  }
}

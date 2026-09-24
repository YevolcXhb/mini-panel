<template>
  <div class="tool-call-card" :class="statusClass">
    <!-- 卡片头部 (Gemini Thinking Steps 风格) -->
    <div class="card-header" @click="toggleExpand">
      <div class="header-left">
        <span class="status-indicator">
          <span v-if="status === 'running'" class="gemini-spin-ring"></span>
          <span v-else-if="status === 'success'" class="icon-ok">✓</span>
          <span v-else-if="status === 'failed'" class="icon-fail">✗</span>
          <span v-else class="icon-wait">○</span>
        </span>
        <span class="tool-name">{{ toolCall.name }}</span>
        <span class="phase-badge" v-if="phase">{{ phaseLabel }}</span>
      </div>

      <div class="header-right">
        <span class="status-text">{{ statusText }}</span>
        <span class="expand-arrow" :class="{ 'is-expanded': expanded }">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </span>
      </div>
    </div>

    <!-- 卡片展开细节 -->
    <div v-if="expanded" class="card-body">
      <!-- 参数区域 -->
      <div class="section">
        <div class="section-label">入参 Arguments</div>
        <pre class="json-block">{{ formattedArgs }}</pre>
      </div>

      <!-- 结果区域 -->
      <div v-if="toolResult" class="section">
        <div class="section-label">
          <span>调用结果 Output</span>
          <div class="result-actions">
            <span class="result-tag" :class="toolResult.success ? 'ok' : 'err'">
              {{ toolResult.success ? '成功' : '失败' }}
            </span>
            <button class="mini-copy-btn" @click.stop="copyResult">复制</button>
          </div>
        </div>
        <pre class="json-block result-content" :class="{ collapsed: !resultExpanded }">{{ displayResult }}</pre>
        <button
          v-if="toolResult.content && toolResult.content.length > 400"
          class="text-expand-btn"
          @click.stop="resultExpanded = !resultExpanded"
        >
          {{ resultExpanded ? '折叠' : `展开全部 (${toolResult.content.length} 字符)` }}
        </button>
      </div>

      <!-- lazy-ref 提示 -->
      <div v-if="hasLazyRef" class="lazy-ref-hint">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="hint-icon">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span>输出过长已转为引用缓存，Agent 将根据需要通过 resolve_lazy_ref 精确检索</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'

interface ToolCallInfo {
  id: string
  name: string
  arguments: string
}

interface ToolResultInfo {
  content: string
  success: boolean
}

const props = defineProps<{
  toolCall: ToolCallInfo
  toolResult: ToolResultInfo | null
  phase?: string
}>()

const expanded = ref(false)
const resultExpanded = ref(false)

const status = computed<'pending' | 'running' | 'success' | 'failed'>(() => {
  if (props.toolResult === null) return 'running'
  return props.toolResult.success ? 'success' : 'failed'
})

const statusClass = computed(() => `status-${status.value}`)

const statusText = computed(() => {
  switch (status.value) {
    case 'running': return '执行中...'
    case 'success': return '已完成'
    case 'failed': return '异常'
    default: return '准备'
  }
})

const phaseLabel = computed(() => {
  switch (props.phase) {
    case 'planning': return '规划'
    case 'coding': return '执行'
    case 'reviewing': return '审查'
    default: return props.phase || ''
  }
})

const formattedArgs = computed(() => {
  try {
    return JSON.stringify(JSON.parse(props.toolCall.arguments), null, 2)
  } catch {
    return props.toolCall.arguments
  }
})

const hasLazyRef = computed(() => {
  return props.toolResult?.content?.includes('[lazy-ref:') || false
})

const displayResult = computed(() => {
  if (!props.toolResult) return ''
  const content = props.toolResult.content
  if (!resultExpanded.value && content.length > 400) {
    return content.substring(0, 200) + '\n... [点击下方展开完整结果]'
  }
  return content
})

function toggleExpand() {
  expanded.value = !expanded.value
}

function copyResult() {
  if (props.toolResult?.content) {
    navigator.clipboard.writeText(props.toolResult.content).then(() => {
      ElMessage.success('已复制到剪贴板')
    }).catch(() => {
      ElMessage.error('复制失败')
    })
  }
}
</script>

<style scoped>
.tool-call-card {
  border: 1px solid var(--bdr);
  background: var(--card);
  border-radius: 12px;
  margin: 6px 0;
  overflow: hidden;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.tool-call-card:hover {
  border-color: rgba(66, 133, 244, 0.4);
}

.tool-call-card.status-running {
  border-color: rgba(240, 168, 48, 0.4);
  background: rgba(240, 168, 48, 0.02);
}

.tool-call-card.status-success {
  border-color: var(--bdr);
}

.tool-call-card.status-failed {
  border-color: rgba(240, 101, 112, 0.4);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 14px;
  cursor: pointer;
  user-select: none;
  background: rgba(255, 255, 255, 0.01);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  font-size: 12px;
}

.gemini-spin-ring {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(240, 168, 48, 0.2);
  border-top-color: var(--org);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.icon-ok {
  color: var(--grn);
  font-weight: bold;
}

.icon-fail {
  color: var(--red);
  font-weight: bold;
}

.icon-wait {
  color: var(--dim);
}

.tool-name {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--txt);
}

.phase-badge {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  background: var(--bg2);
  color: var(--dim);
}

.status-text {
  font-size: 12px;
  color: var(--dim);
}

.expand-arrow {
  width: 16px;
  height: 16px;
  color: var(--dim);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.2s;
}

.expand-arrow svg {
  width: 14px;
  height: 14px;
}

.expand-arrow.is-expanded {
  transform: rotate(180deg);
}

.card-body {
  padding: 10px 14px 14px;
  border-top: 1px solid var(--bdr);
  background: var(--bg);
}

.section {
  margin-top: 8px;
}

.section-label {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--dim);
  margin-bottom: 6px;
}

.result-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.result-tag {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 4px;
}

.result-tag.ok {
  background: rgba(52, 211, 153, 0.15);
  color: var(--grn);
}

.result-tag.err {
  background: rgba(240, 101, 112, 0.15);
  color: var(--red);
}

.mini-copy-btn {
  background: transparent;
  border: none;
  font-size: 11px;
  color: var(--dim);
  cursor: pointer;
  padding: 0 4px;
}

.mini-copy-btn:hover {
  color: var(--acc);
}

.json-block {
  margin: 0;
  padding: 8px 12px;
  background: #151824;
  border: 1px solid var(--bdr);
  border-radius: 8px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  line-height: 1.45;
  color: #cad0db;
  overflow-x: auto;
  max-height: 240px;
}

.result-content.collapsed {
  max-height: 120px;
}

.text-expand-btn {
  margin-top: 6px;
  background: transparent;
  border: none;
  color: var(--acc);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}

.text-expand-btn:hover {
  text-decoration: underline;
}

.lazy-ref-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  padding: 6px 10px;
  border-radius: 6px;
  background: rgba(66, 133, 244, 0.08);
  border: 1px solid rgba(66, 133, 244, 0.2);
  color: var(--blue);
  font-size: 11px;
}

.hint-icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
</style>

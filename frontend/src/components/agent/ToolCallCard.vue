<template>
  <div class="tool-call-card" :class="statusClass">
    <!-- 卡片头部 -->
    <div class="card-header" @click="toggleExpand">
      <span class="status-indicator">
        <span v-if="status === 'running'" class="spinner"></span>
        <span v-else-if="status === 'success'">✓</span>
        <span v-else-if="status === 'failed'">✗</span>
        <span v-else>○</span>
      </span>
      <span class="tool-name">{{ toolCall.name }}</span>
      <span class="phase-badge" v-if="phase">{{ phaseLabel }}</span>
      <span class="expand-icon">{{ expanded ? '▼' : '▶' }}</span>
    </div>

    <!-- 卡片内容 -->
    <div v-if="expanded" class="card-body">
      <!-- 参数区域 -->
      <div class="section">
        <div class="section-label">参数</div>
        <pre class="json-block">{{ formattedArgs }}</pre>
      </div>

      <!-- 结果区域 -->
      <div v-if="toolResult" class="section">
        <div class="section-label">
          结果
          <span class="result-status" :class="toolResult.success ? 'ok' : 'err'">
            {{ toolResult.success ? '成功' : '失败' }}
          </span>
          <el-button text size="small" @click.stop="copyResult" class="copy-btn">
            复制
          </el-button>
        </div>
        <pre class="json-block" :class="{ collapsed: !resultExpanded }">{{ displayResult }}</pre>
        <el-button
          v-if="toolResult.content && toolResult.content.length > 500"
          text
          size="small"
          @click.stop="resultExpanded = !resultExpanded"
          class="expand-result-btn"
        >
          {{ resultExpanded ? '折叠' : `展开全部 (${toolResult.content.length} 字符)` }}
        </el-button>
      </div>

      <!-- lazy-ref 提示 -->
      <div v-if="hasLazyRef" class="lazy-ref-hint">
        <el-icon><InfoFilled /></el-icon>
        <span>完整内容已缓存，可由 Agent 通过 resolve_lazy_ref 工具按需取回</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { InfoFilled } from '@element-plus/icons-vue'
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

const expanded = ref(true)
const resultExpanded = ref(false)

const status = computed<'pending' | 'running' | 'success' | 'failed'>(() => {
  if (props.toolResult === null) return 'running'
  return props.toolResult.success ? 'success' : 'failed'
})

const statusClass = computed(() => `status-${status.value}`)

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
  if (!resultExpanded.value && content.length > 500) {
    return content.substring(0, 200) + '\n... [点击展开全部]'
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
  border-radius: 8px;
  margin: 6px 0;
  overflow: hidden;
  transition: border-color 0.2s;
}

.tool-call-card.status-success {
  border-color: var(--grn);
}

.tool-call-card.status-failed {
  border-color: var(--red);
}

.tool-call-card.status-running {
  border-color: var(--acc);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  background: var(--bg2);
  transition: background 0.2s;
}

.card-header:hover {
  background: var(--bg);
}

.status-indicator {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  font-size: 13px;
  font-weight: bold;
}

.status-success .status-indicator {
  color: var(--grn);
}

.status-failed .status-indicator {
  color: var(--red);
}

.status-running .status-indicator {
  color: var(--acc);
}

.spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid var(--acc);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.tool-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--txt);
  flex: 1;
}

.phase-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--acc-bg);
  color: var(--acc);
}

.expand-icon {
  font-size: 10px;
  color: var(--txt2);
}

.card-body {
  padding: 10px 12px;
}

.section {
  margin-bottom: 10px;
}

.section:last-child {
  margin-bottom: 0;
}

.section-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--txt2);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.result-status {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
}

.result-status.ok {
  background: rgba(52, 211, 153, 0.15);
  color: var(--grn);
}

.result-status.err {
  background: rgba(240, 101, 112, 0.15);
  color: var(--red);
}

.copy-btn {
  margin-left: auto;
  font-size: 11px;
}

.json-block {
  margin: 4px 0 0;
  padding: 8px;
  background: var(--bg);
  border-radius: 4px;
  font-size: 12px;
  font-family: monospace;
  overflow-x: auto;
  max-height: 300px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--txt);
}

.json-block.collapsed {
  max-height: 100px;
  overflow: hidden;
}

.expand-result-btn {
  font-size: 11px;
  margin-top: 4px;
}

.lazy-ref-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: var(--acc-bg);
  border-radius: 4px;
  font-size: 11px;
  color: var(--acc);
  margin-top: 6px;
}
</style>

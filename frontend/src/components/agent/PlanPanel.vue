<template>
  <div class="plan-panel">
    <div class="plan-header">
      <span class="plan-icon">📋</span>
      <span class="plan-title">执行计划已生成</span>
      <el-tag type="warning" size="small">等待确认</el-tag>
    </div>

    <div class="plan-body">
      <!-- 计划详情 -->
      <div v-if="planDetails" class="plan-section">
        <div class="section-label">计划详情</div>
        <div class="section-content" v-html="renderMarkdown(planDetails)"></div>
      </div>

      <!-- 实施方法 -->
      <div v-if="planApproach" class="plan-section">
        <div class="section-label">实施方法</div>
        <div class="section-content" v-html="renderMarkdown(planApproach)"></div>
      </div>

      <!-- 修改模式 -->
      <div v-if="modifyMode" class="modify-area">
        <div class="section-label">修改计划</div>
        <el-input
          v-model="modifiedPlan"
          type="textarea"
          :rows="8"
          placeholder="在此修改计划内容..."
        />
      </div>
    </div>

    <div class="plan-actions">
      <template v-if="!modifyMode">
        <el-button type="primary" @click="handleConfirm">确认执行</el-button>
        <el-button @click="handleModify">修改计划</el-button>
        <el-button type="danger" plain @click="handleCancel">取消</el-button>
      </template>
      <template v-else>
        <el-button type="primary" @click="handleSubmitModified">提交修改后的计划</el-button>
        <el-button @click="handleCancelModify">取消修改</el-button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

const props = defineProps<{
  plan: string
}>()

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
  (e: 'modify', modifiedPlan: string): void
}>()

const modifyMode = ref(false)
const modifiedPlan = ref('')

// 解析 <plan_details>...</plan_details> 标签
const planDetails = computed(() => {
  const match = props.plan.match(/<plan_details>([\s\S]*?)<\/plan_details>/i)
  return match ? match[1].trim() : props.plan.trim()
})

// 解析 <plan_approach>...</plan_approach> 标签
const planApproach = computed(() => {
  const match = props.plan.match(/<plan_approach>([\s\S]*?)<\/plan_approach>/i)
  return match ? match[1].trim() : ''
})

function handleConfirm() {
  emit('confirm')
}

function handleModify() {
  modifiedPlan.value = props.plan
  modifyMode.value = true
}

function handleSubmitModified() {
  if (modifiedPlan.value.trim()) {
    emit('modify', modifiedPlan.value.trim())
  }
  modifyMode.value = false
}

function handleCancelModify() {
  modifyMode.value = false
  modifiedPlan.value = ''
}

function handleCancel() {
  emit('cancel')
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
  html = html.replace(/^\d+\.\s/gm, '<span class="step-num">$&</span>')
  html = html.replace(/\n/g, '<br>')
  return html
}
</script>

<style scoped>
.plan-panel {
  background: var(--card);
  border: 1px solid var(--acc);
  border-radius: 12px;
  padding: 16px;
  margin: 12px 0;
  box-shadow: var(--shadow);
}

.plan-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--bdr);
}

.plan-icon {
  font-size: 20px;
}

.plan-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--txt);
  flex: 1;
}

.plan-body {
  max-height: 400px;
  overflow-y: auto;
}

.plan-section {
  margin-bottom: 14px;
}

.section-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--acc);
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.section-content {
  font-size: 13px;
  line-height: 1.7;
  color: var(--txt);
  word-break: break-word;
}

.section-content :deep(pre) {
  background: #282c34;
  color: #abb2bf;
  padding: 10px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 8px 0;
}

.section-content :deep(code) {
  background: var(--bg2);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
  color: var(--red);
}

.section-content :deep(.step-num) {
  color: var(--acc);
  font-weight: 600;
}

.modify-area {
  margin-top: 12px;
}

.plan-actions {
  display: flex;
  gap: 10px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--bdr);
}
</style>

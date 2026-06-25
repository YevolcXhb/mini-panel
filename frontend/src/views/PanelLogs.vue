<template>
  <div class="page">
    <h2>面板运行日志</h2>
    <div class="toolbar">
      <el-checkbox-group v-model="selectedLevels">
        <el-checkbox value="info" label="Info" />
        <el-checkbox value="warning" label="Warning" />
        <el-checkbox value="error" label="Error" />
        <el-checkbox value="debug" label="Debug" />
        <el-checkbox value="fatal" label="Fatal" />
      </el-checkbox-group>
      <el-button :icon="Refresh" @click="fetchLogs" :loading="loading">刷新</el-button>
    </div>
    <div class="log-container" ref="logContainer">
      <div
        v-for="(entry, idx) in filteredEntries"
        :key="idx"
        class="log-line"
        :class="'level-' + entry.level"
      >
        <span class="log-time" v-if="entry.time">{{ entry.time }}</span>
        <span class="log-level">{{ entry.level.toUpperCase() }}</span>
        <span class="log-msg">{{ entry.message }}</span>
      </div>
      <div v-if="filteredEntries.length === 0" class="log-empty">暂无日志</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { logApi } from '../api'

interface LogEntry {
  level: string
  time: string
  message: string
  raw: string
}

const entries = ref<LogEntry[]>([])
const selectedLevels = ref<string[]>(JSON.parse(localStorage.getItem('logLevels') || '["info","warning","error","fatal"]'))
const loading = ref(false)
const logContainer = ref<HTMLDivElement>()
let timer: ReturnType<typeof setInterval> | null = null

const filteredEntries = computed(() => {
  const set = new Set(selectedLevels.value.map(v => v.toLowerCase()))
  return entries.value.filter(e => set.has(e.level.toLowerCase()))
})

async function fetchLogs() {
  loading.value = true
  try {
    const res: any = await logApi.list(selectedLevels.value, 500)
    entries.value = res.data || []
    await nextTick()
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  } catch (e) {
    // ignore
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchLogs()
  timer = setInterval(fetchLogs, 3000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

watch(selectedLevels, (v) => {
  localStorage.setItem('logLevels', JSON.stringify(v))
  fetchLogs()
})
</script>

<style scoped>
.page {
  padding: 20px;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
}
h2 {
  margin: 0 0 16px;
  font-size: 20px;
  color: var(--txt);
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.log-container {
  flex: 1;
  background: var(--card);
  border-radius: 8px;
  padding: 12px;
  overflow-y: auto;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.6;
  border: 1px solid var(--bg2);
}
.log-line {
  white-space: pre-wrap;
  word-break: break-all;
  padding: 2px 0;
}
.log-time {
  color: var(--txt2);
  margin-right: 8px;
}
.log-level {
  display: inline-block;
  min-width: 50px;
  font-weight: bold;
  margin-right: 8px;
}
:deep(.dark) .level-info .log-level,
.dark .level-info .log-level { color: #58a6ff; }
:deep(.dark) .level-warning .log-level,
.dark .level-warning .log-level { color: #d29922; }
:deep(.dark) .level-error .log-level,
.dark .level-error .log-level { color: #f85149; }
:deep(.dark) .level-debug .log-level,
.dark .level-debug .log-level { color: #a5d6ff; }
:deep(.dark) .level-fatal .log-level,
.dark .level-fatal .log-level { color: #da3633; }
:deep(.dark) .log-msg,
.dark .log-msg { color: #c9d1d9; }
.level-info .log-level { color: #0969da; }
.level-warning .log-level { color: #9a6700; }
.level-error .log-level { color: #cf222e; }
.level-debug .log-level { color: #1a7f37; }
.level-fatal .log-level { color: #82071e; }
.log-msg {
  color: var(--txt);
}
.log-empty {
  color: var(--txt2);
  text-align: center;
  padding: 40px 0;
}
</style>

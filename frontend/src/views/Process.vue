<template>
  <div>
    <h2 class="page-title">⚙️ 进程管理</h2>
    <div class="table-wrap">
      <el-table :data="filteredProcesses" v-loading="loading" size="small">
        <el-table-column prop="pid" label="PID" width="80" sortable />
        <el-table-column prop="name" label="名称" sortable />
        <el-table-column prop="cpu_percent" label="CPU%" width="100" sortable>
          <template #default="{ row }">
            <span :style="{ color: pctColor(row.cpu_percent) }">{{ row.cpu_percent?.toFixed(1) }}%</span>
          </template>
        </el-table-column>
        <el-table-column prop="mem_percent" label="内存%" width="100" sortable>
          <template #default="{ row }">{{ row.mem_percent?.toFixed(1) }}%</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="cmdline" label="命令行" show-overflow-tooltip min-width="200" />
        <el-table-column label="操作" width="150" :fixed="isWide ? 'right' : false">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button size="small" type="danger" @click="killProcess(row.pid, false)">结束</el-button>
              <el-button size="small" @click="killProcess(row.pid, true)">强制</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { processApi } from '../api'
import { usePolling } from '../composables/usePolling'
import { ElMessage, ElMessageBox } from 'element-plus'

// 窄屏移除操作列 sticky 固定，避免覆盖进程名列
const WIDE_MQ = '(min-width: 901px)'
const isWide = ref(typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ).matches : true)
const mql = typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ) : null
function onMqlChange(e: MediaQueryListEvent) { isWide.value = e.matches }

const processes = ref<any[]>([])
const loading = ref(false)
const search = ref('')

const filteredProcesses = computed(() => {
  if (!search.value) return processes.value
  return processes.value.filter(p =>
    p.name?.toLowerCase().includes(search.value.toLowerCase()) ||
    String(p.pid).includes(search.value)
  )
})

function pctColor(p: number) {
  return p > 80 ? 'var(--red)' : p > 50 ? 'var(--org)' : 'var(--grn)'
}

async function loadProcesses(showLoading = false) {
  if (showLoading) loading.value = true
  try {
    const res: any = await processApi.list()
    processes.value = res.data || []
  } catch (e) {
    console.error(e)
  } finally {
    if (showLoading) loading.value = false
  }
}

async function killProcess(pid: number, force: boolean) {
  try {
    await ElMessageBox.confirm(`确定要${force ? '强制' : ''}结束进程 ${pid} 吗？`, '确认', { confirmButtonClass: 'el-button--danger' })
    await processApi.kill(String(pid), force)
    ElMessage.success('操作成功')
    loadProcesses()
  } catch (e) {}
}

// keep-alive 兼容：切走页面即停轮询，切回来恢复（见 composables/usePolling.ts）
const polling = usePolling(() => loadProcesses(false), 10000)

onMounted(() => {
  loadProcesses(true)
  polling.start()
  if (mql) mql.addEventListener('change', onMqlChange)
})
onUnmounted(() => {
  polling.stop()
  if (mql) mql.removeEventListener('change', onMqlChange)
})
</script>

<style scoped>
.table-wrap {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.table-wrap :deep(.el-table) {
  min-width: 900px;
}
.row-actions {
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  align-items: center;
}
.row-actions .el-button {
  margin-left: 0 !important;
}
.table-wrap :deep(.el-table .cell) {
  white-space: nowrap;
  word-break: keep-all;
}

/* ── 移动端：进程名不再被压成竖排 ── */
@media (max-width: 900px) {
  .table-wrap {
    width: 100%;
    max-width: 100%;
    overflow-x: auto;
  }
  .table-wrap :deep(.el-table) {
    min-width: 900px;
  }
  .table-wrap :deep(.el-table .cell) {
    white-space: nowrap;
    word-break: keep-all;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

@media (max-width: 480px) {
  .table-wrap :deep(.el-table) {
    min-width: 860px;
  }
}
</style>

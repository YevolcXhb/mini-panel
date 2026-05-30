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
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="killProcess(row.pid, false)">结束</el-button>
            <el-button size="small" @click="killProcess(row.pid, true)">强制</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { processApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const processes = ref<any[]>([])
const loading = ref(false)
const search = ref('')
let timer: any

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

onMounted(() => { loadProcesses(true); timer = setInterval(() => loadProcesses(false), 10000) })
onUnmounted(() => clearInterval(timer))
</script>

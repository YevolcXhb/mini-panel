<template>
  <div>
    <el-card>
      <template #header>
        <div class="process-header">
          <span>进程管理</span>
          <el-input v-model="search" placeholder="搜索进程" size="small" style="width: 200px" clearable />
        </div>
      </template>
      <el-table :data="filteredProcesses" v-loading="loading" size="small">
        <el-table-column prop="pid" label="PID" width="80" sortable />
        <el-table-column prop="name" label="名称" sortable />
        <el-table-column prop="cpu_percent" label="CPU%" width="100" sortable>
          <template #default="{ row }">
            <el-progress :percentage="row.cpu_percent" :color="getCpuColor" />
          </template>
        </el-table-column>
        <el-table-column prop="mem_percent" label="内存%" width="100" sortable>
          <template #default="{ row }">
            {{ row.mem_percent?.toFixed(1) }}%
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="cmdline" label="命令行" show-overflow-tooltip />
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="killProcess(row.pid, false)">结束</el-button>
            <el-button size="small" type="danger" plain @click="killProcess(row.pid, true)">强制</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
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

const getCpuColor = (percentage: number) => {
  if (percentage < 10) return '#67c23a'
  if (percentage < 50) return '#e6a23c'
  return '#f56c6c'
}

async function loadProcesses() {
  loading.value = true
  try {
    const res: any = await processApi.list()
    processes.value = res.data || []
  } catch (e) {
  } finally {
    loading.value = false
  }
}

async function killProcess(pid: number, force: boolean) {
  try {
    await ElMessageBox.confirm(`确定要${force ? '强制' : ''}结束进程 ${pid} 吗？`, '确认')
    await processApi.kill(String(pid), force)
    ElMessage.success('操作成功')
    loadProcesses()
  } catch (e) {}
}

onMounted(() => {
  loadProcesses()
  timer = setInterval(loadProcesses, 5000)
})

onUnmounted(() => {
  clearInterval(timer)
})
</script>

<style scoped>
.process-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>

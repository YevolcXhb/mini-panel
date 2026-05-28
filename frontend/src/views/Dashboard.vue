<template>
  <div>
    <el-row :gutter="20">
      <el-col :span="6">
        <el-card>
          <div class="stat-card">
            <el-icon size="40" color="#409eff"><Cpu /></el-icon>
            <div class="stat-info">
              <div class="stat-label">CPU 使用率</div>
              <div class="stat-value">{{ monitor.cpu_usage?.toFixed(1) || 0 }}%</div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-card">
            <el-icon size="40" color="#67c23a"><Tickets /></el-icon>
            <div class="stat-info">
              <div class="stat-label">内存使用</div>
              <div class="stat-value">{{ formatBytes(monitor.memory?.used) }} / {{ formatBytes(monitor.memory?.total) }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-card">
            <el-icon size="40" color="#e6a23c"><Box /></el-icon>
            <div class="stat-info">
              <div class="stat-label">容器运行</div>
              <div class="stat-value">{{ runningContainers }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <div class="stat-card">
            <el-icon size="40" color="#f56c6c"><Timer /></el-icon>
            <div class="stat-info">
              <div class="stat-label">运行时间</div>
              <div class="stat-value">{{ formatUptime(info.system?.uptime) }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="12">
        <el-card title="系统信息">
          <template #header>系统信息</template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="主机名">{{ info.system?.hostname }}</el-descriptions-item>
            <el-descriptions-item label="操作系统">{{ info.system?.os }} {{ info.system?.platform }}</el-descriptions-item>
            <el-descriptions-item label="内核版本">{{ info.system?.kernel_version }}</el-descriptions-item>
            <el-descriptions-item label="架构">{{ info.system?.kernel_arch }}</el-descriptions-item>
            <el-descriptions-item label="进程数">{{ info.system?.procs }}</el-descriptions-item>
            <el-descriptions-item label="平台版本">{{ info.system?.platform_version }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card title="磁盘使用">
          <template #header>磁盘使用</template>
          <el-table :data="monitor.disks" size="small">
            <el-table-column prop="path" label="挂载点" />
            <el-table-column label="已用/总量">
              <template #default="{ row }">
                {{ formatBytes(row.used) }} / {{ formatBytes(row.total) }}
              </template>
            </el-table-column>
            <el-table-column label="使用率">
              <template #default="{ row }">
                <el-progress :percentage="row.used_percent" :status="row.used_percent > 90 ? 'exception' : ''" />
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="24">
        <el-card title="实时负载">
          <template #header>实时负载</template>
          <div ref="chartRef" style="height: 300px"></div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { dashboardApi, containerApi } from '../api'

const info = ref<any>({})
const monitor = ref<any>({})
const runningContainers = ref(0)
const chartRef = ref<HTMLElement>()
let timer: any

function formatBytes(bytes: number) {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

function formatUptime(seconds: number) {
  if (!seconds) return '0h'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}h ${m}m`
}

async function loadData() {
  try {
    const res: any = await dashboardApi.getInfo()
    info.value = res.data || {}
    const mon: any = await dashboardApi.getMonitor()
    monitor.value = mon.data || {}
    const ctn: any = await containerApi.list()
    const list = ctn.data || []
    runningContainers.value = list.filter((x: any) => x.status === 'running').length
  } catch (e) {
    // ignore
  }
}

onMounted(() => {
  loadData()
  timer = setInterval(loadData, 5000)
})

onUnmounted(() => {
  clearInterval(timer)
})
</script>

<style scoped>
.stat-card {
  display: flex;
  align-items: center;
  gap: 15px;
}
.stat-info {
  flex: 1;
}
.stat-label {
  color: #888;
  font-size: 14px;
}
.stat-value {
  font-size: 24px;
  font-weight: bold;
  margin-top: 5px;
}
</style>

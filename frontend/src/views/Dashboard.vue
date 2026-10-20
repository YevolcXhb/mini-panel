<template>
  <div>
    <h2 class="page-title">📊 系统概览</h2>

    <div class="info-grid">
      <div class="info-card">
        <div class="info-label">主机名</div>
        <div class="info-value">{{ info.system?.hostname || '-' }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">系统</div>
        <div class="info-value">{{ info.system?.platform || '-' }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">内核</div>
        <div class="info-value">{{ info.system?.kernel_version || '-' }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">架构</div>
        <div class="info-value">{{ info.system?.kernel_arch || '-' }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">运行时间</div>
        <div class="info-value">{{ formatUptime(info.system?.uptime) }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">CPU 核心</div>
        <div class="info-value">{{ info.cpu?.length || '-' }} 核</div>
      </div>
    </div>

    <h3 class="section-title">资源使用</h3>
    <div class="meter-grid">
      <div class="meter-card">
        <div class="meter-header">
          <span>CPU 占用</span>
          <span class="meter-pct" :style="{ color: pctColor(cpuPct) }">{{ cpuPct }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: cpuPct + '%', background: pctColor(cpuPct) }"></div>
        </div>
        <div class="meter-detail">使用率: {{ monitor.cpu_usage?.toFixed(1) || 0 }}%</div>
      </div>
      <div class="meter-card">
        <div class="meter-header">
          <span>CPU 负载</span>
          <span class="meter-pct" :style="{ color: pctColor(loadPct) }">{{ loadPct }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: loadPct + '%', background: pctColor(loadPct) }"></div>
        </div>
        <div class="meter-detail">负载: {{ monitor.load?.load1?.toFixed(2) || 0 }} / {{ monitor.load?.load5?.toFixed(2) || 0 }} / {{ monitor.load?.load15?.toFixed(2) || 0 }}</div>
      </div>
      <div class="meter-card">
        <div class="meter-header">
          <span>内存使用</span>
          <span class="meter-pct" :style="{ color: pctColor(Number(memPct)) }">{{ memPct }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill mem" :style="{ width: memPct + '%', background: pctColor(Number(memPct)) }"></div>
        </div>
        <div class="meter-detail">{{ formatBytes(monitor.memory?.used) }} / {{ formatBytes(monitor.memory?.total) }}</div>
      </div>
      <div class="meter-card">
        <div class="meter-header">
          <span>磁盘使用</span>
          <span class="meter-pct" :style="{ color: pctColor(Number(diskPct)) }">{{ diskPct }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill disk" :style="{ width: diskPct + '%', background: pctColor(Number(diskPct)) }"></div>
        </div>
        <div class="meter-detail">{{ formatBytes(diskUsed) }} / {{ formatBytes(diskTotal) }}</div>
      </div>
    </div>

    <h3 class="section-title">磁盘详情</h3>
    <div class="table-wrap">
      <el-table :data="monitor.disks" size="small" style="width: 100%">
        <el-table-column prop="path" label="挂载点" />
        <el-table-column label="已用/总量">
          <template #default="{ row }">{{ formatBytes(row.used) }} / {{ formatBytes(row.total) }}</template>
        </el-table-column>
        <el-table-column label="使用率" width="180">
          <template #default="{ row }">
            <div class="progress-bar" style="margin-top:4px">
              <div class="progress-fill" :style="{ width: row.used_percent + '%', background: pctColor(row.used_percent) }"></div>
            </div>
            <span style="font-size:11px;color:var(--dim)">{{ row.used_percent?.toFixed(1) }}%</span>
          </template>
        </el-table-column>
        <el-table-column prop="fs_type" label="类型" width="100" />
      </el-table>
    </div>

    <h3 class="section-title">网络监控</h3>
    <div class="table-wrap">
      <el-table :data="monitor.network" size="small" style="width: 100%">
        <el-table-column prop="name" label="接口" />
        <el-table-column label="接收">
          <template #default="{ row }">{{ formatBytes(row.bytes_recv) }}</template>
        </el-table-column>
        <el-table-column label="发送">
          <template #default="{ row }">{{ formatBytes(row.bytes_sent) }}</template>
        </el-table-column>
        <el-table-column label="包数(收/发)">
          <template #default="{ row }">{{ row.packets_recv }} / {{ row.packets_sent }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { dashboardApi } from '../api'
import { usePolling } from '../composables/usePolling'

const info = ref<any>({})
const monitor = ref<any>({})
let loading = false
let hostMode = ''

const cpuPct = computed(() => {
  const v = monitor.value.cpu_usage || 0
  return Math.min(Math.round(v), 100)
})
const loadPct = computed(() => {
  const l = monitor.value.load?.load1 || 0
  const cores = info.value.cpu?.length || 1
  return Math.min(Math.round((l / cores) * 100), 100)
})
const memPct = computed(() => {
  const m = monitor.value.memory
  if (!m || !m.total) return 0
  return ((m.used / m.total) * 100).toFixed(1)
})
const diskUsed = computed(() => {
  const d = monitor.value.disks || []
  return d.reduce((sum: number, x: any) => sum + (x.used || 0), 0)
})
const diskTotal = computed(() => {
  const d = monitor.value.disks || []
  return d.reduce((sum: number, x: any) => sum + (x.total || 0), 0)
})
const diskPct = computed(() => {
  if (!diskTotal.value) return 0
  return ((diskUsed.value / diskTotal.value) * 100).toFixed(1)
})

function formatBytes(bytes: number) {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
function formatUptime(seconds: number) {
  if (!seconds) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor(seconds % 86400 / 3600)
  const m = Math.floor(seconds % 3600 / 60)
  if (d > 0) return d + '天' + h + '时'
  if (h > 0) return h + '时' + m + '分'
  return m + '分'
}
function pctColor(p: number) {
  return p > 80 ? 'var(--red)' : p > 60 ? 'var(--org)' : 'var(--grn)'
}

async function loadData() {
  // 互斥：上一轮还没回来就跳过，避免慢请求堆积
  // （/dashboard/monitor 单次可达 700ms，叠压会形成自我拥塞）
  if (loading) return
  loading = true
  try {
    const res: any = await dashboardApi.getInfo()
    info.value = res.data || {}
    // load_host_mode 很少变，只在首次拉取，避免每 5 秒重复请求 settings
    if (!hostMode) {
      const settingsRes: any = await (await import('../api')).settingApi.get()
      hostMode = settingsRes?.data?.load_host_mode || 'chroot'
    }
    const mon: any = await dashboardApi.getMonitor(hostMode)
    monitor.value = mon.data || {}
  } catch (e) {
  } finally {
    loading = false
  }
}

// keep-alive 兼容：页面切走时停轮询，切回来恢复（见 composables/usePolling.ts）
const polling = usePolling(loadData, 5000)

onMounted(() => {
  loadData()
  polling.start()
})
</script>

<style scoped>
.page-title { margin-bottom: 20px; }
.section-title { margin-top: 24px; }

/* ── 表格容器 ──
   关键：不要给 .el-table 设 min-width。设了 min-width，el-table 根节点就会
   比容器宽、整张表撑破视口（docW=390 但 table right=574 就是这个原因）。
   让 el-table 收缩到容器宽度，横向滚动交给 el-table 自己的
   .el-scrollbar__wrap（实测 cw=322 / sw=920，内部滚动本来就能用）。 */
.table-wrap {
  width: 100%;
  max-width: 100%;
  min-width: 0;
}
.table-wrap :deep(.el-table) {
  width: 100%;
  min-width: 0;
  max-width: 100%;
}

@media (max-width: 900px) {
  /* 窄屏再额外兜一层：万一全局 .table-wrap .el-table 的 min-width 没被改掉，
     外层也还有滚动条可用，不会把整个页面撑出横向位移 */
  .table-wrap {
    overflow-x: auto;
    overflow-y: hidden;
    -webkit-overflow-scrolling: touch;
    overscroll-behavior-x: contain;
  }
  .table-wrap :deep(.el-table .cell) { word-break: break-all; }
}
</style>

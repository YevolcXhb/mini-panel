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
          <span>整机功耗</span>
          <span class="meter-pct power-value" :title="powerHint">
            <template v-if="powerAvg > 0">
              <span v-if="powerSign === 'charge'" class="pw-arrow up">↑</span>
              <span v-else-if="powerSign === 'discharge'" class="pw-arrow down">↓</span>
              <span v-if="powerIsAveraged" class="pw-approx">≈</span>{{ powerAvg.toFixed(2) }}<span class="pw-unit">W</span>
            </template>
            <template v-else>— W</template>
          </span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill power" :style="{ width: powerBarPct + '%', background: powerColor }"></div>
        </div>
        <div class="meter-detail">{{ powerDetail }}</div>
      </div>
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

// ─── 整机功耗 ───
//
// 数据来自 /monitor/battery，后端按供电形态自动选源：
//   usb.online=1 → usb 节点 V×I（适配器进线侧，≈ 整机取电）
//   usb.online=0 → battery 节点
// usb/current_now 零值率约 50%，所以这里再做一次 3 点滑动平均稳住显示。
const battery = ref<any>(null)
const POWER_AVG_WINDOW = 3
const powerHistory = ref<number[]>([])

function pushPowerSample(w: number) {
  if (!(w > 0)) return
  const arr = powerHistory.value
  arr.push(w)
  if (arr.length > POWER_AVG_WINDOW) arr.shift()
}

const powerAvg = computed(() => {
  const arr = powerHistory.value
  if (arr.length === 0) return 0
  return arr.reduce((a, b) => a + b, 0) / arr.length
})
const powerIsAveraged = computed(() => powerHistory.value.length > 1)
const powerSign = computed(() => battery.value?.power_sign || '')

// 进度条刻度：2W 视为满格（本机实测满载约 2.0W），超出则按比例钳到 100%
const powerBarPct = computed(() => {
  const w = powerAvg.value
  if (!(w > 0)) return 0
  return Math.min(Math.round((w / 2) * 100), 100)
})
const powerColor = computed(() => {
  const w = powerAvg.value
  if (w >= 1.5) return 'var(--org)'   // 接近满载
  if (w >= 0.8) return 'var(--blue)'  // 中等负载
  return 'var(--grn)'                 // 空闲
})
const powerDetail = computed(() => {
  const b = battery.value
  if (!b?.present) return '未检测到电池'
  if (!(powerAvg.value > 0)) {
    return b.power_mode === 'usb' ? '外接供电，无有效读数' : '电池供电，无有效读数'
  }
  const parts: string[] = []
  if (b.power_detail) parts.push(b.power_detail)
  if (b.power_mode === 'usb') parts.push('适配器进线侧')
  return parts.join(' · ')
})
const powerHint = computed(() => {
  const b = battery.value
  if (!b) return ''
  const parts: string[] = []
  if (b.voltage_v) parts.push(`${b.voltage_v.toFixed(2)}V`)
  if (b.current_ma !== undefined) parts.push(`${Math.abs(b.current_ma).toFixed(1)}mA`)
  if (b.power_detail) parts.push(b.power_detail)
  if (powerIsAveraged.value) parts.push(`最近 ${powerHistory.value.length} 次读数的平均值`)
  return parts.join(' · ')
})

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
    // 整机功耗：单独取 battery 接口，失败不影响其它卡片
    try {
      const bat: any = await (await import('../api')).monitorApi.battery()
      battery.value = bat?.data || null
      if (battery.value?.power_source && battery.value.power_source !== 'none') {
        pushPowerSample(Number(battery.value.power_w) || 0)
      }
    } catch (e) {}
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

/* ── 整机功耗卡片 ── */
.power-value {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
}
.pw-unit {
  font-size: 12px;
  font-weight: 500;
  color: var(--dim);
  margin-left: 1px;
}
.pw-arrow { font-size: 12px; font-weight: 700; }
.pw-arrow.up { color: var(--grn); }
.pw-arrow.down { color: var(--org); }
/* 做过滑动平均时用 ≈ 提示，避免把平滑值当成瞬时精确值 */
.pw-approx { color: var(--dim); font-weight: 600; margin-right: 1px; }


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
